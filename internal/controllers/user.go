package controllers

import (
	"messaging-app/internal/models"
	"messaging-app/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type UserController struct {
	userService *services.UserService
}

func NewUserController(userService *services.UserService) *UserController {
	return &UserController{userService: userService}
}

// GetUser godoc
// @Summary Get current user profile
// @Security BearerAuth
// @Tags users
// @Produce json
// @Success 200 {object} models.User
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Router /api/user [get]
func (c *UserController) GetUser(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	user, err := c.userService.GetUserByID(ctx.Request.Context(), objID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	userDTO := models.User{
        ID:        user.ID,
        Username:  user.Username,
		Email: 	   user.Email,
        Avatar:    user.Avatar,
        CreatedAt: user.CreatedAt,
		Friends:   user.Friends,
		Blocked:   user.Blocked,
    }
	ctx.JSON(http.StatusOK, userDTO)
}

// GetUserByID godoc
// @Summary Get user by ID
// @Security BearerAuth
// @Tags users
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} models.PublicUser
// @Failure 400 {object} gin.H
// @Failure 404 {object} gin.H
// @Router /api/users/{id} [get]
func (c *UserController) GetUserByID(ctx *gin.Context) {
    userID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
        return
    }

    user, err := c.userService.GetUserByID(ctx.Request.Context(), userID)
    if err != nil {
        ctx.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
        return
    }

    publicUser := models.User{
        ID:        user.ID,
        Username:  user.Username,
        Avatar:    user.Avatar,
        CreatedAt: user.CreatedAt,
    }
    ctx.JSON(http.StatusOK, publicUser)
}

// UpdateUser godoc
// @Summary Update user profile
// @Security BearerAuth
// @Tags users
// @Accept json
// @Produce json
// @Param body body models.UserUpdateRequest true "Update data"
// @Success 200 {object} models.User
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Router /api/user [put]
func (c *UserController) UpdateUser(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	
	var updateReq models.UserUpdateRequest
	if err := ctx.ShouldBindJSON(&updateReq); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	updatedUser, err := c.userService.UpdateUser(ctx.Request.Context(), objID, &updateReq)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, updatedUser)
}

// ListUsers godoc
// @Summary List users (paginated)
// @Security BearerAuth
// @Tags users
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Param search query string false "Search query"
// @Success 200 {object} models.UserListResponse
// @Failure 400 {object} gin.H
// @Router /api/users [get]
func (c *UserController) ListUsers(ctx *gin.Context) {
	page, _ := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(ctx.DefaultQuery("limit", "20"), 10, 64)
	search := ctx.Query("search")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	response, err := c.userService.ListUsers(ctx.Request.Context(), page, limit, search)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, response)
}

// UpdateEmail godoc
// @Summary Update user's email address
// @Security BearerAuth
// @Tags users
// @Accept json
// @Produce json
// @Param body body models.UpdateEmailRequest true "New email address"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/user/email [put]
func (c *UserController) UpdateEmail(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req models.UpdateEmailRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return	
	}

	err = c.userService.UpdateEmail(ctx.Request.Context(), objID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}

// UpdatePassword godoc
// @Summary Update user's password
// @Security BearerAuth
// @Tags users
// @Accept json
// @Produce json
// @Param body body models.UpdatePasswordRequest true "Current and new password"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/user/password [put]
func (c *UserController) UpdatePassword(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req models.UpdatePasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = c.userService.UpdatePassword(ctx.Request.Context(), objID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}

// ToggleTwoFactor godoc
// @Summary Enable or disable two-factor authentication
// @Security BearerAuth
// @Tags users
// @Accept json
// @Produce json
// @Param body body models.ToggleTwoFactorRequest true "Enable/disable 2FA"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/user/2fa [put]
func (c *UserController) ToggleTwoFactor(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req models.ToggleTwoFactorRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = c.userService.ToggleTwoFactor(ctx.Request.Context(), objID, req.Enabled)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}

// DeactivateAccount godoc
// @Summary Deactivate user account
// @Security BearerAuth
// @Tags users
// @Produce json
// @Success 200 {object} models.SuccessResponse
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/user/deactivate [put]
func (c *UserController) DeactivateAccount(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	err = c.userService.DeactivateAccount(ctx.Request.Context(), objID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}

// UpdatePrivacySettings godoc
// @Summary Update user privacy settings
// @Security BearerAuth
// @Tags users
// @Accept json
// @Produce json
// @Param body body models.UpdatePrivacySettingsRequest true "Privacy settings update data"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/user/privacy [put]
func (c *UserController) UpdatePrivacySettings(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req models.UpdatePrivacySettingsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = c.userService.UpdatePrivacySettings(ctx.Request.Context(), objID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}