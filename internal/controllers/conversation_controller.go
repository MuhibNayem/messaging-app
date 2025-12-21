package controllers

import (
	"log"
	"messaging-app/internal/models"
	"messaging-app/internal/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ConversationController struct {
	conversationService *services.ConversationService
}

func NewConversationController(cs *services.ConversationService) *ConversationController {
	return &ConversationController{conversationService: cs}
}

// @Summary Get conversation summaries
// @Description Get a list of all conversations (direct and group) with last message details
// @Tags conversations
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {array} models.ConversationSummary
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /conversations [get]
func (c *ConversationController) GetConversationSummaries(ctx *gin.Context) {
	log.Printf("[%s] Entering GetConversationSummaries controller", ctx.GetString("requestID"))
	userID := ctx.MustGet("userID").(string)
	currentUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		log.Printf("[%s] Error parsing user ID %s: %v", ctx.GetString("requestID"), userID, err)
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	log.Printf("[%s] Calling conversation service for user %s", ctx.GetString("requestID"), currentUserID.Hex())
	summaries, err := c.conversationService.GetConversationSummaries(ctx.Request.Context(), currentUserID)
	if err != nil {
		log.Printf("[%s] Error from conversation service: %v", ctx.GetString("requestID"), err)
		ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to retrieve conversation summaries"})
		return
	}

	log.Printf("[%s] Successfully retrieved %d conversation summaries for user %s", ctx.GetString("requestID"), len(summaries), currentUserID.Hex())
	ctx.JSON(http.StatusOK, summaries)
}
