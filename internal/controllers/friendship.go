package controllers

import (
	"errors"
	"messaging-app/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type FriendshipController struct {
	friendshipService *services.FriendshipService
}

func NewFriendshipController(fs *services.FriendshipService) *FriendshipController {
	return &FriendshipController{fs}
}

type friendshipRequest struct {
	ReceiverID string `json:"receiver_id" binding:"required"`
}

type friendshipResponse struct {
	FriendshipID string `json:"friendship_id" binding:"required"`
	Accept       bool   `json:"accept"`
}

// @Summary Send friend request
// @Description Send a friend request to another user
// @Tags friendships
// @Accept json
// @Produce json
// @Param request body friendshipRequest true "Friend request details"
// @Success 201 {object} models.Friendship
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Router /friendships/requests [post]
func (c *FriendshipController) SendRequest(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	requesterID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req friendshipRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	receiverID, err := primitive.ObjectIDFromHex(req.ReceiverID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid receiver ID"})
		return
	}

	friendship, err := c.friendshipService.SendRequest(ctx.Request.Context(), requesterID, receiverID)
	if err != nil {
		status := http.StatusBadRequest
		if err == services.ErrCannotFriendSelf || err == services.ErrFriendRequestExists {
			status = http.StatusConflict
		}
		ctx.JSON(status, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, friendship)
}

// @Summary Respond to friend request
// @Description Accept or reject a friend request
// @Tags friendships
// @Accept json
// @Produce json
// @Param request body friendshipResponse true "Response details"
// @Success 200 {object} gin.H
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 403 {object} gin.H
// @Failure 404 {object} gin.H
// @Router /friendships/requests/respond [post]
func (c *FriendshipController) RespondToRequest(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	receiverID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req friendshipResponse
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	friendshipID, err := primitive.ObjectIDFromHex(req.FriendshipID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid friendship ID"})
		return
	}

	if err := c.friendshipService.RespondToRequest(ctx.Request.Context(), friendshipID, receiverID, req.Accept); err != nil {
		status := http.StatusBadRequest
		switch err {
		case services.ErrFriendRequestNotFound:
			status = http.StatusNotFound
		case services.ErrNotAuthorized:
			status = http.StatusForbidden
		}
		ctx.JSON(status, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"status": "success"})
}

// @Summary List friendships
// @Description Get a list of friendships with optional status filter
// @Tags friendships
// @Produce json
// @Param status query string false "Friendship status (pending/accepted/rejected)"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(10)
// @Success 200 {object} gin.H
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Router /friendships [get]
func (c *FriendshipController) ListFriendships(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	currentUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	status := ctx.Query("status")
	
	page, err := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.ParseInt(ctx.DefaultQuery("limit", "10"), 10, 64)
	if err != nil || limit < 1 {
		limit = 10
	}

	friendships, total, err := c.friendshipService.ListFriendships(ctx.Request.Context(), currentUserID, status, page, limit)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data":       friendships,
		"total":      total,
		"page":       page,
		"totalPages": (total + limit - 1) / limit,
	})
}

// @Summary Check friendship status
// @Description Check if two users are friends
// @Tags friendships
// @Produce json
// @Param other_user_id query string true "Other user ID to check"
// @Success 200 {object} gin.H
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Router /friendships/check [get]
func (c *FriendshipController) CheckFriendship(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	currentUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	otherUserID, err := primitive.ObjectIDFromHex(ctx.Query("other_user_id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid other user ID"})
		return
	}

	areFriends, err := c.friendshipService.CheckFriendship(ctx.Request.Context(), currentUserID, otherUserID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"are_friends": areFriends})
}

// @Summary Unfriend a user
// @Description Remove a friendship between two users
// @Tags friendships
// @Produce json
// @Param friend_id path string true "Friend ID to unfriend"
// @Success 200 {object} gin.H
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Router /friendships/{friend_id} [delete]
func (c *FriendshipController) Unfriend(ctx *gin.Context) {
    userID := ctx.MustGet("userID").(string)
    currentUserID, err := primitive.ObjectIDFromHex(userID)
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
        return
    }

    friendID, err := primitive.ObjectIDFromHex(ctx.Param("friend_id"))
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid friend ID"})
        return
    }

    if err := c.friendshipService.Unfriend(ctx.Request.Context(), currentUserID, friendID); err != nil {
        status := http.StatusBadRequest
        if errors.Is(err, services.ErrNotFriends) {
            status = http.StatusNotFound
        }
        ctx.JSON(status, gin.H{"error": err.Error()})
        return
    }

    ctx.JSON(http.StatusOK, gin.H{"status": "success"})
}

// @Summary Block a user
// @Description Block another user
// @Tags friendships
// @Accept json
// @Produce json
// @Param user_id path string true "User ID to block"
// @Success 200 {object} gin.H
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 409 {object} gin.H
// @Router /friendships/block/{user_id} [post]
func (c *FriendshipController) BlockUser(ctx *gin.Context) {
    userID := ctx.MustGet("userID").(string)
    blockerID, err := primitive.ObjectIDFromHex(userID)
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
        return
    }

    blockedID, err := primitive.ObjectIDFromHex(ctx.Param("user_id"))
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID to block"})
        return
    }

    if err := c.friendshipService.BlockUser(ctx.Request.Context(), blockerID, blockedID); err != nil {
        status := http.StatusBadRequest
        switch {
        case errors.Is(err, services.ErrCannotBlockSelf):
            status = http.StatusBadRequest
        case errors.Is(err, services.ErrAlreadyBlocked):
            status = http.StatusConflict
        }
        ctx.JSON(status, gin.H{"error": err.Error()})
        return
    }

    ctx.JSON(http.StatusOK, gin.H{"status": "success"})
}

// @Summary Unblock a user
// @Description Remove a block between users
// @Tags friendships
// @Produce json
// @Param user_id path string true "User ID to unblock"
// @Success 200 {object} gin.H
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Router /friendships/block/{user_id} [delete]
func (c *FriendshipController) UnblockUser(ctx *gin.Context) {
    userID := ctx.MustGet("userID").(string)
    blockerID, err := primitive.ObjectIDFromHex(userID)
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
        return
    }

    blockedID, err := primitive.ObjectIDFromHex(ctx.Param("user_id"))
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID to unblock"})
        return
    }

    if err := c.friendshipService.UnblockUser(ctx.Request.Context(), blockerID, blockedID); err != nil {
        status := http.StatusBadRequest
        if errors.Is(err, services.ErrBlockNotFound) {
            status = http.StatusNotFound
        }
        ctx.JSON(status, gin.H{"error": err.Error()})
        return
    }

    ctx.JSON(http.StatusOK, gin.H{"status": "success"})
}

// @Summary Check if user is blocked
// @Description Check if a user is blocked by another user
// @Tags friendships
// @Produce json
// @Param user_id path string true "User ID to check block status"
// @Success 200 {object} gin.H
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Router /friendships/block/{user_id}/status [get]
func (c *FriendshipController) IsBlocked(ctx *gin.Context) {
    userID := ctx.MustGet("userID").(string)
    currentUserID, err := primitive.ObjectIDFromHex(userID)
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
        return
    }

    otherUserID, err := primitive.ObjectIDFromHex(ctx.Param("user_id"))
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid other user ID"})
        return
    }

    isBlocked, err := c.friendshipService.IsBlocked(ctx.Request.Context(), currentUserID, otherUserID)
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    ctx.JSON(http.StatusOK, gin.H{"is_blocked": isBlocked})
}

// @Summary Get blocked users list
// @Description Get a list of users blocked by the current user
// @Tags friendships
// @Produce json
// @Success 200 {object} gin.H
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Router /friendships/blocked [get]
func (c *FriendshipController) GetBlockedUsers(ctx *gin.Context) {
    userID := ctx.MustGet("userID").(string)
    currentUserID, err := primitive.ObjectIDFromHex(userID)
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
        return
    }

    blockedUsers, err := c.friendshipService.GetBlockedUsers(ctx.Request.Context(), currentUserID)
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    ctx.JSON(http.StatusOK, gin.H{"blocked_users": blockedUsers})
}
