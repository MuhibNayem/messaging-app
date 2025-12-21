package controllers

import (
	"gitlab.com/spydotech-group/shared-entity/models"
	"messaging-app/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ReelController struct {
	reelService *services.ReelService
}

func NewReelController(reelService *services.ReelService) *ReelController {
	return &ReelController{reelService: reelService}
}

func (c *ReelController) CreateReel(ctx *gin.Context) {
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req models.CreateReelRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	objUserID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	reel, err := c.reelService.CreateReel(ctx.Request.Context(), objUserID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, reel)
}

func (c *ReelController) GetReelsFeed(ctx *gin.Context) {
	limitStr := ctx.DefaultQuery("limit", "10")
	offsetStr := ctx.DefaultQuery("offset", "0")

	limit, _ := strconv.ParseInt(limitStr, 10, 64)
	offset, _ := strconv.ParseInt(offsetStr, 10, 64)

	userID, _ := ctx.Get("userID")
	objUserID, _ := primitive.ObjectIDFromHex(userID.(string))

	reels, err := c.reelService.GetReelsFeed(ctx.Request.Context(), objUserID, limit, offset)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, reels)
}

func (c *ReelController) GetUserReels(ctx *gin.Context) {
	targetUserIDStr := ctx.Param("id")
	targetUserID, err := primitive.ObjectIDFromHex(targetUserIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	reels, err := c.reelService.GetUserReels(ctx.Request.Context(), targetUserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, reels)
}

func (c *ReelController) GetReel(ctx *gin.Context) {
	reelIDStr := ctx.Param("id")
	reelID, err := primitive.ObjectIDFromHex(reelIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reel ID"})
		return
	}

	reel, err := c.reelService.GetReel(ctx.Request.Context(), reelID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, reel)
}

func (c *ReelController) AddComment(ctx *gin.Context) {
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	reelIDStr := ctx.Param("id")
	reelID, err := primitive.ObjectIDFromHex(reelIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reel ID"})
		return
	}

	var req struct {
		Content  string               `json:"content" binding:"required"`
		Mentions []primitive.ObjectID `json:"mentions"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	objUserID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	comment, err := c.reelService.AddComment(ctx.Request.Context(), reelID, objUserID, req.Content, req.Mentions)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, comment)
}

func (c *ReelController) GetComments(ctx *gin.Context) {
	reelIDStr := ctx.Param("id")
	reelID, err := primitive.ObjectIDFromHex(reelIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reel ID"})
		return
	}

	limitStr := ctx.DefaultQuery("limit", "20")
	offsetStr := ctx.DefaultQuery("offset", "0")

	limit, _ := strconv.ParseInt(limitStr, 10, 64)
	offset, _ := strconv.ParseInt(offsetStr, 10, 64)

	comments, err := c.reelService.GetComments(ctx.Request.Context(), reelID, limit, offset)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, comments)
}

func (c *ReelController) AddReply(ctx *gin.Context) {
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	reelIDStr := ctx.Param("id")
	reelID, err := primitive.ObjectIDFromHex(reelIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reel ID"})
		return
	}

	commentIDStr := ctx.Param("commentId")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid comment ID"})
		return
	}

	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	objUserID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	reply, err := c.reelService.AddReply(ctx.Request.Context(), reelID, commentID, objUserID, req.Content)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, reply)
}

func (c *ReelController) ReactToComment(ctx *gin.Context) {
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	reelIDStr := ctx.Param("id")
	reelID, err := primitive.ObjectIDFromHex(reelIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reel ID"})
		return
	}

	commentIDStr := ctx.Param("commentId")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid comment ID"})
		return
	}

	var req struct {
		ReactionType models.ReactionType `json:"reaction_type" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	objUserID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	err = c.reelService.ReactToComment(ctx.Request.Context(), reelID, commentID, objUserID, req.ReactionType)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Reacted successfully"})
}

func (c *ReelController) IncrementView(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	reelIDStr := ctx.Param("id")
	reelID, err := primitive.ObjectIDFromHex(reelIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reel ID"})
		return
	}

	err = c.reelService.IncrementViews(ctx.Request.Context(), reelID, objUserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "View incremented"})
}

// ReactToReel handles reacting to a reel
func (c *ReelController) ReactToReel(ctx *gin.Context) {
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	reelIDStr := ctx.Param("id")
	reelID, err := primitive.ObjectIDFromHex(reelIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reel ID"})
		return
	}

	var req struct {
		Type models.ReactionType `json:"type" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	objUserID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	err = c.reelService.ReactToReel(ctx.Request.Context(), reelID, objUserID, req.Type)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Reacted successfully"})
}
