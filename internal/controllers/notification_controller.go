package controllers

import (
	"errors"
	services "messaging-app/internal/notifications"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type NotificationController struct {
	notificationService *services.NotificationService
}

func NewNotificationController(ns *services.NotificationService) *NotificationController {
	return &NotificationController{notificationService: ns}
}

// ListNotifications godoc
// @Summary List notifications for the authenticated user
// @Description Get a paginated list of notifications for the current user, with optional read status filter.
// @Tags Notifications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Number of items per page" default(10)
// @Param read query bool false "Filter by read status (true for read, false for unread)"
// @Success 200 {object} models.NotificationListResponse
// @Failure 400 {object} gin.H{"error":string}
// @Failure 401 {object} gin.H{"error":string}
// @Failure 500 {object} gin.H{"error":string}
// @Router /notifications [get]
func (c *NotificationController) ListNotifications(ctx *gin.Context) {
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user ID not found in context"})
		return
	}
	objUserID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID format"})
		return
	}

	pageStr := ctx.DefaultQuery("page", "1")
	limitStr := ctx.DefaultQuery("limit", "10")

	page, err := strconv.ParseInt(pageStr, 10, 64)
	if err != nil || page < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid page number"})
		return
	}

	limit, err := strconv.ParseInt(limitStr, 10, 64)
	if err != nil || limit < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit number"})
		return
	}

	var readStatus *bool
	if readStr := ctx.Query("read"); readStr != "" {
		parsedRead, err := strconv.ParseBool(readStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid read status value"})
			return
		}
		readStatus = &parsedRead
	}

	notifications, err := c.notificationService.ListNotifications(ctx.Request.Context(), objUserID, page, limit, readStatus)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, notifications)
}

// MarkNotificationAsRead godoc
// @Summary Mark a notification as read
// @Description Mark a specific notification as read for the authenticated user.
// @Tags Notifications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Notification ID"
// @Success 200 {object} gin.H{"success":bool}
// @Failure 400 {object} gin.H{"error":string}
// @Failure 401 {object} gin.H{"error":string}
// @Failure 403 {object} gin.H{"error":string}
// @Failure 404 {object} gin.H{"error":string}
// @Failure 500 {object} gin.H{"error":string}
// @Router /notifications/{id}/read [put]
func (c *NotificationController) MarkNotificationAsRead(ctx *gin.Context) {
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user ID not found in context"})
		return
	}
	objUserID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID format"})
		return
	}

	notificationID := ctx.Param("id")
	objNotificationID, err := primitive.ObjectIDFromHex(notificationID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification ID format"})
		return
	}

	err = c.notificationService.MarkNotificationAsRead(ctx.Request.Context(), objNotificationID, objUserID)
	if err != nil {
		if errors.Is(err, errors.New("notification not found")) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, errors.New("unauthorized to mark this notification as read")) {
			ctx.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// GetUnreadNotificationCount godoc
// @Summary Get unread notification count
// @Description Get the total count of unread notifications for the authenticated user.
// @Tags Notifications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} gin.H{"count":int}
// @Failure 401 {object} gin.H{"error":string}
// @Failure 500 {object} gin.H{"error":string}
// @Router /notifications/unread [get]
func (c *NotificationController) GetUnreadNotificationCount(ctx *gin.Context) {
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user ID not found in context"})
		return
	}
	objUserID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID format"})
		return
	}

	// For now, we'll just list all notifications and filter for unread count
	// A more optimized approach would be to have a dedicated service method for this
	notifications, err := c.notificationService.ListNotifications(ctx.Request.Context(), objUserID, 1, 1000000, nil) // Fetch all for count
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	unreadCount := 0
	for _, n := range notifications.Notifications {
		if !n.Read {
			unreadCount++
		}
	}

	ctx.JSON(http.StatusOK, gin.H{"count": unreadCount})
}
