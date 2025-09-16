package repositories

import (
	"context"
	"time"

	"messaging-app/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type FeedRepository struct {
	db *mongo.Database
}

func NewFeedRepository(db *mongo.Database) *FeedRepository {
	// Create indexes for posts
	_, err := db.Collection("posts").Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index(),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index(),
		},
		{
			Keys:    bson.D{{Key: "hashtags", Value: 1}},
			Options: options.Index(),
		},
	})
	if err != nil {
		panic("Failed to create post indexes: " + err.Error())
	}

	// Create indexes for comments
	_, err = db.Collection("comments").Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "post_id", Value: 1}},
			Options: options.Index(),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index(),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index(),
		},
	})
	if err != nil {
		panic("Failed to create comment indexes: " + err.Error())
	}

	// Create indexes for replies
	_, err = db.Collection("replies").Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "comment_id", Value: 1}},
			Options: options.Index(),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index(),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index(),
		},
	})
	if err != nil {
		panic("Failed to create reply indexes: " + err.Error())
	}

	// Create indexes for reactions
	_, err = db.Collection("reactions").Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "target_id", Value: 1}, {Key: "target_type", Value: 1}},
			Options: options.Index(),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index(),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index(),
		},
	})
	if err != nil {
		panic("Failed to create reaction indexes: " + err.Error())
	}

	return &FeedRepository{db: db}
}

// Post operations
func (r *FeedRepository) CreatePost(ctx context.Context, post *models.Post) (*models.Post, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	post.CreatedAt = time.Now()
	post.UpdatedAt = time.Now()
	result, err := r.db.Collection("posts").InsertOne(ctx, post)
	if err != nil {
		return nil, err
	}
	post.ID = result.InsertedID.(primitive.ObjectID)
	return post, nil
}

func (r *FeedRepository) GetPostByID(ctx context.Context, postID primitive.ObjectID) (*models.Post, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var post models.Post
	err := r.db.Collection("posts").FindOne(ctx, bson.M{"_id": postID}).Decode(&post)
	if err != nil {
		return nil, err
	}
	return &post, nil
}

func (r *FeedRepository) UpdatePost(ctx context.Context, postID primitive.ObjectID, update bson.M) (*models.Post, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	update["updated_at"] = time.Now()
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	result := r.db.Collection("posts").FindOneAndUpdate(
		ctx,
		bson.M{"_id": postID},
		bson.M{"$set": update},
		opts,
	)

	var updatedPost models.Post
	if err := result.Decode(&updatedPost); err != nil {
		return nil, err
	}
	return &updatedPost, nil
}

func (r *FeedRepository) DeletePost(ctx context.Context, postID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.db.Collection("posts").DeleteOne(ctx, bson.M{"_id": postID})
	return err
}

func (r *FeedRepository) ListPosts(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]models.Post, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cursor, err := r.db.Collection("posts").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var posts []models.Post
	if err := cursor.All(ctx, &posts); err != nil {
		return nil, err
	}
	return posts, nil
}

func (r *FeedRepository) CountPosts(ctx context.Context, filter bson.M) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	count, err := r.db.Collection("posts").CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// Comment operations
func (r *FeedRepository) CreateComment(ctx context.Context, comment *models.Comment) (*models.Comment, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	comment.CreatedAt = time.Now()
	comment.UpdatedAt = time.Now()
	result, err := r.db.Collection("comments").InsertOne(ctx, comment)
	if err != nil {
		return nil, err
	}
	comment.ID = result.InsertedID.(primitive.ObjectID)

	// Add comment ID to the post's comments array
	_, err = r.db.Collection("posts").UpdateOne(
		ctx,
		bson.M{"_id": comment.PostID},
		bson.M{"$push": bson.M{"comments": comment.ID}},
	)
	if err != nil {
		// Consider rolling back comment creation or logging a warning
		return nil, err
	}

	return comment, nil
}

func (r *FeedRepository) GetCommentByID(ctx context.Context, commentID primitive.ObjectID) (*models.Comment, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var comment models.Comment
	err := r.db.Collection("comments").FindOne(ctx, bson.M{"_id": commentID}).Decode(&comment)
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

func (r *FeedRepository) UpdateComment(ctx context.Context, commentID primitive.ObjectID, update bson.M) (*models.Comment, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	update["updated_at"] = time.Now()
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	result := r.db.Collection("comments").FindOneAndUpdate(
		ctx,
		bson.M{"_id": commentID},
		bson.M{"$set": update},
		opts,
	)

	var updatedComment models.Comment
	if err := result.Decode(&updatedComment); err != nil {
		return nil, err
	}
	return &updatedComment, nil
}

func (r *FeedRepository) DeleteComment(ctx context.Context, postID, commentID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Remove comment ID from the post's comments array
	_, err := r.db.Collection("posts").UpdateOne(
		ctx,
		bson.M{"_id": postID},
		bson.M{"$pull": bson.M{"comments": commentID}},
	)
	if err != nil {
		return err
	}

	// Delete the comment itself
	_, err = r.db.Collection("comments").DeleteOne(ctx, bson.M{"_id": commentID})
	return err
}

// Reply operations
func (r *FeedRepository) CreateReply(ctx context.Context, reply *models.Reply) (*models.Reply, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	reply.CreatedAt = time.Now()
	reply.UpdatedAt = time.Now()
	result, err := r.db.Collection("replies").InsertOne(ctx, reply)
	if err != nil {
		return nil, err
	}
	reply.ID = result.InsertedID.(primitive.ObjectID)

	// Add reply ID to the comment's replies array
	_, err = r.db.Collection("comments").UpdateOne(
		ctx,
		bson.M{"_id": reply.CommentID},
		bson.M{"$push": bson.M{"replies": reply.ID}},
	)
	if err != nil {
		// Consider rolling back reply creation or logging a warning
		return nil, err
	}

	return reply, nil
}

func (r *FeedRepository) GetReplyByID(ctx context.Context, replyID primitive.ObjectID) (*models.Reply, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var reply models.Reply
	err := r.db.Collection("replies").FindOne(ctx, bson.M{"_id": replyID}).Decode(&reply)
	if err != nil {
		return nil, err
	}
	return &reply, nil
}

func (r *FeedRepository) UpdateReply(ctx context.Context, replyID primitive.ObjectID, update bson.M) (*models.Reply, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	update["updated_at"] = time.Now()
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	result := r.db.Collection("replies").FindOneAndUpdate(
		ctx,
		bson.M{"_id": replyID},
		bson.M{"$set": update},
		opts,
	)

	var updatedReply models.Reply
	if err := result.Decode(&updatedReply); err != nil {
		return nil, err
	}
	return &updatedReply, nil
}

func (r *FeedRepository) DeleteReply(ctx context.Context, commentID, replyID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Remove reply ID from the comment's replies array
	_, err := r.db.Collection("comments").UpdateOne(
		ctx,
		bson.M{"_id": commentID},
		bson.M{"$pull": bson.M{"replies": replyID}},
	)
	if err != nil {
		return err
	}

	// Delete the reply itself
	_, err = r.db.Collection("replies").DeleteOne(ctx, bson.M{"_id": replyID})
	return err
}

// Reaction operations
func (r *FeedRepository) CreateReaction(ctx context.Context, reaction *models.Reaction) (*models.Reaction, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	reaction.CreatedAt = time.Now()
	result, err := r.db.Collection("reactions").InsertOne(ctx, reaction)
	if err != nil {
		return nil, err
	}
	reaction.ID = result.InsertedID.(primitive.ObjectID)

	// Update the target (post, comment, or reply) with the new reaction
	switch reaction.TargetType {
	case "post":
		_, err = r.db.Collection("posts").UpdateOne(
			ctx,
			bson.M{"_id": reaction.TargetID},
			bson.M{"$addToSet": bson.M{"likes": reaction.UserID}},
		)
	case "comment":
		_, err = r.db.Collection("comments").UpdateOne(
			ctx,
			bson.M{"_id": reaction.TargetID},
			bson.M{"$addToSet": bson.M{"likes": reaction.UserID}},
		)
	case "reply":
		_, err = r.db.Collection("replies").UpdateOne(
			ctx,
			bson.M{"_id": reaction.TargetID},
			bson.M{"$addToSet": bson.M{"likes": reaction.UserID}},
		)
	}

	if err != nil {
		// Consider rolling back reaction creation or logging a warning
		return nil, err
	}

	return reaction, nil
}

func (r *FeedRepository) DeleteReaction(ctx context.Context, reactionID, userID, targetID primitive.ObjectID, targetType string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.db.Collection("reactions").DeleteOne(ctx, bson.M{"_id": reactionID, "user_id": userID, "target_id": targetID, "target_type": targetType})
	if err != nil {
		return err
	}

	// Remove reaction from the target (post, comment, or reply)
	switch targetType {
	case "post":
		_, err = r.db.Collection("posts").UpdateOne(
			ctx,
			bson.M{"_id": targetID},
			bson.M{"$pull": bson.M{"likes": userID}},
		)
	case "comment":
		_, err = r.db.Collection("comments").UpdateOne(
			ctx,
			bson.M{"_id": targetID},
			bson.M{"$pull": bson.M{"likes": userID}},
		)
	case "reply":
		_, err = r.db.Collection("replies").UpdateOne(
			ctx,
			bson.M{"_id": targetID},
			bson.M{"$pull": bson.M{"likes": userID}},
		)
	}

	return err
}

func (r *FeedRepository) ListComments(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]models.Comment, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cursor, err := r.db.Collection("comments").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var comments []models.Comment
	if err := cursor.All(ctx, &comments); err != nil {
		return nil, err	}
	return comments, nil
}

func (r *FeedRepository) ListReplies(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]models.Reply, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cursor, err := r.db.Collection("replies").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var replies []models.Reply
	if err := cursor.All(ctx, &replies); err != nil {
		return nil, err
	}
	return replies, nil
}

func (r *FeedRepository) ListReactions(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]models.Reaction, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cursor, err := r.db.Collection("reactions").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var reactions []models.Reaction
	if err := cursor.All(ctx, &reactions); err != nil {
		return nil, err
	}
	return reactions, nil
}