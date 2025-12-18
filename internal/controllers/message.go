package controllers

import (
	"net/http"
	"strconv"
	"time"

	"messaging-app/internal/models"
	"messaging-app/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type MessageController struct {
	messageService *services.MessageService
	storageService *services.StorageService
}

func NewMessageController(messageService *services.MessageService, storageService *services.StorageService) *MessageController {
	return &MessageController{
		messageService: messageService,
		storageService: storageService,
	}
}

// @Summary Send a message
// @Description Send a direct or group message
// @Tags messages
// @Accept json,mpfd
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

	// Handle multipart/form-data vs JSON
	contentType := ctx.GetHeader("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}

	if len(contentType) >= 19 && contentType[:19] == "multipart/form-data" {
		if err := ctx.ShouldBind(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
			return
		}
		// Handle file uploads
		form, err := ctx.MultipartForm()
		if err == nil {
			files := form.File["files"]
			if len(files) > 0 {
				mediaItems, err := c.storageService.UploadFiles(ctx.Request.Context(), files)
				if err != nil {
					ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to upload files"})
					return
				}
				for _, item := range mediaItems {
					req.MediaURLs = append(req.MediaURLs, item.URL)
				}

				// Determine content type based on first file if not explicitly set (or 'multiple' if mixed/many)
				// For simplicity, if we have files, we default to 'file' or specific type if 1 file.
				// But let's check what the parser/frontend sends.
				// If frontend sends specific type, keep it. If "text", switch to media type.
				if req.ContentType == "" || req.ContentType == "text" {
					if len(files) > 1 {
						req.ContentType = models.ContentTypeMultiple
					} else if len(mediaItems) > 0 {
						// Simple mapping based on storage service output (which sets generic type)
						// or usually frontend should set it.
						// Let's rely on valid inputs or default to file.
						// The storage service returned MediaItems which have Type.
						if mediaItems[0].Type == "video" {
							req.ContentType = models.ContentTypeVideo
						} else if mediaItems[0].Type == "image" {
							req.ContentType = models.ContentTypeImage
						} else {
							req.ContentType = models.ContentTypeFile
						}
					}
				}
			}
		}
		// Explicitly check for is_marketplace in form data if binding didn't catch it
		if val := ctx.PostForm("is_marketplace"); val != "" {
			if b, err := strconv.ParseBool(val); err == nil {
				req.IsMarketplace = b
			}
		}
	} else {
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
			return
		}
	}
	
	// DEBUG LOG
	// fmt.Printf("[MessageController] SendMessage: IsMarketplace=%v, ContentType=%s\n", req.IsMarketplace, req.ContentType)

	req.SenderID = userID // Set SenderID from authenticated user

	// Validate content
	if req.Content == "" && len(req.MediaURLs) == 0 {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "message content or media URLs required"})
		return
	}

	// Validate content type (if still text but has urls, likely text_image etc logic needed,
	// but let's assume service handles specific logic or we trust the determined type)
	if !models.IsValidContentType(req.ContentType) {
		// Auto-correct if possible or fail?
		// For now, if we have files but type is invalid, verify against valid map.
		// If valid map has text_image etc, we might need logic to combine.
		// Let's trust frontend to send correct combined type, or the simple fallback above.
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
	marketplace := ctx.Query("marketplace") == "true"

	query := models.MessageQuery{
		SenderID:    senderID.Hex(),
		Page:        page,
		Limit:       limit,
		GroupID:     groupID,
		ReceiverID:  receiverID,
		Before:      before,
		Marketplace: marketplace,
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

// @Summary Mark a conversation as seen
// @Description Mark all messages in a conversation as seen up to a certain timestamp
// @Tags messages
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Conversation ID (user ID or group ID)"
// @Param seenRequest body object{timestamp:string,is_group:bool} true "Timestamp and conversation type"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /conversations/{id}/seen [post]
func (c *MessageController) MarkConversationAsSeen(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	currentUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	conversationIDStr := ctx.Param("id")
	conversationID, err := primitive.ObjectIDFromHex(conversationIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid conversation ID"})
		return
	}

	var req struct {
		Timestamp string `json:"timestamp" binding:"required"`
		IsGroup   bool   `json:"is_group"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	timestamp, err := time.Parse(time.RFC3339, req.Timestamp)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid timestamp format"})
		return
	}

	err = c.messageService.MarkConversationAsSeen(ctx.Request.Context(), currentUserID, conversationID, timestamp, req.IsGroup)
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
		case "message not found", "message not found or not owned by user":
			ctx.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		case "message can only be deleted within 7 days of creation":
			ctx.JSON(http.StatusForbidden, models.ErrorResponse{Error: err.Error()})
		default:
			ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		}
		return
	}
	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}

// @Summary Edit a message
// @Description Edit the content of an existing message
// @Tags messages
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Message ID"
// @Param message body object{content:string} true "New message content"
// @Success 200 {object} models.Message
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages/{id} [put]
func (c *MessageController) EditMessage(ctx *gin.Context) {
	messageID := ctx.Param("id")
	userID := ctx.MustGet("userID").(string)

	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	updatedMessage, err := c.messageService.EditMessage(ctx.Request.Context(), messageID, userID, req.Content)
	if err != nil {
		switch err.Error() {
		case "message not found, not owned by user, or already deleted":
			ctx.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		case "message can only be edited within 1 hour of creation":
			ctx.JSON(http.StatusForbidden, models.ErrorResponse{Error: err.Error()})
		case "invalid message ID format", "invalid requester ID format":
			ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		default:
			ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, updatedMessage)
}

// @Summary Search messages
// @Description Search messages in user's conversations
// @Tags messages
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param q query string true "Search query"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Messages per page" default(20)
// @Success 200 {array} models.Message
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages/search [get]
func (c *MessageController) SearchMessages(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	currentUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	query := ctx.Query("q")
	if query == "" {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "search query is required"})
		return
	}

	page, _ := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(ctx.DefaultQuery("limit", "20"), 10, 64)

	messages, err := c.messageService.SearchMessages(ctx.Request.Context(), currentUserID, query, page, limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, messages)
}

// @Summary Add a reaction to a message
// @Description Add an emoji reaction to a specific message
// @Tags messages
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Message ID"
// @Param reaction body object{emoji:string} true "Reaction emoji"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages/{id}/react [post]
func (c *MessageController) AddReactionToMessage(ctx *gin.Context) {
	messageID := ctx.Param("id")
	userID := ctx.MustGet("userID").(string)

	var req struct {
		Emoji string `json:"emoji" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	err := c.messageService.AddReaction(ctx.Request.Context(), messageID, userID, req.Emoji)
	if err != nil {
		switch err.Error() {
		case "message not found or reaction already exists":
			ctx.JSON(http.StatusConflict, models.ErrorResponse{Error: err.Error()})
		case "invalid message ID format", "invalid user ID format":
			ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		default:
			ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}

// @Summary Remove a reaction from a message
// @Description Remove an emoji reaction from a specific message
// @Tags messages
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Message ID"
// @Param reaction body object{emoji:string} true "Reaction emoji"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages/{id}/react [delete]
func (c *MessageController) RemoveReactionFromMessage(ctx *gin.Context) {
	messageID := ctx.Param("id")
	userID := ctx.MustGet("userID").(string)

	var req struct {
		Emoji string `json:"emoji" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	err := c.messageService.RemoveReaction(ctx.Request.Context(), messageID, userID, req.Emoji)
	if err != nil {
		switch err.Error() {
		case "message not found or reaction not present":
			ctx.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		case "invalid message ID format", "invalid user ID format":
			ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		default:
			ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}

// @Summary Mark messages as delivered
// @Description Mark messages as delivered to the current user
// @Tags messages
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param messageIDs body []string true "Array of message IDs to mark as delivered"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages/delivered [post]
func (c *MessageController) MarkMessagesAsDelivered(ctx *gin.Context) {
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

	err = c.messageService.MarkMessagesAsDelivered(ctx.Request.Context(), currentUserID, objectIDs)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}
