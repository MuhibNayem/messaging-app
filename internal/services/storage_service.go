package services

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"messaging-app/config"
	"messaging-app/internal/models"
	"mime/multipart"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type StorageService struct {
	client       *minio.Client
	bucketName   string
	endpoint     string
	useSSL       bool
	externalHost string // For constructing public URLs
}

func NewStorageService(cfg *config.Config) (*StorageService, error) {
	endpoint := cfg.StorageEndpoint
	accessKeyID := cfg.StorageAccessKey
	secretAccessKey := cfg.StorageSecretKey
	useSSL := cfg.StorageUseSSL
	bucketName := cfg.StorageBucket
	externalHost := cfg.StoragePublicURL

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	// Check if bucket exists, create if not
	ctx := context.Background()
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to check if bucket exists: %w", err)
	}

	if !exists {
		err = minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
		fmt.Printf("Successfully created bucket %s\n", bucketName)

		// Set public policy
		policy := fmt.Sprintf(`{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Principal": {"AWS": ["*"]},
					"Action": ["s3:GetObject"],
					"Resource": ["arn:aws:s3:::%s/*"]
				}
			]
		}`, bucketName)

		err = minioClient.SetBucketPolicy(ctx, bucketName, policy)
		if err != nil {
			return nil, fmt.Errorf("failed to set bucket policy: %w", err)
		}
		fmt.Printf("Successfully set public policy for bucket %s\n", bucketName)
	}

	return &StorageService{
		client:       minioClient,
		bucketName:   bucketName,
		endpoint:     endpoint,
		useSSL:       useSSL,
		externalHost: externalHost,
	}, nil
}

// UploadFiles uploads multiple files in parallel and returns a slice of MediaItems
func (s *StorageService) UploadFiles(ctx context.Context, files []*multipart.FileHeader) ([]models.MediaItem, error) {
	var wg sync.WaitGroup
	results := make([]models.MediaItem, len(files))
	errors := make([]error, len(files))

	for i, fileHeader := range files {
		wg.Add(1)
		go func(index int, fh *multipart.FileHeader) {
			defer wg.Done()

			// Open the file
			file, err := fh.Open()
			if err != nil {
				errors[index] = fmt.Errorf("failed to open file %s: %v", fh.Filename, err)
				return
			}
			defer file.Close()

			// Generate a unique object name
			ext := filepath.Ext(fh.Filename)
			objectName := fmt.Sprintf("%d-%s%s", time.Now().UnixNano(), uuid.New().String(), ext)

			// Determine content type
			contentType := fh.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}

			// Determine media type for our model (image vs video)
			mediaType := "image" // Default
			if strings.HasPrefix(contentType, "video/") {
				mediaType = "video"
			}

			// Upload
			info, err := s.client.PutObject(ctx, s.bucketName, objectName, file, fh.Size, minio.PutObjectOptions{
				ContentType: contentType,
			})
			if err != nil {
				errors[index] = fmt.Errorf("failed to upload file %s: %v", fh.Filename, err)
				return
			}

			// Construct URL
			// Since we set the bucket policy to public, we can construct the direct URL.
			url := fmt.Sprintf("%s/%s/%s", s.externalHost, s.bucketName, info.Key)

			results[index] = models.MediaItem{
				URL:  url,
				Type: mediaType,
			}
		}(i, fileHeader)
	}

	wg.Wait()

	// Check for errors
	// If any upload failed, we return an error (and arguably should cleanup, but for now simple fail)
	for _, err := range errors {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// DeleteFile deletes a file from object storage given its full URL
func (s *StorageService) DeleteFile(ctx context.Context, fileURL string) error {
	// Extract object name from URL
	// URL format: externalHost/bucketName/objectName
	// We can parse the URL and take the last part of the path

	parsedURL, err := url.Parse(fileURL)
	if err != nil {
		return fmt.Errorf("invalid file URL: %w", err)
	}

	path := parsedURL.Path
	// Remove leading slash if present
	path = strings.TrimPrefix(path, "/")

	// Path should be "bucketName/objectKey"
	// Remove bucketName prefix
	prefix := s.bucketName + "/"
	if !strings.HasPrefix(path, prefix) {
		// Maybe the URL path doesn't include bucket name if using virtual-host style
		// But based on UploadFiles, it does.
		// Let's handle generic case: just take the filename (basename) if standard structure
		// But safest is to strip expected prefix.
	}
	objectName := strings.TrimPrefix(path, prefix)

	// Delete object
	err = s.client.RemoveObject(ctx, s.bucketName, objectName, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object %s: %w", objectName, err)
	}

	return nil
}

// UploadArchive uploads compressed JSON archive to a specific bucket and path
// Used for tiered message storage - cold storage for old messages
func (s *StorageService) UploadArchive(ctx context.Context, archiveBucket, objectPath string, data []byte) error {
	// Compress data with gzip
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	_, err := gzWriter.Write(data)
	if err != nil {
		return fmt.Errorf("failed to compress archive: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Ensure bucket exists
	exists, err := s.client.BucketExists(ctx, archiveBucket)
	if err != nil {
		return fmt.Errorf("failed to check archive bucket: %w", err)
	}
	if !exists {
		if err := s.client.MakeBucket(ctx, archiveBucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("failed to create archive bucket: %w", err)
		}
	}

	// Upload compressed archive
	_, err = s.client.PutObject(ctx, archiveBucket, objectPath, &buf, int64(buf.Len()), minio.PutObjectOptions{
		ContentType:     "application/gzip",
		ContentEncoding: "gzip",
	})
	if err != nil {
		return fmt.Errorf("failed to upload archive: %w", err)
	}

	return nil
}

// DownloadArchive downloads and decompresses JSON archive from cold storage
func (s *StorageService) DownloadArchive(ctx context.Context, archiveBucket, objectPath string) ([]byte, error) {
	// Get object from MinIO
	obj, err := s.client.GetObject(ctx, archiveBucket, objectPath, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get archive object: %w", err)
	}
	defer obj.Close()

	// Decompress gzip
	gzReader, err := gzip.NewReader(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	data, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive: %w", err)
	}

	return data, nil
}
