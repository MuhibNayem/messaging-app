package services

import (
	"context"
	"errors"
	"messaging-app/internal/kafka"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type FeedService struct {
	feedRepo        *repositories.FeedRepository
	userRepo        *repositories.UserRepository
	friendshipRepo  *repositories.FriendshipRepository
	privacyRepo     repositories.PrivacyRepository
	kafkaProducer   *kafka.MessageProducer
}

func NewFeedService(feedRepo *repositories.FeedRepository, userRepo *repositories.UserRepository, friendshipRepo *repositories.FriendshipRepository, privacyRepo repositories.PrivacyRepository, kafkaProducer *kafka.MessageProducer) *FeedService {
	return &FeedService{feedRepo: feedRepo, userRepo: userRepo, friendshipRepo: friendshipRepo, privacyRepo: privacyRepo, kafkaProducer: kafkaProducer}
}

// Post operations
func (s *FeedService) CreatePost(ctx context.Context, userID primitive.ObjectID, req *models.CreatePostRequest) (*models.Post, error) {
	post := &models.Post{
		UserID:      userID,
		Content:     req.Content,
		MediaType:   req.MediaType,
		MediaURL:    req.MediaURL,
		Privacy:     req.Privacy,
		CustomAudience: req.CustomAudience,
		Mentions:    req.Mentions,
		Hashtags:    req.Hashtags,
	}

	createdPost, err := s.feedRepo.CreatePost(ctx, post)
	if err != nil {
		return nil, err
	}
	return createdPost, nil
}

func (s *FeedService) GetPostByID(ctx context.Context, postID primitive.ObjectID) (*models.Post, error) {
	return s.feedRepo.GetPostByID(ctx, postID)
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

func (s *FeedService) ListPosts(ctx context.Context, viewerID primitive.ObjectID, page, limit int64) (*models.FeedResponse, error) {
	// TODO: Implement privacy logic here
	// For now, return all posts
	filter := bson.M{}

	opts := options.Find().
		SetSkip((page - 1) * limit).
		SetLimit(limit).
		SetSort(bson.D{{Key: "created_at", Value: -1}})

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
	// Check if post exists
	_, err := s.feedRepo.GetPostByID(ctx, req.PostID)
	if err != nil {
		return nil, errors.New("post not found")
	}

	comment := &models.Comment{
		PostID:   req.PostID,
		UserID:   userID,
		Content:  req.Content,
		Mentions: req.Mentions,
	}

	createdComment, err := s.feedRepo.CreateComment(ctx, comment)
	if err != nil {
		return nil, err
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

	reply := &models.Reply{
		CommentID: req.CommentID,
		UserID:    userID,
		Content:   req.Content,
		Mentions:  req.Mentions,
	}

	createdReply, err := s.feedRepo.CreateReply(ctx, reply)
	if err != nil {
		return nil, err
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
	return createdReaction, nil
}

func (s *FeedService) DeleteReaction(ctx context.Context, userID primitive.ObjectID, reactionID, targetID primitive.ObjectID, targetType string) error {
	// TODO: Verify if the reaction belongs to the user
	return s.feedRepo.DeleteReaction(ctx, reactionID, userID, targetID, targetType)
}

func (s *FeedService) GetCommentsByPostID(ctx context.Context, postID primitive.ObjectID, page, limit int64) ([]models.Comment, error) {
	filter := bson.M{"post_id": postID}
	opts := options.Find().
		SetSkip((page - 1) * limit).
		SetLimit(limit).
		SetSort(bson.D{{Key: "created_at", Value: 1}})
	return s.feedRepo.ListComments(ctx, filter, opts)
}

func (s *FeedService) GetRepliesByCommentID(ctx context.Context, commentID primitive.ObjectID, page, limit int64) ([]models.Reply, error) {
	filter := bson.M{"comment_id": commentID}
	opts := options.Find().
		SetSkip((page - 1) * limit).
		SetLimit(limit).
		SetSort(bson.D{{Key: "created_at", Value: 1}})
	return s.feedRepo.ListReplies(ctx, filter, opts)
}

func (s *FeedService) GetReactionsByTargetID(ctx context.Context, targetID primitive.ObjectID, targetType string) ([]models.Reaction, error) {
	filter := bson.M{"target_id": targetID, "target_type": targetType}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: 1}})
	return s.feedRepo.ListReactions(ctx, filter, opts)
}