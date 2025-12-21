package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"messaging-app/internal/kafka"
	"messaging-app/internal/models"
	notifications "messaging-app/internal/notifications"
	"messaging-app/internal/repositories"
	"messaging-app/pkg/utils"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type FeedService struct {
	feedRepo            *repositories.FeedRepository
	userRepo            *repositories.UserRepository
	friendshipRepo      *repositories.FriendshipRepository
	communityRepo       *repositories.CommunityRepository // Added
	privacyRepo         repositories.PrivacyRepository
	kafkaProducer       *kafka.MessageProducer
	notificationService *notifications.NotificationService
	storageService      *StorageService
}

func NewFeedService(feedRepo *repositories.FeedRepository, userRepo *repositories.UserRepository, friendshipRepo *repositories.FriendshipRepository, communityRepo *repositories.CommunityRepository, privacyRepo repositories.PrivacyRepository, kafkaProducer *kafka.MessageProducer, notificationService *notifications.NotificationService, storageService *StorageService) *FeedService {
	return &FeedService{feedRepo: feedRepo, userRepo: userRepo, friendshipRepo: friendshipRepo, communityRepo: communityRepo, privacyRepo: privacyRepo, kafkaProducer: kafkaProducer, notificationService: notificationService, storageService: storageService}
}

// Post operations
func (s *FeedService) CreatePost(ctx context.Context, userID primitive.ObjectID, req *models.CreatePostRequest) (*models.Post, error) {
	// Extract mentions from content
	mentionedUsernames := utils.ExtractMentions(req.Content)
	mentionedUsers, err := s.userRepo.FindUsersByUserNames(ctx, mentionedUsernames)
	if err != nil {
		// Log error but don't fail post creation if mentioned users are not found
		fmt.Printf("Failed to find mentioned users: %v\n", err)
	}
	// Use a map to deduplicate mentions
	uniqueMentions := make(map[string]primitive.ObjectID)
	for _, user := range mentionedUsers {
		uniqueMentions[user.ID.Hex()] = user.ID
	}

	// Merge explicitly tagged users with mentioned users from text
	for _, id := range req.Mentions {
		uniqueMentions[id.Hex()] = id
	}

	var mentionedUserIDs []primitive.ObjectID
	for _, id := range uniqueMentions {
		mentionedUserIDs = append(mentionedUserIDs, id)
	}

	var communityID *primitive.ObjectID
	if req.CommunityID != "" {
		id, err := primitive.ObjectIDFromHex(req.CommunityID)
		if err != nil {
			return nil, fmt.Errorf("invalid community ID: %w", err)
		}
		communityID = &id
	}

	// Check Community Logic
	status := models.PostStatusActive
	if communityID != nil {
		community, err := s.communityRepo.GetByID(ctx, *communityID)
		if err != nil {
			return nil, fmt.Errorf("failed to get community: %w", err)
		}

		// Check if member posts are allowed
		// Note provided schema says AllowMemberPosts in Settings, check if implemented in models
		if !community.Settings.AllowMemberPosts {
			// Check if user is admin
			isAdmin := false
			for _, adminID := range community.Admins {
				if adminID == userID {
					isAdmin = true
					break
				}
			}
			if !isAdmin {
				return nil, errors.New("members are not allowed to post in this community")
			}
		}

		if community.Settings.RequirePostApproval {
			// Check if user is admin (admins surely bypass approval)
			isAdmin := false
			for _, adminID := range community.Admins {
				if adminID == userID {
					isAdmin = true
					break
				}
			}
			if !isAdmin {
				status = models.PostStatusPending
			}
		}
	}

	post := &models.Post{
		UserID:         userID,
		Content:        req.Content,
		Media:          req.Media, // This is where the media is saved
		Location:       req.Location,
		Privacy:        req.Privacy,
		CommunityID:    communityID,
		CustomAudience: req.CustomAudience,
		Status:         status,                 // New Field
		Comments:       []models.Comment{},     // Initialize as empty array
		CommentIDs:     []primitive.ObjectID{}, // Initialize as empty array
		Mentions:       mentionedUserIDs,
		Hashtags:       req.Hashtags,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	createdPost, err := s.feedRepo.CreatePost(ctx, post)
	if err != nil {
		return nil, err
	}

	// Fetch sender's user details for notification content
	senderUser, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		fmt.Printf("Failed to find sender user %s for mention notification: %v\n", userID.Hex(), err)
		// Continue without notification if sender not found, or handle as appropriate
	}

	// Send notifications to mentioned users
	for _, mentionedUserID := range mentionedUserIDs {
		notificationReq := &models.CreateNotificationRequest{
			RecipientID: mentionedUserID,
			SenderID:    userID,
			Type:        models.NotificationTypeMention,
			TargetID:    createdPost.ID,
			TargetType:  "post",
			Content:     fmt.Sprintf("%s mentioned you in a post.", senderUser.Username),
		}
		_, err := s.notificationService.CreateNotification(ctx, notificationReq)
		if err != nil {
			// Log the error but don't block post creation
			fmt.Printf("Failed to create mention notification for user %s: %v\n", mentionedUserID.Hex(), err)
		}
	}

	// Publish PostCreated event to Kafka
	if senderUser != nil {
		createdPost.Author = models.PostAuthor{
			ID:       senderUser.ID.Hex(),
			Username: senderUser.Username,
			Avatar:   senderUser.Avatar,
			FullName: senderUser.FullName,
		}
	}

	postDataBytes, err := json.Marshal(createdPost)
	if err != nil {
		fmt.Printf("Failed to marshal createdPost for WebSocketEvent: %v\n", err)
		// Log the error but don't block post creation
	} else {
		wsEvent := models.WebSocketEvent{
			Type: "PostCreated",
			Data: postDataBytes,
		}
		eventBytes, err := json.Marshal(wsEvent)
		if err != nil {
			fmt.Printf("Failed to marshal WebSocketEvent for PostCreated: %v\n", err)
		} else {
			kafkaMsg := kafkago.Message{
				Key:   []byte(createdPost.UserID.Hex()), // Key for post events (using post owner ID)
				Value: eventBytes,
				Time:  time.Now(),
			}
			err = s.kafkaProducer.ProduceMessage(ctx, kafkaMsg)
			if err != nil {
				fmt.Printf("Failed to produce PostCreated WebSocketEvent to Kafka: %v\n", err)
				// Log the error but don't block post creation
			}
		}
	}

	// Populate MentionedUsers for the response
	var mentionedPostAuthors []models.PostAuthor
	for _, user := range mentionedUsers {
		mentionedPostAuthors = append(mentionedPostAuthors, models.PostAuthor{
			ID:       user.ID.Hex(),
			Username: user.Username,
			Avatar:   user.Avatar,
			FullName: user.FullName,
		})
	}
	createdPost.MentionedUsers = mentionedPostAuthors

	return createdPost, nil
}

func (s *FeedService) GetPostByID(ctx context.Context, viewerID, postID primitive.ObjectID) (*models.Post, error) {
	post, err := s.feedRepo.GetPostByID(ctx, postID)
	if err != nil {
		return nil, errors.New("post not found")
	}

	// Check privacy
	canView, err := s.canViewPost(ctx, viewerID, post)
	if err != nil {
		return nil, err
	}
	if !canView {
		return nil, errors.New("unauthorized to view this post")
	}

	return post, nil
}

// UpdatePostStatus updates the status of a post (e.g., for moderation)
func (s *FeedService) UpdatePostStatus(ctx context.Context, postID primitive.ObjectID, userID primitive.ObjectID, status models.PostStatus) error {
	post, err := s.feedRepo.GetPostByID(ctx, postID)
	if err != nil {
		return err
	}

	// Check if post belongs to a community
	if post.CommunityID == nil {
		return errors.New("post does not belong to a community")
	}

	// Check authorization: Must be Community Admin
	community, err := s.communityRepo.GetByID(ctx, *post.CommunityID)
	if err != nil {
		return fmt.Errorf("failed to get community: %w", err)
	}

	isAdmin := false
	for _, adminID := range community.Admins {
		if adminID == userID {
			isAdmin = true
			break
		}
	}

	if !isAdmin {
		return errors.New("unauthorized: only community admins can update post status")
	}

	// Update status
	_, err = s.feedRepo.UpdatePost(ctx, post.ID, bson.M{
		"status": status,
		// "updated_at": time.Now(), // Don't update this to avoid "Edited" label
	})
	return err
}

// canViewPost checks if a user has permission to view a post based on its privacy settings
func (s *FeedService) canViewPost(ctx context.Context, viewerID primitive.ObjectID, post *models.Post) (bool, error) {
	// Post owner can always view their own post
	if viewerID == post.UserID {
		return true, nil
	}

	switch post.Privacy {
	case models.PrivacySettingPublic:
		return true, nil
	case models.PrivacySettingFriends:
		// Check if viewer is friends with post owner
		isFriends, err := s.friendshipRepo.AreFriends(ctx, viewerID, post.UserID)
		if err != nil {
			return false, fmt.Errorf("failed to check friendship status: %w", err)
		}
		return isFriends, nil
	case models.PrivacySettingOnlyMe:
		// Only the post owner can view (already handled above)
		return false, nil
	default:
		return false, errors.New("unknown privacy setting")
	}
}

func (s *FeedService) UpdatePost(ctx context.Context, userID, postID primitive.ObjectID, req *models.UpdatePostRequest) (*models.Post, error) {
	post, err := s.feedRepo.GetPostByID(ctx, postID)
	if err != nil {
		return nil, errors.New("post not found")
	}
	if post.UserID != userID {
		return nil, errors.New("unauthorized to update this post")
	}

	updateData := bson.M{
		"updated_at": time.Now(),
	}

	if req.Content != "" {
		updateData["content"] = req.Content
	}
	if len(req.Media) > 0 {
		updateData["media"] = req.Media
	}
	if req.Privacy != "" {
		updateData["privacy"] = req.Privacy
	}
	if req.CustomAudience != nil {
		updateData["custom_audience"] = req.CustomAudience
	}
	if req.Mentions != nil {
		updateData["mentions"] = req.Mentions
	}
	if req.Hashtags != nil {
		updateData["hashtags"] = req.Hashtags
	}

	updatedPost, err := s.feedRepo.UpdatePost(ctx, postID, updateData)
	if err != nil {
		return nil, err
	}

	// Publish PostUpdated event to Kafka
	senderUser, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		fmt.Printf("Failed to find sender user %s for PostUpdate event: %v\n", userID.Hex(), err)
	} else {
		updatedPost.Author = models.PostAuthor{
			ID:       senderUser.ID.Hex(),
			Username: senderUser.Username,
			Avatar:   senderUser.Avatar,
			FullName: senderUser.FullName,
		}
	}

	postDataBytes, err := json.Marshal(updatedPost)
	if err != nil {
		fmt.Printf("Failed to marshal updatedPost for WebSocketEvent: %v\n", err)
	} else {
		wsEvent := models.WebSocketEvent{
			Type: "PostUpdated",
			Data: postDataBytes,
		}
		eventBytes, err := json.Marshal(wsEvent)
		if err != nil {
			fmt.Printf("Failed to marshal WebSocketEvent for PostUpdated: %v\n", err)
		} else {
			kafkaMsg := kafkago.Message{
				Key:   []byte(updatedPost.UserID.Hex()),
				Value: eventBytes,
				Time:  time.Now(),
			}
			err = s.kafkaProducer.ProduceMessage(ctx, kafkaMsg)
			if err != nil {
				fmt.Printf("Failed to produce PostUpdated WebSocketEvent to Kafka: %v\n", err)
			}
		}
	}

	return updatedPost, nil
}

func (s *FeedService) DeletePost(ctx context.Context, userID, postID primitive.ObjectID) error {
	// Fetch post before deletion to get details for event
	post, err := s.feedRepo.GetPostByID(ctx, postID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return errors.New("post not found")
		}
		return err
	}

	if post.UserID != userID {
		return errors.New("unauthorized to delete this post")
	}

	// 1. Cleanup related data (Cascade Delete)

	// A. Comments & Replies
	commentIDs, err := s.feedRepo.GetCommentIDsByPostID(ctx, postID)
	if err != nil {
		fmt.Printf("Failed to fetch comment IDs for post %s: %v\n", postID.Hex(), err)
	} else if len(commentIDs) > 0 {
		// 1. Fetch Reply IDs to delete their reactions
		replyIDs, err := s.feedRepo.GetReplyIDsByCommentIDs(ctx, commentIDs)
		if err != nil {
			fmt.Printf("Failed to fetch reply IDs: %v\n", err)
		}

		// 2. Delete Reactions on Replies
		if len(replyIDs) > 0 {
			if err := s.feedRepo.DeleteReactionsByTargetIDs(ctx, replyIDs); err != nil {
				fmt.Printf("Failed to delete reactions on replies: %v\n", err)
			}
		}

		// 3. Delete Replies
		if err := s.feedRepo.DeleteRepliesByCommentIDs(ctx, commentIDs); err != nil {
			fmt.Printf("Failed to delete replies: %v\n", err)
		}

		// 4. Delete Reactions on Comments
		if err := s.feedRepo.DeleteReactionsByTargetIDs(ctx, commentIDs); err != nil {
			fmt.Printf("Failed to delete reactions on comments: %v\n", err)
		}

		// 5. Delete Comments
		if err := s.feedRepo.DeleteCommentsByPostID(ctx, postID); err != nil {
			fmt.Printf("Failed to delete comments for post %s: %v\n", postID.Hex(), err)
		}
	}

	// B. Reactions (on Post)
	if err := s.feedRepo.DeleteReactionsByTargetID(ctx, postID); err != nil {
		fmt.Printf("Failed to delete reactions for post %s: %v\n", postID.Hex(), err)
	}

	// C. Media Cleanup (Album Links and Storage)
	if post.Media != nil && len(post.Media) > 0 {
		for _, media := range post.Media {
			// Remove from Album Media links
			if err := s.feedRepo.DeleteAlbumMediaByURL(ctx, media.URL); err != nil {
				fmt.Printf("Failed to delete album media link %s: %v\n", media.URL, err)
			}

			// Remove from Album Covers if used
			if err := s.feedRepo.RemoveAlbumCoverByURL(ctx, media.URL); err != nil {
				fmt.Printf("Failed to remove album cover for url %s: %v\n", media.URL, err)
			}

			// Delete from Object Storage
			if err := s.storageService.DeleteFile(ctx, media.URL); err != nil {
				fmt.Printf("Failed to delete file from storage %s: %v\n", media.URL, err)
			}
		}
	}

	// 2. Delete the Post itself
	err = s.feedRepo.DeletePost(ctx, userID, postID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return errors.New("post not found or unauthorized to delete")
		}
		return err
	}

	// Publish PostDeleted event to Kafka
	postDataBytes, err := json.Marshal(post)
	if err != nil {
		fmt.Printf("Failed to marshal deletedPost for WebSocketEvent: %v\n", err)
	} else {
		wsEvent := models.WebSocketEvent{
			Type: "PostDeleted",
			Data: postDataBytes,
		}
		eventBytes, err := json.Marshal(wsEvent)
		if err != nil {
			fmt.Printf("Failed to marshal WebSocketEvent for PostDeleted: %v\n", err)
		} else {
			kafkaMsg := kafkago.Message{
				Key:   []byte(post.UserID.Hex()),
				Value: eventBytes,
				Time:  time.Now(),
			}
			err = s.kafkaProducer.ProduceMessage(ctx, kafkaMsg)
			if err != nil {
				fmt.Printf("Failed to produce PostDeleted WebSocketEvent to Kafka: %v\n", err)
			}
		}
	}

	return nil
}

func (s *FeedService) ListPosts(ctx context.Context, viewerID primitive.ObjectID, filterUserID string, communityID string, page, limit int64, sortBy, sortOrder string, hasMedia bool, mediaType string, status string) (*models.FeedResponse, error) {
	// Base filter for public posts
	filter := bson.M{}

	// If a specific community is requested, filter by that community ID
	if communityID != "" {
		objCommunityID, err := primitive.ObjectIDFromHex(communityID)
		if err != nil {
			return nil, fmt.Errorf("invalid community ID: %w", err)
		}
		filter["community_id"] = objCommunityID

		// If status is provided, filter by it. Otherwise show all (or active?)
		// For communities, we might want to default to active unless specified.
		if status != "" {
			filter["status"] = status
		} else {
			// Default to Active if not specified
			filter["$or"] = []bson.M{
				{"status": models.PostStatusActive},
				{"status": bson.M{"$exists": false}},
				{"status": nil},
			}
		}

	} else if filterUserID != "" {
		// If a specific user's posts are requested, filter by that user ID
		objFilterUserID, err := primitive.ObjectIDFromHex(filterUserID)
		if err != nil {
			return nil, fmt.Errorf("invalid filter user ID: %w", err)
		}
		filter["user_id"] = objFilterUserID

		if status != "" {
			filter["status"] = status
		} else {
			filter["$or"] = []bson.M{
				{"status": models.PostStatusActive},
				{"status": bson.M{"$exists": false}},
				{"status": nil},
			}
		}

	} else {

		// Main Feed Default: Active Only
		statusFilter := bson.M{"$or": []bson.M{
			{"status": models.PostStatusActive},
			{"status": bson.M{"$exists": false}},
			{"status": nil},
		}}

		if status != "" {
			statusFilter = bson.M{"status": status}
		}

		// If no specific user or community is requested (Main Feed), apply privacy filters
		// Exclude community posts from the main feed
		filter["community_id"] = bson.M{"$exists": false}

		// Combined $or for privacy and status is tricky.
		// We have two distinct requirements: STATUS IS (Active OR Missing) AND PRIVACY IS (Public OR Friends).
		// MongoDB doesn't allow multiple top-level $or operators easily without $and.

		privacyFilter := []bson.M{
			{"privacy": models.PrivacySettingPublic},
		}

		// If user is logged in, include friends' posts
		if viewerID != primitive.NilObjectID {
			friendIDs, err := s.friendshipRepo.GetFriendIDs(ctx, viewerID)
			if err == nil {
				// Only add friends filter if user has friends (avoid empty $in array)
				if len(friendIDs) > 0 {
					privacyFilter = append(privacyFilter, bson.M{
						"privacy": models.PrivacySettingFriends,
						"user_id": bson.M{"$in": friendIDs},
					})
				}
				// Also include own posts
				privacyFilter = append(privacyFilter, bson.M{"user_id": viewerID})
			}
		}

		// Combine Status and Privacy filters using $and
		filter = bson.M{
			"$and": []bson.M{
				filter, // Includes community_id exists:false
				statusFilter,
				{
					"$or": privacyFilter,
				},
			},
		}

		// Note regarding the previous code structure:
		// The original code was appending to top-level $or for privacy.
		// We need to completely restructure the query construction to avoid overwriting.
	}

	// Apply filter for posts with media
	if hasMedia {
		filter["media"] = bson.M{"$exists": true, "$ne": []interface{}{}}
		// Start index 0 check is safer if 'media' is always an array
		filter["media.0"] = bson.M{"$exists": true}
	}

	// Filter by specific media type (image or video)
	if mediaType != "" {
		filter["media"] = bson.M{"$elemMatch": bson.M{"type": mediaType}}
	}

	// TODO: Implement CUSTOM privacy logic (requires fetching custom audience lists)
	sortField := "created_at"
	sortDir := -1 // descending

	switch sortBy {
	case "created_at":
		sortField = "created_at"
	case "reaction_count":
		sortField = "total_reactions"
	case "comment_count":
		sortField = "total_comments"
	}

	if sortOrder == "asc" {
		sortDir = 1 // ascending
	}

	opts := options.Find().
		SetSkip((page - 1) * limit).
		SetLimit(limit).
		SetSort(bson.D{{Key: sortField, Value: sortDir}})

	posts, err := s.feedRepo.ListPosts(ctx, filter, opts)
	if err != nil {
		return nil, err
	}

	total, err := s.feedRepo.CountPosts(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &models.FeedResponse{
		Posts: posts,
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}

func (s *FeedService) GetPostsByHashtag(ctx context.Context, viewerID primitive.ObjectID, hashtag string, page, limit int64) (*models.FeedResponse, error) {
	// Normalize hashtag to lowercase for consistent searching
	normalizedHashtag := strings.ToLower(hashtag)

	// Base filter for posts containing the hashtag
	filter := bson.M{
		"hashtags": normalizedHashtag,
		"$or": []bson.M{
			{"privacy": models.PrivacySettingPublic},
			{"user_id": viewerID, "privacy": models.PrivacySettingOnlyMe},
		},
	}

	// Get viewer's friends for FRIENDS privacy
	friends, err := s.friendshipRepo.GetFriends(ctx, viewerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get viewer's friends: %w", err)
	}

	var friendIDs []primitive.ObjectID
	for _, friend := range friends {
		friendIDs = append(friendIDs, friend.ID)
	}

	// Add FRIENDS privacy filter if viewer has friends
	if len(friendIDs) > 0 {
		filter["$or"] = append(filter["$or"].([]bson.M), bson.M{
			"privacy": models.PrivacySettingFriends,
			"user_id": bson.M{"$in": friendIDs},
		})
	}

	// Pagination and sorting options
	opts := options.Find().
		SetSkip((page - 1) * limit).
		SetLimit(limit).
		SetSort(bson.D{{Key: "created_at", Value: -1}}) // Sort by creation date, newest first

	posts, err := s.feedRepo.ListPosts(ctx, filter, opts)
	if err != nil {
		return nil, err
	}

	total, err := s.feedRepo.CountPosts(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &models.FeedResponse{
		Posts: posts,
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}

// Comment operations
func (s *FeedService) CreateComment(ctx context.Context, userID primitive.ObjectID, req *models.CreateCommentRequest) (*models.Comment, error) {
	// Fetch sender's user details for notification content
	senderUser, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		fmt.Printf("Failed to find sender user %s for notification: %v\n", userID.Hex(), err)
		// Decide how to handle: return error, or proceed with generic content
		// For now, we'll proceed, but log the error.
	}

	if req.PostID == nil {
		return nil, errors.New("post ID is required for this operation")
	}

	post, err := s.feedRepo.GetPostByID(ctx, *req.PostID)
	if err != nil {
		return nil, errors.New("post not found")
	}

	// Extract mentions from content
	mentionedUsernames := utils.ExtractMentions(req.Content)
	mentionedUsers, err := s.userRepo.FindUsersByUserNames(ctx, mentionedUsernames)
	if err != nil {
		// Log error but don't fail comment creation if mentioned users are not found
		fmt.Printf("Failed to find mentioned users: %v\n", err)
	}
	var mentionedUserIDs []primitive.ObjectID
	for _, user := range mentionedUsers {
		mentionedUserIDs = append(mentionedUserIDs, user.ID)
	}

	comment := &models.Comment{
		PostID:    *req.PostID,
		UserID:    userID,
		Content:   req.Content,
		MediaType: req.MediaType,
		MediaURL:  req.MediaURL,
		Mentions:  mentionedUserIDs,
		Replies:   []models.Reply{}, // Initialize as empty array
	}

	createdComment, err := s.feedRepo.CreateComment(ctx, comment)
	if err != nil {
		return nil, err
	}

	// --- Notify Post Author ---

	if post != nil {
		// Check if the commenter is not the post author
		if post.UserID != userID {
			// Check if the post author was already mentioned
			alreadyMentioned := false
			for _, mentionedID := range mentionedUserIDs {
				if mentionedID == post.UserID {
					alreadyMentioned = true
					break
				}
			}

			if !alreadyMentioned {
				notificationReq := &models.CreateNotificationRequest{
					RecipientID: post.UserID,
					SenderID:    userID,
					Type:        models.NotificationTypeComment,
					TargetID:    createdComment.ID,
					TargetType:  "comment",
					Content:     fmt.Sprintf("%s commented on your post.", senderUser.Username),
				}
				_, err := s.notificationService.CreateNotification(ctx, notificationReq)
				if err != nil {
					fmt.Printf("Failed to create comment notification for user %s: %v\n", post.UserID.Hex(), err)
				}
			}
		}
	}

	// --- End Notify Post Author ---

	// Send notifications to mentioned users
	for _, mentionedUserID := range mentionedUserIDs {
		notificationReq := &models.CreateNotificationRequest{
			RecipientID: mentionedUserID,
			SenderID:    userID,
			Type:        models.NotificationTypeMention,
			TargetID:    createdComment.ID,
			TargetType:  "comment",
			Content:     fmt.Sprintf("%s mentioned you in a comment.", senderUser.Username),
		}
		_, err := s.notificationService.CreateNotification(ctx, notificationReq)
		if err != nil {
			// Log the error but don't block comment creation
			fmt.Printf("Failed to create mention notification for user %s: %v\n", mentionedUserID.Hex(), err)
		}
	}

	// Publish CommentCreated event to Kafka
	if senderUser != nil {
		createdComment.Author = models.PostAuthor{
			ID:       senderUser.ID.Hex(),
			Username: senderUser.Username,
			Avatar:   senderUser.Avatar,
			FullName: senderUser.FullName,
		}
	}

	commentDataBytes, err := json.Marshal(createdComment)
	if err != nil {
		fmt.Printf("Failed to marshal createdComment for WebSocketEvent: %v\n", err)
		// Log the error but don't block comment creation
	} else {
		wsEvent := models.WebSocketEvent{
			Type: "CommentCreated",
			Data: commentDataBytes,
		}
		eventBytes, err := json.Marshal(wsEvent)
		if err != nil {
			fmt.Printf("Failed to marshal WebSocketEvent for CommentCreated: %v\n", err)
		} else {
			// Safe dereference for key since we checked nil above
			kafkaMsg := kafkago.Message{
				Key:   []byte(createdComment.PostID.Hex()), // Key for comment events (using post ID)
				Value: eventBytes,
				Time:  time.Now(),
			}
			err = s.kafkaProducer.ProduceMessage(ctx, kafkaMsg)
			if err != nil {
				fmt.Printf("Failed to produce CommentCreated WebSocketEvent to Kafka: %v\n", err)
				// Log the error but don't block comment creation
			}
		}
	}

	// Increment comment count on the post
	err = s.feedRepo.IncrementPostCommentCount(ctx, *req.PostID)
	if err != nil {
		return nil, fmt.Errorf("failed to increment comment count for post %s: %w", req.PostID.Hex(), err)
	}

	return createdComment, nil
}

func (s *FeedService) UpdateComment(ctx context.Context, userID, commentID primitive.ObjectID, req *models.UpdateCommentRequest) (*models.Comment, error) {
	comment, err := s.feedRepo.GetCommentByID(ctx, commentID)
	if err != nil {
		return nil, errors.New("comment not found")
	}
	if comment.UserID != userID {
		return nil, errors.New("unauthorized to update this comment")
	}

	updateData := bson.M{
		"updated_at": time.Now(),
	}

	if req.Content != "" {
		updateData["content"] = req.Content
	}
	if req.Mentions != nil {
		updateData["mentions"] = req.Mentions
	}

	updatedComment, err := s.feedRepo.UpdateComment(ctx, commentID, updateData)
	if err != nil {
		return nil, err
	}
	return updatedComment, nil
}

func (s *FeedService) DeleteComment(ctx context.Context, userID, postID, commentID primitive.ObjectID) error {
	comment, err := s.feedRepo.GetCommentByID(ctx, commentID)
	if err != nil {
		return errors.New("comment not found")
	}
	if comment.UserID != userID {
		return errors.New("unauthorized to delete this comment")
	}

	err = s.feedRepo.DeleteComment(ctx, postID, commentID)
	if err != nil {
		return err
	}

	// Decrement comment count on the post
	err = s.feedRepo.DecrementPostCommentCount(ctx, postID)
	if err != nil {
		return fmt.Errorf("failed to decrement comment count for post %s: %w", postID.Hex(), err)
	}

	return nil
}

// Reply operations
func (s *FeedService) CreateReply(ctx context.Context, userID primitive.ObjectID, req *models.CreateReplyRequest) (*models.Reply, error) {
	// Fetch sender's user details for notification content
	senderUser, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		fmt.Printf("Failed to find sender user %s for notification: %v\n", userID.Hex(), err)
		// Decide how to handle: return error, or proceed with generic content
		// For now, we'll proceed, but log the error.
	}

	// Check if comment exists
	_, err = s.feedRepo.GetCommentByID(ctx, req.CommentID)
	if err != nil {
		return nil, errors.New("comment not found")
	}

	// Disallow replies to replies
	if req.ParentReplyID != nil && !req.ParentReplyID.IsZero() {
		return nil, errors.New("replies to replies are not allowed")
	}

	// Extract mentions from content
	mentionedUsernames := utils.ExtractMentions(req.Content)
	mentionedUsers, err := s.userRepo.FindUsersByUserNames(ctx, mentionedUsernames)
	if err != nil {
		// Log error but don't fail reply creation if mentioned users are not found
		fmt.Printf("Failed to find mentioned users: %v\n", err)
	}
	var mentionedUserIDs []primitive.ObjectID
	for _, user := range mentionedUsers {
		mentionedUserIDs = append(mentionedUserIDs, user.ID)
	}

	reply := &models.Reply{
		CommentID: req.CommentID,
		UserID:    userID,
		Content:   req.Content,
		MediaType: req.MediaType,
		MediaURL:  req.MediaURL,
		Mentions:  mentionedUserIDs,
	}

	createdReply, err := s.feedRepo.CreateReply(ctx, reply)
	if err != nil {
		return nil, err
	}

	// --- Notify Comment Author ---
	comment, err := s.feedRepo.GetCommentByID(ctx, req.CommentID)
	if err != nil {
		fmt.Printf("Failed to get comment %s for reply notification: %v\n", req.CommentID.Hex(), err)
	} else {
		// Check if the replier is not the comment author
		if comment.UserID != userID {
			// Check if the comment author was already mentioned
			alreadyMentioned := false
			for _, mentionedID := range mentionedUserIDs {
				if mentionedID == comment.UserID {
					alreadyMentioned = true
					break
				}
			}

			if !alreadyMentioned {
				notificationReq := &models.CreateNotificationRequest{
					RecipientID: comment.UserID,
					SenderID:    userID,
					Type:        models.NotificationTypeReply,
					TargetID:    createdReply.ID,
					TargetType:  "reply",
					Content:     fmt.Sprintf("%s replied to your comment.", senderUser.Username),
				}
				_, err := s.notificationService.CreateNotification(ctx, notificationReq)
				if err != nil {
					fmt.Printf("Failed to create reply notification for user %s: %v\n", comment.UserID.Hex(), err)
				}
			}
		}
	}
	// --- End Notify Comment Author ---

	// Send notifications to mentioned users
	for _, mentionedUserID := range mentionedUserIDs {
		notificationReq := &models.CreateNotificationRequest{
			RecipientID: mentionedUserID,
			SenderID:    userID,
			Type:        models.NotificationTypeMention,
			TargetID:    createdReply.ID,
			TargetType:  "reply",
			Content:     fmt.Sprintf("%s mentioned you in a reply.", senderUser.Username),
		}
		_, err := s.notificationService.CreateNotification(ctx, notificationReq)
		if err != nil {
			// Log the error but don't block reply creation
			fmt.Printf("Failed to create mention notification for user %s: %v\n", mentionedUserID.Hex(), err)
		}
	}

	// Publish ReplyCreated event to Kafka
	if senderUser != nil {
		createdReply.Author = models.PostAuthor{
			ID:       senderUser.ID.Hex(),
			Username: senderUser.Username,
			Avatar:   senderUser.Avatar,
			FullName: senderUser.FullName,
		}
	}

	replyDataBytes, err := json.Marshal(createdReply)
	if err != nil {
		fmt.Printf("Failed to marshal createdReply for WebSocketEvent: %v\n", err)
		// Log the error but don't block reply creation
	} else {
		wsEvent := models.WebSocketEvent{
			Type: "ReplyCreated",
			Data: replyDataBytes,
		}
		eventBytes, err := json.Marshal(wsEvent)
		if err != nil {
			fmt.Printf("Failed to marshal WebSocketEvent for ReplyCreated: %v\n", err)
		} else {
			kafkaMsg := kafkago.Message{
				Key:   []byte(createdReply.CommentID.Hex()), // Key for reply events (using comment ID)
				Value: eventBytes,
				Time:  time.Now(),
			}
			err = s.kafkaProducer.ProduceMessage(ctx, kafkaMsg)
			if err != nil {
				fmt.Printf("Failed to produce ReplyCreated WebSocketEvent to Kafka: %v\n", err)
				// Log the error but don't block reply creation
			}
		}
	}

	return createdReply, nil

}

func (s *FeedService) UpdateReply(ctx context.Context, userID, replyID primitive.ObjectID, req *models.UpdateReplyRequest) (*models.Reply, error) {
	reply, err := s.feedRepo.GetReplyByID(ctx, replyID)
	if err != nil {
		return nil, errors.New("reply not found")
	}
	if reply.UserID != userID {
		return nil, errors.New("unauthorized to update this reply")
	}

	updateData := bson.M{
		"updated_at": time.Now(),
	}

	if req.Content != "" {
		updateData["content"] = req.Content
	}
	if req.Mentions != nil {
		updateData["mentions"] = req.Mentions
	}

	updatedReply, err := s.feedRepo.UpdateReply(ctx, replyID, updateData)
	if err != nil {
		return nil, err
	}
	return updatedReply, nil
}

func (s *FeedService) DeleteReply(ctx context.Context, userID, commentID, replyID primitive.ObjectID) error {
	reply, err := s.feedRepo.GetReplyByID(ctx, replyID)
	if err != nil {
		return errors.New("reply not found")
	}
	if reply.UserID != userID {
		return errors.New("unauthorized to delete this reply")
	}

	return s.feedRepo.DeleteReply(ctx, commentID, replyID)
}

// Reaction operations
func (s *FeedService) CreateReaction(ctx context.Context, userID primitive.ObjectID, req *models.CreateReactionRequest) (*models.Reaction, error) {
	reaction := &models.Reaction{
		UserID:     userID,
		TargetID:   req.TargetID,
		TargetType: req.TargetType,
		Type:       req.Type,
	}

	createdReaction, err := s.feedRepo.CreateReaction(ctx, reaction)
	if err != nil {
		return nil, err
	}

	// Send notification to the target's owner
	var targetOwnerID primitive.ObjectID
	switch req.TargetType {
	case "post":
		post, err := s.feedRepo.GetPostByID(ctx, req.TargetID)
		if err != nil {
			fmt.Printf("Failed to get post %s for reaction notification: %v\n", req.TargetID.Hex(), err)
			return createdReaction, nil
		}
		targetOwnerID = post.UserID
	case "comment":
		comment, err := s.feedRepo.GetCommentByID(ctx, req.TargetID)
		if err != nil {
			fmt.Printf("Failed to get comment %s for reaction notification: %v\n", req.TargetID.Hex(), err)
			return createdReaction, nil
		}
		targetOwnerID = comment.UserID
	case "reply":
		reply, err := s.feedRepo.GetReplyByID(ctx, req.TargetID)
		if err != nil {
			fmt.Printf("Failed to get reply %s for reaction notification: %v\n", req.TargetID.Hex(), err)
			return createdReaction, nil
		}
		targetOwnerID = reply.UserID
	}

	if targetOwnerID != userID { // Don't notify if user reacts to their own content
		// Fetch sender's user details
		senderUser, err := s.userRepo.FindUserByID(ctx, userID)
		if err != nil {
			fmt.Printf("Failed to find sender user %s for reaction notification: %v\n", userID.Hex(), err)
			return createdReaction, nil // Continue without notification if sender not found
		}

		notificationReq := &models.CreateNotificationRequest{
			RecipientID: targetOwnerID,
			SenderID:    userID,
			Type:        models.NotificationTypeLike, // Using LIKE for all reactions for now
			TargetID:    createdReaction.TargetID,
			TargetType:  createdReaction.TargetType,
			Content:     fmt.Sprintf("%s reacted to your %s with %s.", senderUser.Username, createdReaction.TargetType, createdReaction.Type),
			Data: map[string]interface{}{
				"sender_username": senderUser.Username,
				"sender_avatar":   senderUser.Avatar,
				"reaction_type":   createdReaction.Type,
				"target_type":     createdReaction.TargetType,
			},
		}
		_, err = s.notificationService.CreateNotification(ctx, notificationReq)
		if err != nil {
			fmt.Printf("Failed to create reaction notification for user %s: %v\n", targetOwnerID.Hex(), err)
		}
	}

	// Publish ReactionCreated event to Kafka
	reactionDataBytes, err := json.Marshal(createdReaction)
	if err != nil {
		fmt.Printf("Failed to marshal createdReaction for WebSocketEvent: %v\n", err)
		// Log the error but don't block reaction creation
	} else {
		wsEvent := models.WebSocketEvent{
			Type: "ReactionCreated",
			Data: reactionDataBytes,
		}
		eventBytes, err := json.Marshal(wsEvent)
		if err != nil {
			fmt.Printf("Failed to marshal WebSocketEvent for ReactionCreated: %v\n", err)
		} else {
			kafkaMsg := kafkago.Message{
				Key:   []byte(createdReaction.TargetID.Hex()), // Key for reaction events (using target ID)
				Value: eventBytes,
				Time:  time.Now(),
			}
			err = s.kafkaProducer.ProduceMessage(ctx, kafkaMsg)
			if err != nil {
				fmt.Printf("Failed to produce ReactionCreated WebSocketEvent to Kafka: %v\n", err)
				// Log the error but don't block reaction creation
			}
		}
	}
	// Increment reaction count on the target document
	switch req.TargetType {
	case "post":
		err = s.feedRepo.IncrementPostReactionCount(ctx, req.TargetID)
	case "comment":
		err = s.feedRepo.IncrementCommentReactionCount(ctx, req.TargetID)
	case "reply":
		err = s.feedRepo.IncrementReplyReactionCount(ctx, req.TargetID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to increment reaction count for target %s (%s): %w", req.TargetID.Hex(), req.TargetType, err)
	}

	return createdReaction, nil
}

func (s *FeedService) DeleteReaction(ctx context.Context, userID primitive.ObjectID, reactionID, targetID primitive.ObjectID, targetType string) error {
	// Verify if the reaction belongs to the user
	reaction, err := s.feedRepo.GetReactionByID(ctx, reactionID) // Assuming GetReactionByID exists or needs to be created
	if err != nil {
		return errors.New("reaction not found")
	}
	if reaction.UserID != userID {
		return errors.New("unauthorized to delete this reaction")
	}

	err = s.feedRepo.DeleteReaction(ctx, reactionID, userID, targetID, targetType)
	if err != nil {
		return err
	}

	// Publish ReactionDeleted event to Kafka
	reactionDataBytes, err := json.Marshal(reaction) // Marshal the deleted reaction
	if err != nil {
		fmt.Printf("Failed to marshal deletedReaction for WebSocketEvent: %v\n", err)
	} else {
		wsEvent := models.WebSocketEvent{
			Type: "ReactionDeleted", // New event type
			Data: reactionDataBytes,
		}
		eventBytes, err := json.Marshal(wsEvent)
		if err != nil {
			fmt.Printf("Failed to marshal WebSocketEvent for ReactionDeleted: %v\n", err)
		} else {
			kafkaMsg := kafkago.Message{
				Key:   []byte(reaction.TargetID.Hex()), // Key for reaction events (using target ID)
				Value: eventBytes,
				Time:  time.Now(),
			}
			err = s.kafkaProducer.ProduceMessage(ctx, kafkaMsg)
			if err != nil {
				fmt.Printf("Failed to produce ReactionDeleted WebSocketEvent to Kafka: %v\n", err)
			}
		}
	}
	// Decrement reaction count on the target document
	switch targetType {
	case "post":
		err = s.feedRepo.DecrementPostReactionCount(ctx, targetID)
	case "comment":
		err = s.feedRepo.DecrementCommentReactionCount(ctx, targetID)
	case "reply":
		err = s.feedRepo.DecrementReplyReactionCount(ctx, targetID)
	}
	if err != nil {
		return fmt.Errorf("failed to decrement reaction count for target %s (%s): %w", targetID.Hex(), targetType, err)
	}

	return nil
}

func (s *FeedService) GetReactionsByTargetID(ctx context.Context, targetID primitive.ObjectID, targetType string) ([]models.Reaction, error) {
	filter := bson.M{"target_id": targetID, "target_type": targetType}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: 1}})
	return s.feedRepo.ListReactions(ctx, filter, opts)
}

func (s *FeedService) GetCommentsByPostID(ctx context.Context, postID primitive.ObjectID, page, limit int64) ([]models.Comment, error) {
	filter := bson.M{"post_id": postID}
	opts := options.Find().
		SetSkip((page - 1) * limit).
		SetLimit(limit).
		SetSort(bson.D{{Key: "created_at", Value: 1}})
	comments, err := s.feedRepo.ListComments(ctx, filter, opts)
	if err != nil {
		return nil, err
	}

	return comments, nil
}

func (s *FeedService) GetRepliesByCommentID(ctx context.Context, commentID primitive.ObjectID, page, limit int64) ([]models.Reply, error) {
	filter := bson.M{"comment_id": commentID}
	opts := options.Find().
		SetSkip((page - 1) * limit).
		SetLimit(limit).
		SetSort(bson.D{{Key: "created_at", Value: 1}})
	replies, err := s.feedRepo.ListReplies(ctx, filter, opts)
	if err != nil {
		return nil, err
	}

	return replies, nil
}

// ----------------------------- Albums -----------------------------

// ----------------------------- Albums -----------------------------

func (s *FeedService) CreateAlbum(ctx context.Context, userID primitive.ObjectID, req *models.CreateAlbumRequest) (*models.Album, error) {
	album := &models.Album{
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		Type:        models.AlbumTypeCustom,
		Privacy:     req.Privacy,
		PostIDs:     []primitive.ObjectID{},
	}

	return s.feedRepo.CreateAlbum(ctx, album)
}

func (s *FeedService) GetUserAlbums(ctx context.Context, userID primitive.ObjectID, limit, offset int64) ([]models.Album, error) {
	// Ensure default albums exist (virtual or real)
	// We check if they exist, if not create them
	// This might be better done on demand, but for listing we want them to appear.

	// 1. Profile Pictures
	_, err := s.EnsureAlbumExists(ctx, userID, models.AlbumTypeProfile, "Profile Pictures", false)
	if err != nil {
		fmt.Printf("Failed to ensure profile album: %v\n", err)
	}

	// 2. Cover Photos
	_, err = s.EnsureAlbumExists(ctx, userID, models.AlbumTypeCover, "Cover Photos", false)
	if err != nil {
		fmt.Printf("Failed to ensure cover album: %v\n", err)
	}

	// 3. Timeline Photos (Virtual/Aggregated)
	_, err = s.EnsureAlbumExists(ctx, userID, models.AlbumTypeTimeline, "Timeline Photos", false)
	if err != nil {
		fmt.Printf("Failed to ensure timeline album: %v\n", err)
	}

	return s.feedRepo.ListAlbums(ctx, userID, limit, offset)
}

func (s *FeedService) EnsureAlbumExists(ctx context.Context, userID primitive.ObjectID, albumType models.AlbumType, defaultName string, skipBackfill bool) (*models.Album, error) {
	album, err := s.feedRepo.GetAlbumByType(ctx, userID, albumType)
	createdNew := false
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}

		// Create if not exists
		newAlbum := &models.Album{
			UserID:  userID,
			Name:    defaultName,
			Type:    albumType,
			Privacy: models.PrivacySettingPublic, // Default to public for profile/cover
		}
		album, err = s.feedRepo.CreateAlbum(ctx, newAlbum)
		if err != nil {
			return nil, err
		}
		createdNew = true
	}

	// Backfill logic: If album is profile/cover and empty/new, ensure current user photo is in it
	if !skipBackfill && (albumType == models.AlbumTypeProfile || albumType == models.AlbumTypeCover) {
		shouldCheck := createdNew
		if !shouldCheck {
			// Check if empty
			media, _, err := s.feedRepo.GetAlbumMedia(ctx, album.ID, 1, 0, "")
			if err == nil && len(media) == 0 {
				shouldCheck = true
			}
		}

		if shouldCheck {
			user, err := s.userRepo.FindUserByID(ctx, userID)
			if err == nil && user != nil {
				var urlToBackfill string
				// Assume image type for profile/cover photos
				const mediaType = "image"

				if albumType == models.AlbumTypeProfile && user.Avatar != "" {
					urlToBackfill = user.Avatar
				} else if albumType == models.AlbumTypeCover && user.CoverPicture != "" {
					urlToBackfill = user.CoverPicture
				}

				if urlToBackfill != "" {
					// Add to album
					err := s.AddMediaToAlbum(ctx, userID, album.ID, []models.MediaItem{{
						URL:  urlToBackfill,
						Type: mediaType,
					}})
					if err != nil {
						fmt.Printf("Failed to backfill %s to album %s: %v\n", urlToBackfill, album.ID.Hex(), err)
					} else {
						// Refresh album to return updated state (e.g. cover url)
						updatedAlbum, err := s.feedRepo.GetAlbumByID(ctx, album.ID)
						if err == nil {
							album = updatedAlbum
						}
					}
				}
			}
		}
	}

	return album, nil
}

func (s *FeedService) AddMediaToAlbum(ctx context.Context, userID, albumID primitive.ObjectID, media []models.MediaItem) error {
	album, err := s.feedRepo.GetAlbumByID(ctx, albumID)
	if err != nil {
		return err
	}
	if album.UserID != userID {
		return errors.New("unauthorized to update this album")
	}

	if album.Type == models.AlbumTypeTimeline {
		return errors.New("cannot manually add media to timeline photos, use posts instead")
	}

	// Convert MediaItem to AlbumMedia
	albumMediaItems := make([]models.AlbumMedia, len(media))
	now := time.Now()
	for i, item := range media {
		albumMediaItems[i] = models.AlbumMedia{
			AlbumID:   albumID,
			UserID:    userID,
			URL:       item.URL,
			Type:      item.Type,
			CreatedAt: now,
		}
	}

	// Add media
	if err := s.feedRepo.AddMediaToAlbum(ctx, albumMediaItems); err != nil {
		return err
	}

	// If it's the first media (or logic dictates), set as cover if none exists -> This logic is harder now as we don't know total count easily without query
	// But we can check if current cover is empty
	if album.CoverURL == "" && len(media) > 0 {
		// Try to find an image
		for _, m := range media {
			if m.Type == "image" {
				// Update cover (ignore error)
				_ = s.feedRepo.UpdateAlbumCover(ctx, albumID, m.URL)
				break
			}
		}
	}

	return nil
}

func (s *FeedService) GetAlbumMedia(ctx context.Context, albumID primitive.ObjectID, limit, offset int64, mediaType string) ([]models.AlbumMedia, int64, error) {
	album, err := s.feedRepo.GetAlbumByID(ctx, albumID)
	if err != nil {
		return nil, 0, err
	}

	if album.Type == models.AlbumTypeTimeline {
		return s.feedRepo.GetTimelineMedia(ctx, album.UserID, limit, offset, mediaType)
	}

	return s.feedRepo.GetAlbumMedia(ctx, albumID, limit, offset, mediaType)
}

func (s *FeedService) GetAlbum(ctx context.Context, albumID primitive.ObjectID) (*models.Album, error) {
	// TODO: Check privacy?
	return s.feedRepo.GetAlbumByID(ctx, albumID)
}

// UpdateAlbum updates album properties (name, description, cover, privacy)
func (s *FeedService) UpdateAlbum(ctx context.Context, userID, albumID primitive.ObjectID, req *models.UpdateAlbumRequest) (*models.Album, error) {
	// First, get the album to verify ownership and prevent updating system albums
	album, err := s.feedRepo.GetAlbumByID(ctx, albumID)
	if err != nil {
		return nil, err
	}

	// Prevent modifying system albums (profile, cover, timeline)
	if album.Type != models.AlbumTypeCustom {
		return nil, fmt.Errorf("cannot modify system albums")
	}

	// Build update map from request
	update := bson.M{}
	if req.Name != "" {
		update["name"] = req.Name
	}
	if req.Description != "" {
		update["description"] = req.Description
	}
	if req.CoverURL != "" {
		update["cover_url"] = req.CoverURL
	}
	if req.Privacy != "" {
		update["privacy"] = req.Privacy
	}

	if len(update) == 0 {
		return album, nil // Nothing to update
	}

	return s.feedRepo.UpdateAlbum(ctx, albumID, userID, update)
}
