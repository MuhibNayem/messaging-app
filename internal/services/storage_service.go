package services

import (
	"context"
	"fmt"
	"messaging-app/config"
	"messaging-app/internal/models"
	"mime/multipart"
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
	// Initialize MinIO client object.
	// For local development with Docker Compose:
	// Endpoint: minio:9000 (internal Docker network)
	// AccessKey: minioadmin
	// SecretKey: minioadmin

	// Note: In production, these should come from config.
	// Assuming config has been updated or we use defaults for now based on the docker-compose we just observed.
	// The docker-compose uses minioadmin/minioadmin and port 9000.

	endpoint := "minio:9000"
	accessKeyID := "minioadmin"
	secretAccessKey := "minioadmin"
	useSSL := false
	bucketName := "connectify-uploads"
	// External host for browser access. Localhost mapped port is 9000.
	externalHost := "http://localhost:9000"

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
