package services

import (
	"context"
	"errors"
	"strings"

	"gitlab.com/spydotech-group/shared-entity/models"
	"messaging-app/internal/repositories"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type CommunityService struct {
	communityRepo *repositories.CommunityRepository
	userRepo      *repositories.UserRepository
}

func NewCommunityService(communityRepo *repositories.CommunityRepository, userRepo *repositories.UserRepository) *CommunityService {
	return &CommunityService{
		communityRepo: communityRepo,
		userRepo:      userRepo,
	}
}

func (s *CommunityService) CreateCommunity(ctx context.Context, userID primitive.ObjectID, req models.CreateCommunityRequest) (*models.Community, error) {
	// Generate slug from name
	slug := strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))

	// Default to visible if private and not specified, or whatever
	visibility := models.CommunityVisibilityVisible // default
	if req.Visibility != "" {
		visibility = req.Visibility
	}

	community := &models.Community{
		Name:        req.Name,
		Description: req.Description,
		Slug:        slug,
		Category:    req.Category,
		Privacy:     req.Privacy,
		Visibility:  visibility,
		CreatorID:   userID,
		Members:     []primitive.ObjectID{userID},
		Admins:      []primitive.ObjectID{userID},
		Settings: models.CommunitySettings{
			RequirePostApproval: req.RequirePostApproval,
			RequireJoinApproval: req.RequireJoinApproval,
			AllowMemberPosts:    true, // Default true
		},
	}

	err := s.communityRepo.Create(ctx, community)
	if err != nil {
		return nil, err
	}

	return community, nil
}

func (s *CommunityService) GetCommunity(ctx context.Context, communityID primitive.ObjectID) (*models.Community, error) {
	return s.communityRepo.GetByID(ctx, communityID)
}

func (s *CommunityService) JoinCommunity(ctx context.Context, communityID, userID primitive.ObjectID) error {
	community, err := s.communityRepo.GetByID(ctx, communityID)
	if err != nil {
		return err
	}

	// Check if already a member
	for _, memberID := range community.Members {
		if memberID == userID {
			return errors.New("already a member")
		}
	}

	// Check if pending
	for _, pendingID := range community.PendingMembers {
		if pendingID == userID {
			return errors.New("request already pending")
		}
	}

	// If Hidden, can't join without invite? Or maybe they found the link.
	// Assume if they have the ID, they can Request to Join if allowed.

	if community.Settings.RequireJoinApproval || community.Privacy == models.CommunityPrivacyPrivate {
		// Private groups usually require approval unless explicitly disabled
		// actually user said "does not it include member approval for private group ?"
		// so yes, private = approval needed typically.
		return s.communityRepo.AddPendingMember(ctx, communityID, userID)
	}

	return s.communityRepo.AddMember(ctx, communityID, userID)
}

func (s *CommunityService) LeaveCommunity(ctx context.Context, communityID, userID primitive.ObjectID) error {
	community, err := s.communityRepo.GetByID(ctx, communityID)
	if err != nil {
		return err
	}

	if community.CreatorID == userID {
		return errors.New("creator cannot leave the community")
	}

	return s.communityRepo.RemoveMember(ctx, communityID, userID)
}

func (s *CommunityService) ApproveMember(ctx context.Context, communityID, actorID, targetID primitive.ObjectID) error {
	community, err := s.communityRepo.GetByID(ctx, communityID)
	if err != nil {
		return err
	}

	// Check if actor is admin
	isAdmin := false
	for _, adminID := range community.Admins {
		if adminID == actorID {
			isAdmin = true
			break
		}
	}
	if !isAdmin {
		return errors.New("unauthorized: only admins can approve members")
	}

	// Check if target is in pending list
	isPending := false
	for _, pendingID := range community.PendingMembers {
		if pendingID == targetID {
			isPending = true
			break
		}
	}
	if !isPending {
		return errors.New("user is not in pending list")
	}

	if err := s.communityRepo.RemovePendingMember(ctx, communityID, targetID); err != nil {
		return err
	}
	return s.communityRepo.AddMember(ctx, communityID, targetID)
}

func (s *CommunityService) RejectMember(ctx context.Context, communityID, actorID, targetID primitive.ObjectID) error {
	community, err := s.communityRepo.GetByID(ctx, communityID)
	if err != nil {
		return err
	}

	// Check if actor is admin
	isAdmin := false
	for _, adminID := range community.Admins {
		if adminID == actorID {
			isAdmin = true
			break
		}
	}
	if !isAdmin {
		return errors.New("unauthorized")
	}

	return s.communityRepo.RemovePendingMember(ctx, communityID, targetID)
}

func (s *CommunityService) ListCommunities(ctx context.Context, userID primitive.ObjectID, limit, page int64, query string) ([]models.CommunityResponse, int64, error) {
	var communities []models.Community
	var total int64
	var err error

	if query != "" {
		communities, total, err = s.communityRepo.Search(ctx, query, limit, page)
	} else {
		communities, total, err = s.communityRepo.List(ctx, limit, page)
	}

	if err != nil {
		return nil, 0, err
	}

	responses := make([]models.CommunityResponse, len(communities))
	for i, community := range communities {
		responses[i] = *s.mapToResponse(&community, userID)
	}

	// Filter Hidden communities if user is NOT a member
	// Although repo Search excludes hidden, repo List might include Visibles
	// We double check here or trust repo. Repo Search excluded hidden.
	// But List included Public || Visible. Hidden is excluded.
	// So we are good.

	return responses, total, nil
}

func (s *CommunityService) GetUserCommunities(ctx context.Context, userID primitive.ObjectID) ([]models.Community, error) {
	return s.communityRepo.GetUserCommunities(ctx, userID)
}

func (s *CommunityService) UpdateSettings(ctx context.Context, communityID, userID primitive.ObjectID, req models.UpdateCommunityRequest) error {
	community, err := s.communityRepo.GetByID(ctx, communityID)
	if err != nil {
		return err
	}

	// Check if admin
	isAdmin := false
	for _, adminID := range community.Admins {
		if adminID == userID {
			isAdmin = true
			break
		}
	}
	if !isAdmin {
		return errors.New("unauthorized")
	}

	if req.Name != "" {
		community.Name = req.Name
		community.Slug = strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))
	}
	if req.Description != "" {
		community.Description = req.Description
	}
	if req.Category != "" {
		community.Category = req.Category
	}
	if req.Avatar != "" {
		community.Avatar = req.Avatar
	}
	if req.CoverImage != "" {
		community.CoverImage = req.CoverImage
	}
	if req.Privacy != "" {
		community.Privacy = req.Privacy
	}
	if req.Visibility != "" {
		community.Visibility = req.Visibility
	}
	// Settings
	if req.RequirePostApproval != nil {
		community.Settings.RequirePostApproval = *req.RequirePostApproval
	}
	if req.RequireJoinApproval != nil {
		community.Settings.RequireJoinApproval = *req.RequireJoinApproval
	}
	if req.AllowMemberPosts != nil {
		community.Settings.AllowMemberPosts = *req.AllowMemberPosts
	}
	if req.ShowGroupAffiliation != nil {
		community.Settings.ShowGroupAffiliation = *req.ShowGroupAffiliation
	}
	if req.Rules != nil {
		community.Rules = req.Rules
	}
	if req.MembershipQuestions != nil {
		community.MembershipQuestions = req.MembershipQuestions
	}

	return s.communityRepo.Update(ctx, community)
}

func (s *CommunityService) IsMember(ctx context.Context, communityID, userID primitive.ObjectID) (bool, error) {
	community, err := s.communityRepo.GetByID(ctx, communityID)
	if err != nil {
		return false, err
	}
	for _, id := range community.Members {
		if id == userID {
			return true, nil
		}
	}
	return false, nil
}

func (s *CommunityService) IsAdmin(ctx context.Context, communityID, userID primitive.ObjectID) (bool, error) {
	community, err := s.communityRepo.GetByID(ctx, communityID)
	if err != nil {
		return false, err
	}
	for _, id := range community.Admins {
		if id == userID {
			return true, nil
		}
	}
	return false, nil
}

func (s *CommunityService) GetDetailedCommunityResponse(ctx context.Context, communityID, userID primitive.ObjectID) (*models.CommunityResponse, error) {
	community, err := s.communityRepo.GetByID(ctx, communityID)
	if err != nil {
		return nil, err
	}

	return s.mapToResponse(community, userID), nil
}

func (s *CommunityService) mapToResponse(community *models.Community, userID primitive.ObjectID) *models.CommunityResponse {
	isMember := false
	for _, id := range community.Members {
		if id == userID {
			isMember = true
			break
		}
	}

	isAdmin := false
	for _, id := range community.Admins {
		if id == userID {
			isAdmin = true
			break
		}
	}

	isPending := false
	for _, id := range community.PendingMembers {
		if id == userID {
			isPending = true
			break
		}
	}

	return &models.CommunityResponse{
		ID:                  community.ID.Hex(),
		Name:                community.Name,
		Description:         community.Description,
		Slug:                community.Slug,
		Category:            community.Category,
		Avatar:              community.Avatar,
		CoverImage:          community.CoverImage,
		Privacy:             community.Privacy,
		Visibility:          community.Visibility,
		Settings:            community.Settings,
		Rules:               community.Rules,
		MembershipQuestions: community.MembershipQuestions,
		Stats:               community.Stats,
		IsMember:            isMember,
		IsAdmin:             isAdmin,
		IsPending:           isPending,
		CreatedAt:           community.CreatedAt,
	}
}

func (s *CommunityService) GetMembers(ctx context.Context, communityID, viewerID primitive.ObjectID, limit, page int64) ([]models.User, int64, error) {
	community, err := s.communityRepo.GetByID(ctx, communityID)
	if err != nil {
		return nil, 0, err
	}

	if community.Privacy == models.CommunityPrivacyPrivate {
		isMember, err := s.IsMember(ctx, communityID, viewerID)
		if err != nil {
			return nil, 0, err
		}
		if !isMember {
			return nil, 0, errors.New("unauthorized: cannot view members of private community")
		}
	}

	return s.communityRepo.GetMembers(ctx, communityID, limit, page)
}

func (s *CommunityService) GetAdmins(ctx context.Context, communityID primitive.ObjectID) ([]models.User, error) {
	return s.communityRepo.GetAdmins(ctx, communityID)
}

func (s *CommunityService) GetPendingMembers(ctx context.Context, communityID, userID primitive.ObjectID, limit, page int64) ([]models.User, int64, error) {
	isAdmin, err := s.IsAdmin(ctx, communityID, userID)
	if err != nil {
		return nil, 0, err
	}
	if !isAdmin {
		return nil, 0, errors.New("unauthorized: only admins can view pending members")
	}

	return s.communityRepo.GetPendingMembers(ctx, communityID, limit, page)
}
