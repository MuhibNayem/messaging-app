package services

import (
	"context"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ReelService struct {
	reelRepo       *repositories.ReelRepository
	userRepo       *repositories.UserRepository
	friendshipRepo *repositories.FriendshipRepository
}

func NewReelService(reelRepo *repositories.ReelRepository, userRepo *repositories.UserRepository, friendshipRepo *repositories.FriendshipRepository) *ReelService {
	return &ReelService{
		reelRepo:       reelRepo,
		userRepo:       userRepo,
		friendshipRepo: friendshipRepo,
	}
}

func (s *ReelService) CreateReel(ctx context.Context, userID primitive.ObjectID, req *models.CreateReelRequest) (*models.Reel, error) {
	// Fetch user for author info
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	privacy := req.Privacy
	if privacy == "" {
		privacy = models.PrivacySettingPublic
	}

	reel := &models.Reel{
		UserID:         userID,
		VideoURL:       req.VideoURL,
		ThumbnailURL:   req.ThumbnailURL,
		Caption:        req.Caption,
		Duration:       req.Duration,
		Privacy:        privacy,
		AllowedViewers: req.AllowedViewers,
		BlockedViewers: req.BlockedViewers,
		Author: models.PostAuthor{
			ID:       user.ID.Hex(),
			Username: user.Username,
			Avatar:   user.Avatar,
			FullName: user.FullName,
		},
	}

	return s.reelRepo.CreateReel(ctx, reel)
}

// GetReelsFeed returns a list of reels for the user's feed with optimized DB filtering
func (s *ReelService) GetReelsFeed(ctx context.Context, userID primitive.ObjectID, limit, offset int64) ([]models.Reel, error) {
	// 1. Get user's friends to filter content (returns []User with IDs)
	friends, err := s.friendshipRepo.GetFriends(ctx, userID)
	if err != nil {
		return nil, err
	}

	friendIDs := make([]primitive.ObjectID, 0)
	for _, f := range friends {
		friendIDs = append(friendIDs, f.ID)
	}

	// 2. Fetch Reels with DB-level filtering
	return s.reelRepo.GetReelsFeed(ctx, userID, friendIDs, limit, offset)
}

func (s *ReelService) GetUserReels(ctx context.Context, userID primitive.ObjectID) ([]models.Reel, error) {
	return s.reelRepo.GetUserReels(ctx, userID)
}

func (s *ReelService) GetReel(ctx context.Context, reelID primitive.ObjectID) (*models.Reel, error) {
	return s.reelRepo.GetReelByID(ctx, reelID)
}

func (s *ReelService) DeleteReel(ctx context.Context, reelID primitive.ObjectID, userID primitive.ObjectID) error {
	return s.reelRepo.DeleteReel(ctx, reelID, userID)
}

func (s *ReelService) IncrementViews(ctx context.Context, reelID primitive.ObjectID, userID primitive.ObjectID) error {
	// Fetch reel to check author
	reel, err := s.reelRepo.GetReelByID(ctx, reelID)
	if err != nil {
		return err
	}

	// Don't count self-views
	if reel.UserID == userID {
		return nil
	}

	return s.reelRepo.IncrementViews(ctx, reelID)
}

// AddComment adds a comment to a reel
// AddComment adds a comment to a reel
func (s *ReelService) AddComment(ctx context.Context, reelID primitive.ObjectID, userID primitive.ObjectID, content string, explicitMentions []primitive.ObjectID) (*models.Comment, error) {
	// Fetch user for comment author
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Parse mentions from content and merge with explicit mentions
	parsedMentions := s.ParseMentions(content)

	// Deduplicate mentions
	mentionMap := make(map[string]primitive.ObjectID)
	for _, id := range parsedMentions {
		mentionMap[id.Hex()] = id
	}
	for _, id := range explicitMentions {
		mentionMap[id.Hex()] = id
	}

	var finalMentions []primitive.ObjectID
	for _, id := range mentionMap {
		finalMentions = append(finalMentions, id)
	}

	comment := models.Comment{
		ID:      primitive.NewObjectID(),
		UserID:  userID,
		Content: content,
		Author: models.PostAuthor{
			ID:       user.ID.Hex(),
			Username: user.Username,
			Avatar:   user.Avatar,
			FullName: user.FullName,
		},
		Mentions:  finalMentions,
		CreatedAt: time.Now(),
	}

	err = s.reelRepo.AddComment(ctx, reelID, comment)
	if err != nil {
		return nil, err
	}

	return &comment, nil
}

// GetComments retrieves all comments for a reel with pagination
func (s *ReelService) GetComments(ctx context.Context, reelID primitive.ObjectID, limit, offset int64) ([]models.Comment, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return s.reelRepo.GetComments(ctx, reelID, limit, offset)
}

// AddReply adds a reply to a comment on a reel
func (s *ReelService) AddReply(ctx context.Context, reelID primitive.ObjectID, commentID primitive.ObjectID, userID primitive.ObjectID, content string) (*models.Reply, error) {
	// Fetch user for reply author
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Parse mentions from content
	mentions := s.ParseMentions(content)

	reply := models.Reply{
		ID:        primitive.NewObjectID(),
		CommentID: commentID,
		UserID:    userID,
		Author: models.PostAuthor{
			ID:       user.ID.Hex(),
			Username: user.Username,
			Avatar:   user.Avatar,
			FullName: user.FullName,
		},
		Mentions:  mentions,
		Content:   content,
		CreatedAt: time.Now(),
	}

	err = s.reelRepo.AddReply(ctx, reelID, commentID, reply)
	if err != nil {
		return nil, err
	}

	return &reply, nil
}

// ReactToComment toggles a reaction on a comment
func (s *ReelService) ReactToComment(ctx context.Context, reelID primitive.ObjectID, commentID primitive.ObjectID, userID primitive.ObjectID, reactionType models.ReactionType) error {
	return s.reelRepo.ReactToComment(ctx, reelID, commentID, userID, reactionType)
}

// ParseMentions extracts @username mentions from content and validates them
func (s *ReelService) ParseMentions(content string) []primitive.ObjectID {
	// Regex for @username
	// Assuming usernames are alphanumeric + underscores, min 3 chars
	re := regexp.MustCompile(`@([a-zA-Z0-9_]{3,})`)
	matches := re.FindAllStringSubmatch(content, -1)

	uniqueUsernames := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			uniqueUsernames[match[1]] = true
		}
	}

	mentionIDs := make([]primitive.ObjectID, 0)
	for username := range uniqueUsernames {
		// Verify user exists
		user, err := s.userRepo.FindUserByUserName(context.Background(), username)
		if err == nil && user != nil {
			mentionIDs = append(mentionIDs, user.ID)
		}
	}

	return mentionIDs
}

// ReactToReel handles toggling a reaction on a reel
func (s *ReelService) ReactToReel(ctx context.Context, reelID primitive.ObjectID, userID primitive.ObjectID, reactionType models.ReactionType) error {
	// Check for existing reaction
	existingReaction, err := s.reelRepo.GetReaction(ctx, reelID, userID)
	if err != nil {
		return err
	}

	if existingReaction != nil {
		// Already reacted
		if existingReaction.Type == reactionType {
			// Same type -> Remove (Toggle OFF)
			return s.reelRepo.RemoveReaction(ctx, existingReaction)
		}

		// Different type -> Remove old, Add new (Change reaction)
		err = s.reelRepo.RemoveReaction(ctx, existingReaction)
		if err != nil {
			return err
		}
	}

	// Add new reaction
	newReaction := &models.Reaction{
		ID:         primitive.NewObjectID(),
		UserID:     userID,
		TargetID:   reelID,
		TargetType: "reel",
		Type:       reactionType,
		CreatedAt:  time.Now(),
	}
	// TODO: Send Notification (like in FeedService)
	// Notification logic can be added here similar to comments.

	return s.reelRepo.AddReaction(ctx, newReaction)
}
