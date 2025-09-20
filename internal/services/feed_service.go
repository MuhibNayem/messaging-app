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
	"go.mongodb.org/mongo-driver/mongo/options"
)

type FeedService struct {
	feedRepo            *repositories.FeedRepository
	userRepo            *repositories.UserRepository
	friendshipRepo      *repositories.FriendshipRepository
	privacyRepo         repositories.PrivacyRepository
	kafkaProducer       *kafka.MessageProducer
	notificationService *notifications.NotificationService
}

func NewFeedService(feedRepo *repositories.FeedRepository, userRepo *repositories.UserRepository, friendshipRepo *repositories.FriendshipRepository, privacyRepo repositories.PrivacyRepository, kafkaProducer *kafka.MessageProducer, notificationService *notifications.NotificationService) *FeedService {
	return &FeedService{feedRepo: feedRepo, userRepo: userRepo, friendshipRepo: friendshipRepo, privacyRepo: privacyRepo, kafkaProducer: kafkaProducer, notificationService: notificationService}
}

// Post operations
func (s *FeedService) CreatePost(ctx context.Context, userID primitive.ObjectID, req *models.CreatePostRequest) (*models.Post, error) {
	// Extract mentions from content
	mentionedUsernames := utils.ExtractMentions(req.Content)
	var mentionedUserIDs []primitive.ObjectID
	for _, username := range mentionedUsernames {
		user, err := s.userRepo.FindUserByUserName(ctx, username)
		if err != nil {
			// Log error but don't fail post creation if a mentioned user is not found
			continue
		}
		if user != nil {
			mentionedUserIDs = append(mentionedUserIDs, user.ID)
		}
	}

	post := &models.Post{
		UserID:         userID,
		Content:        req.Content,
		MediaType:      req.MediaType,
		MediaURL:       req.MediaURL,
		Privacy:        req.Privacy,
		CustomAudience: req.CustomAudience,
		Mentions:       mentionedUserIDs,
		Hashtags:       req.Hashtags,
	}

	createdPost, err := s.feedRepo.CreatePost(ctx, post)
	if err != nil {
		return nil, err
	}

	// Send notifications to mentioned users
	for _, mentionedUserID := range mentionedUserIDs {
		notificationReq := &models.CreateNotificationRequest{
			RecipientID: mentionedUserID,
			SenderID:    userID,
			Type:        models.NotificationTypeMention,
			TargetID:    createdPost.ID,
			TargetType:  "post",
			Content:     fmt.Sprintf("%s mentioned you in a post.", createdPost.UserID),
		}
		_, err := s.notificationService.CreateNotification(ctx, notificationReq)
		if err != nil {
			// Log the error but don't block post creation
			fmt.Printf("Failed to create mention notification for user %s: %v\n", mentionedUserID.Hex(), err)
		}
	}

	// Publish PostCreated event to Kafka
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

	return createdPost, nil
}

func (s *FeedService) GetPostByID(ctx context.Context, viewerID, postID primitive.ObjectID) (*models.Post, error) {
	post, err := s.feedRepo.GetPostByID(ctx, postID)
	if err != nil {
		return nil, errors.New("post not found")
	}

	// Populate author details
	author, err := s.userRepo.FindUserByID(ctx, post.UserID)
	if err != nil {
		fmt.Printf("Failed to find author for post %s (user ID: %s): %v\n", post.ID.Hex(), post.UserID.Hex(), err)
		post.Author = models.PostAuthor{
			ID:       post.UserID,
			Username: "[Deleted User]",
			FullName: "[Deleted User]",
		}
	} else {
		post.Author = models.PostAuthor{
			ID:       author.ID,
			Username: author.Username,
			Avatar:   author.Avatar,
			FullName: author.FullName,
		}
	}

	// Check privacy
	canView, err := s.canViewPost(ctx, viewerID, post)
	if err != nil {
		return nil, err
	}
	if !canView {
		return nil, errors.New("unauthorized to view this post")
	}

	// Populate reaction counts
	reactionCounts, err := s.feedRepo.CountReactionsByType(ctx, post.ID, "post")
	if err != nil {
		return nil, fmt.Errorf("failed to get post reaction counts: %w", err)
	}
	post.ReactionCounts = reactionCounts

	return post, nil
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
	if req.MediaType != "" {
		updateData["media_type"] = req.MediaType
	}
	if req.MediaURL != "" {
		updateData["media_url"] = req.MediaURL
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
	return updatedPost, nil
}

func (s *FeedService) DeletePost(ctx context.Context, userID, postID primitive.ObjectID) error {
	post, err := s.feedRepo.GetPostByID(ctx, postID)
	if err != nil {
		return errors.New("post not found")
	}
	if post.UserID != userID {
		return errors.New("unauthorized to delete this post")
	}

	return s.feedRepo.DeletePost(ctx, postID)
}

func (s *FeedService) ListPosts(ctx context.Context, viewerID primitive.ObjectID, filterUserID string, page, limit int64, sortBy, sortOrder string) (*models.FeedResponse, error) {
	// Base filter for public posts
	filter := bson.M{}

	// If a specific user's posts are requested, filter by that user ID
	if filterUserID != "" {
		objFilterUserID, err := primitive.ObjectIDFromHex(filterUserID)
		if err != nil {
			return nil, fmt.Errorf("invalid filter user ID: %w", err)
		}
		filter["user_id"] = objFilterUserID
	} else {
		// If no specific user is requested, apply privacy filters
		filter["$or"] = []bson.M{
			{"privacy": models.PrivacySettingPublic},
			{"user_id": viewerID}, // Author can always see their own posts, regardless of privacy
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
	}

	// TODO: Implement CUSTOM privacy logic (requires fetching custom audience lists)

	sortField := "created_at"
	sortDir := -1 // descending

	switch sortBy {
	case "created_at":
		sortField = "created_at"
	case "reaction_count":
		// Sorting by reaction_count requires aggregation, which is more complex.
		// For now, we'll just sort by created_at if reaction_count is requested.
		// This is a placeholder for future advanced ranking.
		sortField = "created_at"
	case "comment_count":
		// Similar to reaction_count, requires aggregation.
		sortField = "created_at"
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

	// Populate author details and reaction counts for each post
	for i := range posts {
		// Populate author details
		author, err := s.userRepo.FindUserByID(ctx, posts[i].UserID)
		if err != nil {
			// Log error, but don't fail the entire operation.
			// Set a default/placeholder author if not found.
			fmt.Printf("Failed to find author for post %s (user ID: %s): %v\n", posts[i].ID.Hex(), posts[i].UserID.Hex(), err)
			posts[i].Author = models.PostAuthor{
				ID:       posts[i].UserID,
				Username: "[Deleted User]",
				FullName: "[Deleted User]",
			}
		} else {
			posts[i].Author = models.PostAuthor{
				ID:       author.ID,
				Username: author.Username,
				Avatar:   author.Avatar,
				FullName: author.FullName,
			}
		}

		// Populate reaction counts
		reactionCounts, err := s.feedRepo.CountReactionsByType(ctx, posts[i].ID, "post")
		if err != nil {
			// Log the error but don't fail the entire list operation
			fmt.Printf("Failed to get reaction counts for post %s: %v\n", posts[i].ID.Hex(), err)
			continue
		}
		posts[i].ReactionCounts = reactionCounts
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

	// Populate reaction counts for each post
	for i := range posts {
		reactionCounts, err := s.feedRepo.CountReactionsByType(ctx, posts[i].ID, "post")
		if err != nil {
			fmt.Printf("Failed to get reaction counts for post %s: %v\n", posts[i].ID.Hex(), err)
			continue
		}
		posts[i].ReactionCounts = reactionCounts
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
	// Check if post exists
	_, err := s.feedRepo.GetPostByID(ctx, req.PostID)
	if err != nil {
		return nil, errors.New("post not found")
	}

	// Extract mentions from content
	mentionedUsernames := utils.ExtractMentions(req.Content)
	var mentionedUserIDs []primitive.ObjectID
	for _, username := range mentionedUsernames {
		user, err := s.userRepo.FindUserByUserName(ctx, username)
		if err != nil {
			// Log error but don't fail comment creation if a mentioned user is not found
			continue
		}
		if user != nil {
			mentionedUserIDs = append(mentionedUserIDs, user.ID)
		}
	}

	comment := &models.Comment{
		PostID:   req.PostID,
		UserID:   userID,
		Content:  req.Content,
		Mentions: mentionedUserIDs,
	}

	createdComment, err := s.feedRepo.CreateComment(ctx, comment)
	if err != nil {
		return nil, err
	}

	// Send notifications to mentioned users
	for _, mentionedUserID := range mentionedUserIDs {
		notificationReq := &models.CreateNotificationRequest{
			RecipientID: mentionedUserID,
			SenderID:    userID,
			Type:        models.NotificationTypeMention,
			TargetID:    createdComment.ID,
			TargetType:  "comment",
			Content:     fmt.Sprintf("%s mentioned you in a comment.", userID),
		}
		_, err := s.notificationService.CreateNotification(ctx, notificationReq)
		if err != nil {
			// Log the error but don't block comment creation
			fmt.Printf("Failed to create mention notification for user %s: %v\n", mentionedUserID.Hex(), err)
		}
	}

	// Publish CommentCreated event to Kafka
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

	return s.feedRepo.DeleteComment(ctx, postID, commentID)
}

// Reply operations
func (s *FeedService) CreateReply(ctx context.Context, userID primitive.ObjectID, req *models.CreateReplyRequest) (*models.Reply, error) {
	// Check if comment exists
	_, err := s.feedRepo.GetCommentByID(ctx, req.CommentID)
	if err != nil {
		return nil, errors.New("comment not found")
	}

	// Extract mentions from content
	mentionedUsernames := utils.ExtractMentions(req.Content)
	var mentionedUserIDs []primitive.ObjectID
	for _, username := range mentionedUsernames {
		user, err := s.userRepo.FindUserByUserName(ctx, username)
		if err != nil {
			// Log error but don't fail reply creation if a mentioned user is not found
			continue
		}
		if user != nil {
			mentionedUserIDs = append(mentionedUserIDs, user.ID)
		}
	}

	reply := &models.Reply{
		CommentID: req.CommentID,
		UserID:    userID,
		Content:   req.Content,
		Mentions:  mentionedUserIDs,
	}

	createdReply, err := s.feedRepo.CreateReply(ctx, reply)
	if err != nil {
		return nil, err
	}

	// Send notifications to mentioned users
	for _, mentionedUserID := range mentionedUserIDs {
		notificationReq := &models.CreateNotificationRequest{
			RecipientID: mentionedUserID,
			SenderID:    userID,
			Type:        models.NotificationTypeMention,
			TargetID:    createdReply.ID,
			TargetType:  "reply",
			Content:     fmt.Sprintf("%s mentioned you in a reply.", userID),
		}
		_, err := s.notificationService.CreateNotification(ctx, notificationReq)
		if err != nil {
			// Log the error but don't block reply creation
			fmt.Printf("Failed to create mention notification for user %s: %v\n", mentionedUserID.Hex(), err)
		}
	}

	// Publish ReplyCreated event to Kafka
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

	return nil
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

	// Populate reaction counts for each comment
	for i := range comments {
		reactionCounts, err := s.feedRepo.CountReactionsByType(ctx, comments[i].ID, "comment")
		if err != nil {
			// Log the error but don't fail the entire list operation
			fmt.Printf("Failed to get reaction counts for comment %s: %v\n", comments[i].ID.Hex(), err)
			continue
		}
		comments[i].ReactionCounts = reactionCounts
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

	// Populate reaction counts for each reply
	for i := range replies {
		reactionCounts, err := s.feedRepo.CountReactionsByType(ctx, replies[i].ID, "reply")
		if err != nil {
			// Log the error but don't fail the entire list operation
			fmt.Printf("Failed to get reaction counts for reply %s: %v\n", replies[i].ID.Hex(), err)
			continue
		}
		replies[i].ReactionCounts = reactionCounts
	}

	return replies, nil
}

func (s *FeedService) GetReactionsByTargetID(ctx context.Context, targetID primitive.ObjectID, targetType string) ([]models.Reaction, error) {
	filter := bson.M{"target_id": targetID, "target_type": targetType}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: 1}})
	return s.feedRepo.ListReactions(ctx, filter, opts)
}
