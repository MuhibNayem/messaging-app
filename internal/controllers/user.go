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