package controllers

import (
	"context"
	"fmt"
	"messaging-app/internal/models"
	"messaging-app/internal/services"
	"messaging-app/pkg/utils"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type GroupController struct {
	groupService *services.GroupService
	userService  *services.UserService
}

func NewGroupController(groupService *services.GroupService, userService *services.UserService) *GroupController {
	return &GroupController{
		groupService: groupService,
		userService:  userService,
	}
}

// Request/Response structures
type CreateGroupRequest struct {
	Name      string   `json:"name" binding:"required,min=3,max=50"`
	MemberIDs []string `json:"member_ids" binding:"required,min=1,dive"`
	Avatar    string   `json:"avatar"`
}

type AddMemberRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

type UpdateGroupRequest struct {
	Name   string `json:"name" binding:"omitempty,min=3,max=50"`
	Avatar string `json:"avatar"`
}

// Handlers
func (c *GroupController) CreateGroup(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	var req CreateGroupRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	// Convert string IDs to ObjectIDs
	memberObjectIDs := make([]primitive.ObjectID, len(req.MemberIDs))
	for i, idStr := range req.MemberIDs {
		id, err := primitive.ObjectIDFromHex(idStr)
		if err != nil {
			utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid member ID format")
			return
		}
		memberObjectIDs[i] = id
	}

	group, err := c.groupService.CreateGroup(ctx, userID, req.Name, req.Avatar, memberObjectIDs)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	response, err := c.convertGroupToResponse(ctx, group)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusInternalServerError, "Failed to prepare response")
		return
	}

	ctx.JSON(http.StatusCreated, response)
}

func (c *GroupController) GetGroup(ctx *gin.Context) {
	groupID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid group ID")
		return
	}

	// SECURITY: Get current user ID and verify membership
	userIDValue, exists := ctx.Get("userID")
	if !exists {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "User not authenticated")
		return
	}
	userIDStr, ok := userIDValue.(string)
	if !ok {
		utils.RespondWithError(ctx, http.StatusInternalServerError, "Invalid user ID format in context")
		return
	}
	userID, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusInternalServerError, "Invalid user ID hex string")
		return
	}

	group, err := c.groupService.GetGroup(ctx, groupID)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	// SECURITY: Verify user is a member of this group (IDOR protection)
	isMember := false
	for _, memberID := range group.Members {
		if memberID == userID {
			isMember = true
			break
		}
	}
	if !isMember {
		utils.RespondWithError(ctx, http.StatusForbidden, "You are not a member of this group")
		return
	}

	response, err := c.convertGroupToResponse(ctx, group)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusInternalServerError, "Failed to prepare response")
		return
	}

	ctx.JSON(http.StatusOK, response)
}

func (c *GroupController) AddMember(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	groupID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid group ID")
		return
	}

	var req AddMemberRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	memberID, err := primitive.ObjectIDFromHex(req.UserID)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	if err := c.groupService.AddMember(ctx, groupID, userID, memberID); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (c *GroupController) AddAdmin(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	groupID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid group ID")
		return
	}

	var req AddMemberRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	adminID, err := primitive.ObjectIDFromHex(req.UserID)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	if err := c.groupService.AddAdmin(ctx, groupID, userID, adminID); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (c *GroupController) RemoveMember(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	groupID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid group ID")
		return
	}

	memberID, err := primitive.ObjectIDFromHex(ctx.Param("userId"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := c.groupService.RemoveMember(ctx, groupID, userID, memberID); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (c *GroupController) UpdateGroup(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	groupID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid group ID")
		return
	}

	var req UpdateGroupRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Avatar != "" {
		updates["avatar"] = req.Avatar
	}

	if len(updates) == 0 {
		utils.RespondWithError(ctx, http.StatusBadRequest, "No valid fields to update")
		return
	}

	if err := c.groupService.UpdateGroup(ctx, groupID, userID, updates); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	group, err := c.groupService.GetGroup(ctx, groupID)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	response, err := c.convertGroupToResponse(ctx, group)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusInternalServerError, "Failed to prepare response")
		return
	}

	ctx.JSON(http.StatusOK, response)
}

func (c *GroupController) GetUserGroups(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	groups, err := c.groupService.GetUserGroups(ctx, userID)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	responses := make([]models.GroupResponse, len(groups))
	for i, group := range groups {
		response, err := c.convertGroupToResponse(ctx, group)
		if err != nil {
			utils.RespondWithError(ctx, http.StatusInternalServerError, "Failed to prepare response")
			return
		}
		responses[i] = *response
	}

	ctx.JSON(http.StatusOK, responses)
}

type UpdateGroupSettingsRequest struct {
	RequiresApproval bool `json:"requires_approval"`
}

func (c *GroupController) InviteMember(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	groupID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid group ID")
		return
	}

	var req AddMemberRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	memberID, err := primitive.ObjectIDFromHex(req.UserID)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	if err := c.groupService.InviteMember(ctx, groupID, userID, memberID); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (c *GroupController) ApproveMember(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	groupID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid group ID")
		return
	}

	var req AddMemberRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	targetID, err := primitive.ObjectIDFromHex(req.UserID)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	if err := c.groupService.ApproveMember(ctx, groupID, userID, targetID); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (c *GroupController) RejectMember(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	groupID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid group ID")
		return
	}

	var req AddMemberRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	targetID, err := primitive.ObjectIDFromHex(req.UserID)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	if err := c.groupService.RejectMember(ctx, groupID, userID, targetID); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (c *GroupController) RemoveAdmin(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	groupID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid group ID")
		return
	}

	targetID, err := primitive.ObjectIDFromHex(ctx.Param("userId"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := c.groupService.RemoveAdmin(ctx, groupID, userID, targetID); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (c *GroupController) UpdateGroupSettings(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	groupID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid group ID")
		return
	}

	var req UpdateGroupSettingsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	settings := models.GroupSettings{
		RequiresApproval: req.RequiresApproval,
	}

	if err := c.groupService.UpdateGroupSettings(ctx, groupID, userID, settings); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (c *GroupController) GetActivities(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	groupID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid group ID")
		return
	}

	// Get limit from query params, default to 50
	limit := 50
	if limitStr := ctx.Query("limit"); limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
		if limit <= 0 || limit > 100 {
			limit = 50
		}
	}

	activities, err := c.groupService.GetActivities(ctx, groupID, userID, limit)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, activities)
}

// Helper methods
func (c *GroupController) convertGroupToResponse(ctx context.Context, group *models.Group) (*models.GroupResponse, error) {
	creator, err := c.userService.GetUserByID(ctx, group.CreatorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get creator details")
	}

	members := make([]models.UserShortResponse, len(group.Members))
	for i, memberID := range group.Members {
		user, err := c.userService.GetUserByID(ctx, memberID)
		if err != nil {
			return nil, fmt.Errorf("failed to get member details")
		}
		members[i] = models.UserShortResponse{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Avatar:   user.Avatar,
		}
	}

	pendingMembers := make([]models.UserShortResponse, len(group.PendingMembers))
	for i, memberID := range group.PendingMembers {
		user, err := c.userService.GetUserByID(ctx, memberID)
		if err == nil {
			pendingMembers[i] = models.UserShortResponse{
				ID:       user.ID,
				Username: user.Username,
				Email:    user.Email,
				Avatar:   user.Avatar,
			}
		}
	}

	admins := make([]models.UserShortResponse, len(group.Admins))
	for i, adminID := range group.Admins {
		user, err := c.userService.GetUserByID(ctx, adminID)
		if err != nil {
			return nil, fmt.Errorf("failed to get admin details")
		}
		admins[i] = models.UserShortResponse{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
		}
	}

	return &models.GroupResponse{
		ID:     group.ID,
		Name:   group.Name,
		Avatar: group.Avatar,
		Creator: models.UserShortResponse{
			ID:       creator.ID,
			Username: creator.Username,
			Email:    creator.Email,
		},
		Members:        members,
		PendingMembers: pendingMembers,
		Admins:         admins,
		Settings:       group.Settings,
		CreatedAt:      group.CreatedAt,
		UpdatedAt:      group.UpdatedAt,
	}, nil
}
