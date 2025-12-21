package controllers

import (
	"messaging-app/internal/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type UploadController struct {
	storageService *services.StorageService
}

func NewUploadController(storageService *services.StorageService) *UploadController {
	return &UploadController{storageService: storageService}
}

// Upload godoc
// @Summary Upload files
// @Security BearerAuth
// @Tags upload
// @Accept multipart/form-data
// @Produce json
// @Param files formData file true "Files to upload"
// @Success 200 {object} []models.MediaItem
// @Failure 400 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/upload [post]
func (c *UploadController) Upload(ctx *gin.Context) {
	// Parse multipart form
	form, err := ctx.MultipartForm()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse multipart form: " + err.Error()})
		return
	}

	files := form.File["files[]"]
	if len(files) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "no files provided"})
		return
	}

	// Enforce file size limit (e.g., 100MB per file)
	const MaxFileSize = 100 * 1024 * 1024 // 100MB
	for _, file := range files {
		if file.Size > MaxFileSize {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file too large (max 100MB)"})
			return
		}
	}

	mediaItems, err := c.storageService.UploadFiles(ctx.Request.Context(), files)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload files: " + err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, mediaItems)
}
