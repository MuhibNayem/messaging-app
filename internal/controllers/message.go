package controllers

import (
	"net/http"
	"strconv"

	"messaging-app/internal/models"
	"messaging-app/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type MessageController struct {
	messageService *services.MessageService
}

func NewMessageController(messageService *services.MessageService) *MessageController {
	return &MessageController{messageService: messageService}
}

// @Summary Send a message
// @Description Send a direct or group message
// @Tags messages
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param message body models.MessageRequest true "Message to send"
// @Success 201 {object} models.Message
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages [post]
func (c *MessageController) SendMessage(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	senderID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	var req models.MessageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	// Validate content
	if req.Content == "" && len(req.MediaURLs) == 0 {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "message content or media URLs required"})
		return
	}

	// Validate content type
	if !models.IsValidContentType(req.ContentType) {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid content type"})
		return
	}

	// Validate that either receiverID or groupID is provided but not both
	if req.ReceiverID == "" && req.GroupID == "" {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "either receiverID or groupID must be provided"})
		return
	}
	if req.ReceiverID != "" && req.GroupID != "" {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "cannot specify both receiverID and groupID"})
		return
	}

	message, err := c.messageService.SendMessage(ctx.Request.Context(), senderID, req)
	if err != nil {
		statusCode := http.StatusBadRequest
		switch err.Error() {
		case "not a group member", "can only message friends":
			statusCode = http.StatusForbidden
		case "group not found", "receiver not found":
			statusCode = http.StatusNotFound
		}
		ctx.JSON(statusCode, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, message)
}

// @Summary Get messages
// @Description Get messages for a conversation or group
// @Tags messages
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param groupID query string false "Group ID"
// @Param receiverID query string false "Receiver ID"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Messages per page" default(50)
// @Param before query string false "Get messages before this timestamp (RFC3339)"
// @Success 200 {object} models.MessageResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages [get]
func (c *MessageController) GetMessages(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	senderID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	// Get query parameters
	page, err := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(ctx.DefaultQuery("limit", "50"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 50
	}

	// Check if this is a group conversation or direct message
	groupID := ctx.Query("groupID")
	receiverID := ctx.Query("receiverID")
	before := ctx.Query("before")

	query := models.MessageQuery{
		SenderID:   senderID.Hex(),
		Page:       page,
		Limit:      limit,
		GroupID:    groupID,
		ReceiverID: receiverID,
		Before:     before,
	}

	// Validate the query
	if groupID != "" && receiverID != "" {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "cannot specify both groupID and receiverID"})
		return
	}
	if groupID == "" && receiverID == "" {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "must specify either groupID or receiverID"})
		return
	}

	messages, err := c.messageService.GetAllMessages(ctx.Request.Context(), query)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	// Get total count for pagination
	total, err := c.messageService.GetConversationMessageTotalCount(ctx.Request.Context(), query)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	response := models.MessageResponse{
		Messages: messages,
		Total:    total,
		Page:     int64(page),
		Limit:    int64(limit),
		HasMore:  int64(page)*int64(limit) < total,
	}

	ctx.JSON(http.StatusOK, response)
}

// @Summary Mark messages as seen
// @Description Mark messages as seen by the current user
// @Tags messages
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param messageIDs body []string true "Array of message IDs to mark as seen"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages/seen [post]
func (c *MessageController) MarkMessagesAsSeen(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	currentUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	var messageIDs []string
	if err := ctx.ShouldBindJSON(&messageIDs); err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	if len(messageIDs) == 0 {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "at least one message ID required"})
		return
	}

	// Convert string IDs to ObjectIDs
	var objectIDs []primitive.ObjectID
	for _, id := range messageIDs {
		objID, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid message ID: " + id})
			return
		}
		objectIDs = append(objectIDs, objID)
	}

	err = c.messageService.MarkMessagesAsSeen(ctx.Request.Context(), currentUserID, objectIDs)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}

// @Summary Get unread message count
// @Description Get count of unread messages for the current user
// @Tags messages
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} models.UnreadCountResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages/unread [get]
func (c *MessageController) GetUnreadCount(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	currentUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	count, err := c.messageService.GetUnreadCount(ctx.Request.Context(), currentUserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.UnreadCountResponse{Count: count})
}

// @Summary Delete a message
// @Description Delete a message (only for sender or admin)
// @Tags messages
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Message ID"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages/{id} [delete]
func (c *MessageController) DeleteMessage(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	currentUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	messageID := ctx.Param("id")
	objID, err := primitive.ObjectIDFromHex(messageID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid message ID"})
		return
	}

	_, err = c.messageService.DeleteMessage(ctx.Request.Context(), objID.Hex(), currentUserID)
	if err != nil {
		switch err.Error() {
		case "message not found":
			ctx.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		case "not authorized to delete this message":
			ctx.JSON(http.StatusForbidden, models.ErrorResponse{Error: err.Error()})
		default:
			ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}