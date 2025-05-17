package controllers

import (
	"context"
	"messaging-app/internal/models"
	"messaging-app/internal/services"
	"messaging-app/pkg/utils"
	"net/http"
	"time"
	"fmt"

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
	MemberIDs []string `json:"member_ids" binding:"required,min=1,dive,objectid"`
}

type GroupResponse struct {
	ID        primitive.ObjectID  `json:"id"`
	Name      string              `json:"name"`
	Creator   UserShortResponse   `json:"creator"`
	Members   []UserShortResponse `json:"members"`
	Admins    []UserShortResponse `json:"admins"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

type UserShortResponse struct {
	ID       primitive.ObjectID `json:"id"`
	Username string             `json:"username"`
	Email    string             `json:"email"`
}

type AddMemberRequest struct {
	UserID string `json:"user_id" binding:"required,objectid"`
}

type UpdateGroupRequest struct {
	Name string `json:"name" binding:"omitempty,min=3,max=50"`
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

	group, err := c.groupService.CreateGroup(ctx, userID, req.Name, memberObjectIDs)
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

	memberID, err := primitive.ObjectIDFromHex(ctx.Param("user_id"))
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

	responses := make([]GroupResponse, len(groups))
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

// Helper methods
func (c *GroupController) convertGroupToResponse(ctx context.Context, group *models.Group) (*GroupResponse, error) {
	creator, err := c.userService.GetUserByID(ctx, group.CreatorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get creator details")
	}

	members := make([]UserShortResponse, len(group.Members))
	for i, memberID := range group.Members {
		user, err := c.userService.GetUserByID(ctx, memberID)
		if err != nil {
			return nil, fmt.Errorf("failed to get member details")
		}
		members[i] = UserShortResponse{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
		}
	}

	admins := make([]UserShortResponse, len(group.Admins))
	for i, adminID := range group.Admins {
		user, err := c.userService.GetUserByID(ctx, adminID)
		if err != nil {
			return nil, fmt.Errorf("failed to get admin details")
		}
		admins[i] = UserShortResponse{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
		}
	}

	return &GroupResponse{
		ID:        group.ID,
		Name:      group.Name,
		Creator: UserShortResponse{
			ID:       creator.ID,
			Username: creator.Username,
			Email:    creator.Email,
		},
		Members:   members,
		Admins:    admins,
		CreatedAt: group.CreatedAt,
		UpdatedAt: group.UpdatedAt,
	}, nil
}