package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"gitlab.com/spydotech-group/shared-entity/models"
	"messaging-app/internal/services"
)

type PrivacyController struct {
	privacyService *services.PrivacyService
	userService    *services.UserService
}

func NewPrivacyController(
	privacyService *services.PrivacyService,
	userService *services.UserService,
) *PrivacyController {
	return &PrivacyController{
		privacyService: privacyService,
		userService:    userService,
	}
}

// GetUserPrivacySettings godoc
// @Summary Get current user's privacy settings
// @Security BearerAuth
// @Tags privacy
// @Produce json
// @Success 200 {object} models.UserPrivacySettings
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /privacy/settings [get]
func (c *PrivacyController) GetUserPrivacySettings(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	settings, err := c.privacyService.GetUserPrivacySettings(ctx.Request.Context(), objUserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, settings)
}

// UpdateUserPrivacySettings godoc
// @Summary Update current user's privacy settings
// @Security BearerAuth
// @Tags privacy
// @Accept json
// @Produce json
// @Param settings body models.UpdatePrivacySettingsRequest true "Privacy settings update data"
// @Success 200 {object} models.UserPrivacySettings
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /privacy/settings [put]
func (c *PrivacyController) UpdateUserPrivacySettings(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	var req models.UpdatePrivacySettingsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	settings, err := c.privacyService.UpdateUserPrivacySettings(ctx.Request.Context(), objUserID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, settings)
}

// CreateCustomPrivacyList godoc
// @Summary Create a new custom privacy list
// @Security BearerAuth
// @Tags privacy
// @Accept json
// @Produce json
// @Param list body models.CreateCustomPrivacyListRequest true "Custom privacy list creation data"
// @Success 201 {object} models.CustomPrivacyList
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /privacy/lists [post]
func (c *PrivacyController) CreateCustomPrivacyList(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	var req models.CreateCustomPrivacyListRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	list, err := c.privacyService.CreateCustomPrivacyList(ctx.Request.Context(), objUserID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, list)
}

// GetCustomPrivacyListByID godoc
// @Summary Get a custom privacy list by ID
// @Security BearerAuth
// @Tags privacy
// @Produce json
// @Param id path string true "Custom Privacy List ID"
// @Success 200 {object} models.CustomPrivacyList
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /privacy/lists/{id} [get]
func (c *PrivacyController) GetCustomPrivacyListByID(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	listID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid list ID"})
		return
	}

	list, err := c.privacyService.GetCustomPrivacyListByID(ctx.Request.Context(), listID, objUserID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "custom privacy list not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "not authorized to view this custom privacy list" {
			statusCode = http.StatusForbidden
		}
		ctx.JSON(statusCode, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, list)
}

// GetCustomPrivacyListsByUserID godoc
// @Summary Get all custom privacy lists for the current user
// @Security BearerAuth
// @Tags privacy
// @Produce json
// @Success 200 {array} models.CustomPrivacyList
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /privacy/lists [get]
func (c *PrivacyController) GetCustomPrivacyListsByUserID(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	lists, err := c.privacyService.GetCustomPrivacyListsByUserID(ctx.Request.Context(), objUserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, lists)
}

// UpdateCustomPrivacyList godoc
// @Summary Update an existing custom privacy list
// @Security BearerAuth
// @Tags privacy
// @Accept json
// @Produce json
// @Param id path string true "Custom Privacy List ID"
// @Param list body models.UpdateCustomPrivacyListRequest true "Custom privacy list update data"
// @Success 200 {object} models.CustomPrivacyList
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /privacy/lists/{id} [put]
func (c *PrivacyController) UpdateCustomPrivacyList(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	listID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid list ID"})
		return
	}

	var req models.UpdateCustomPrivacyListRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	updatedList, err := c.privacyService.UpdateCustomPrivacyList(ctx.Request.Context(), listID, objUserID, &req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "custom privacy list not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "not authorized to update this custom privacy list" {
			statusCode = http.StatusForbidden
		}
		ctx.JSON(statusCode, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, updatedList)
}

// DeleteCustomPrivacyList godoc
// @Summary Delete a custom privacy list
// @Security BearerAuth
// @Tags privacy
// @Produce json
// @Param id path string true "Custom Privacy List ID"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /privacy/lists/{id} [delete]
func (c *PrivacyController) DeleteCustomPrivacyList(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	listID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid list ID"})
		return
	}

	err = c.privacyService.DeleteCustomPrivacyList(ctx.Request.Context(), listID, objUserID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "custom privacy list not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "not authorized to delete this custom privacy list" {
			statusCode = http.StatusForbidden
		}
		ctx.JSON(statusCode, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}

// AddMemberToCustomPrivacyList godoc
// @Summary Add a member to a custom privacy list
// @Security BearerAuth
// @Tags privacy
// @Accept json
// @Produce json
// @Param id path string true "Custom Privacy List ID"
// @Param member body models.AddRemoveCustomPrivacyListMemberRequest true "Member to add"
// @Success 200 {object} models.CustomPrivacyList
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /privacy/lists/{id}/members [post]
func (c *PrivacyController) AddMemberToCustomPrivacyList(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	listID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid list ID"})
		return
	}

	var req models.AddRemoveCustomPrivacyListMemberRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	updatedList, err := c.privacyService.AddMemberToCustomPrivacyList(ctx.Request.Context(), listID, objUserID, req.UserID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "custom privacy list not found" || err.Error() == "member user not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "not authorized to modify this custom privacy list" {
			statusCode = http.StatusForbidden
		}
		ctx.JSON(statusCode, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, updatedList)
}

// RemoveMemberFromCustomPrivacyList godoc
// @Summary Remove a member from a custom privacy list
// @Security BearerAuth
// @Tags privacy
// @Produce json
// @Param id path string true "Custom Privacy List ID"
// @Param member_id path string true "Member User ID to remove"
// @Success 200 {object} models.CustomPrivacyList
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /privacy/lists/{id}/members/{member_id} [delete]
func (c *PrivacyController) RemoveMemberFromCustomPrivacyList(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user ID"})
		return
	}

	listID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid list ID"})
		return
	}

	memberUserID, err := primitive.ObjectIDFromHex(ctx.Param("member_id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid member user ID"})
		return
	}

	updatedList, err := c.privacyService.RemoveMemberFromCustomPrivacyList(ctx.Request.Context(), listID, objUserID, memberUserID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "custom privacy list not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "not authorized to modify this custom privacy list" {
			statusCode = http.StatusForbidden
		}
		ctx.JSON(statusCode, models.ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, updatedList)
}
