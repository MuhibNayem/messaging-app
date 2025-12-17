package controllers

import (
	"messaging-app/internal/models"
	"messaging-app/internal/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type StoryController struct {
	storyService *services.StoryService
}

func NewStoryController(storyService *services.StoryService) *StoryController {
	return &StoryController{storyService: storyService}
}

func (c *StoryController) CreateStory(ctx *gin.Context) {
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req models.CreateStoryRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	objUserID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	story, err := c.storyService.CreateStory(ctx.Request.Context(), objUserID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, story)
}

func (c *StoryController) GetStoriesFeed(ctx *gin.Context) {
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	objUserID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req struct {
		Limit  int `form:"limit"`
		Offset int `form:"offset"`
	}
	if err := ctx.ShouldBindQuery(&req); err != nil {
		// Default values if binding fails or params missing (though ShouldBindQuery usually doesn't error on missing optional fields if not 'binding:"required"')
		req.Limit = 10
		req.Offset = 0
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	stories, err := c.storyService.GetStoriesFeed(ctx.Request.Context(), objUserID, req.Limit, req.Offset)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, stories)
}

func (c *StoryController) GetUserStories(ctx *gin.Context) {
	targetUserIDStr := ctx.Param("id")
	targetUserID, err := primitive.ObjectIDFromHex(targetUserIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	stories, err := c.storyService.GetUserStories(ctx.Request.Context(), targetUserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, stories)
}

func (c *StoryController) DeleteStory(ctx *gin.Context) {
	// TODO: implement delete
	ctx.Status(http.StatusNotImplemented)
}

func (c *StoryController) ViewStory(ctx *gin.Context) {
	storyIDStr := ctx.Param("id")
	storyID, err := primitive.ObjectIDFromHex(storyIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid story ID"})
		return
	}

	userID, _ := ctx.Get("userID")
	objUserID, _ := primitive.ObjectIDFromHex(userID.(string))

	if err := c.storyService.RecordView(ctx.Request.Context(), storyID, objUserID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

func (c *StoryController) ReactToStory(ctx *gin.Context) {
	storyIDStr := ctx.Param("id")
	storyID, err := primitive.ObjectIDFromHex(storyIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid story ID"})
		return
	}

	var req struct {
		Type string `json:"type" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := ctx.Get("userID")
	objUserID, _ := primitive.ObjectIDFromHex(userID.(string))

	if err := c.storyService.ReactToStory(ctx.Request.Context(), storyID, objUserID, req.Type); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

func (c *StoryController) GetStoryViewers(ctx *gin.Context) {
	storyIDStr := ctx.Param("id")
	storyID, err := primitive.ObjectIDFromHex(storyIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid story ID"})
		return
	}

	userID, _ := ctx.Get("userID")
	objUserID, _ := primitive.ObjectIDFromHex(userID.(string))

	viewers, err := c.storyService.GetStoryViewers(ctx.Request.Context(), storyID, objUserID)
	if err != nil {
		// unauthorized or not found
		ctx.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, viewers)
}
