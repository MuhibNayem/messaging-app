package repositories

import (
	"context"
	"errors"
	"fmt"
	"log"
	"messaging-app/internal/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MessageRepository struct {
	db         *mongo.Database
	collection *mongo.Collection
}

func NewMessageRepository(db *mongo.Database) *MessageRepository {
	collection := db.Collection("messages")

	// Compound indexes for faster queries
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "sender_id", Value: 1},
				{Key: "receiver_id", Value: 1},
				{Key: "created_at", Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: "group_id", Value: 1},
				{Key: "created_at", Value: -1},
			},
		},
		{
			Keys: bson.D{{Key: "content_type", Value: 1}},
		},
		// TTL index for auto-deleting messages after 1 year
		{
			Keys:    bson.D{{Key: "created_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(365 * 24 * 60 * 60),
		},
	}

	_, err := collection.Indexes().CreateMany(context.Background(), indexes)
	if err != nil {
		panic("Failed to create message indexes: " + err.Error())
	}

	return &MessageRepository{
		db:         db,
		collection: collection,
	}
}

func (r *MessageRepository) GetMessages(ctx context.Context, query models.MessageQuery) ([]models.Message, error) {
	filter := bson.M{}

	if query.GroupID != "" {
		groupID, err := primitive.ObjectIDFromHex(query.GroupID)
		if err != nil {
			return nil, errors.New("invalid group ID")
		}
		filter["chat_id"] = groupID // Assuming chat_id is used for groups
	} else if query.ReceiverID != "" {
		receiverID, err := primitive.ObjectIDFromHex(query.ReceiverID)
		if err != nil {
			return nil, errors.New("invalid receiver ID")
		}
		// For private chats, we need to find messages where both sender and receiver are involved
		// This logic might need to be adjusted based on how private chats are structured (e.g., a dedicated chat document)
		// For now, assuming a direct sender-receiver relationship in messages
		filter["$or"] = []bson.M{
			{"sender_id": query.SenderID, "receiver_id": receiverID},
			{"sender_id": receiverID, "receiver_id": query.SenderID},
		}
	} else {
		return nil, errors.New("either group_id or receiver_id must be provided")
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "sender_id",
			"foreignField": "_id",
			"as":           "sender_info",
		}}},
		bson.D{{Key: "$unwind", Value: bson.M{"path": "$sender_info", "preserveNullAndEmptyArrays": true}}},
		bson.D{{Key: "$project", Value: bson.M{
			"_id":        1,
			"chat_id":    1,
			"sender_id":  1,
			"content":    1,
			"timestamp":  1,
			"read_by":    1,
			"edited":     1,
			"deleted":    1,
			"media_type": 1,
			"media_url":  1,
			"reply_to":   1,
			"sender": bson.M{
				"id":          "$sender_info._id",
				"username":    "$sender_info.username",
				"avatar":      "$sender_info.avatar",
				"full_name":   "$sender_info.full_name",
				"online":      "$sender_info.online",
				"last_active": "$sender_info.last_active",
			},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "timestamp", Value: -1}}}},
		bson.D{{Key: "$skip", Value: int64((query.Page - 1) * query.Limit)}},
		bson.D{{Key: "$limit", Value: int64(query.Limit)}},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var messages []models.Message
	if err = cursor.All(ctx, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (r *MessageRepository) CreateMessage(ctx context.Context, msg *models.Message) (*models.Message, error) {
	msg.CreatedAt = time.Now()
	msg.UpdatedAt = time.Now()

	res, err := r.collection.InsertOne(ctx, msg)
	if err != nil {
		return nil, err
	}

	msg.ID = res.InsertedID.(primitive.ObjectID)
	return msg, nil
}

func (r *MessageRepository) MarkMessagesAsSeen(ctx context.Context, userID primitive.ObjectID, messageIDs []primitive.ObjectID) error {
	_, err := r.collection.UpdateMany(
		ctx,
		bson.M{"_id": bson.M{"$in": messageIDs}},
		bson.M{
			"$addToSet": bson.M{"seen_by": userID},
			"$set":      bson.M{"updated_at": time.Now()},
		},
	)
	return err
}

func (r *MessageRepository) GetUnreadCount(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{
		"receiver_id": userID,
		"seen_by":     bson.M{"$ne": userID},
	})
}

func (r *MessageRepository) GetConversationMessageCount(
	ctx context.Context,
	conversationID primitive.ObjectID,
	isGroup bool,
) (int64, error) {
	filter := bson.M{}

	if isGroup {
		filter["group_id"] = conversationID
	} else {
		filter["$or"] = []bson.M{
			{"sender_id": conversationID},
			{"receiver_id": conversationID},
		}
	}

	count, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to count messages: %w", err)
	}

	return count, nil
}

func (r *MessageRepository) DeleteMessage(
	ctx context.Context,
	messageID primitive.ObjectID,
	requesterID primitive.ObjectID,
	mediaDeleter func(ctx context.Context, urls []string) error,
) (*models.Message, error) {
	log.Printf("Deleting message with ID: %s by user: %s", messageID.Hex(), requesterID.Hex())
	var deletedMessage models.Message
	err := r.collection.FindOneAndUpdate(
		ctx,
		bson.M{
			"_id":       messageID,
			"sender_id": requesterID,
		},
		bson.M{
			"$set": bson.M{
				"deleted_at":       time.Now(),
				"is_deleted":       true,
				"original_content": "$content",
				"content":          "[deleted]",
				"media_urls":       []string{},
				"content_type":     models.ContentTypeDeleted,
			},
		},
		options.FindOneAndUpdate().
			SetReturnDocument(options.After).
			SetProjection(bson.M{
				"original_content": 0,
			}),
	).Decode(&deletedMessage)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.New("message not found or not owned by user")
		}
		return nil, err
	}

	// Async media cleanup
	if len(deletedMessage.MediaURLs) > 0 && mediaDeleter != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := mediaDeleter(ctx, deletedMessage.MediaURLs); err != nil {
				log.Printf("Failed to cleanup media for message %s: %v", messageID.Hex(), err)
			}
		}()
	}

	return &deletedMessage, nil
}

func (r *MessageRepository) SearchMessages(ctx context.Context, userID primitive.ObjectID, query string, groupIDs []primitive.ObjectID, page, limit int64) ([]models.Message, error) {
	// Define the text search stage
	textSearchStage := bson.D{{"$match", bson.D{{"$text", bson.D{{"$search", query}}}}}}

	// Define the filter to only include user's conversations
	conversationFilter := bson.D{{"$match", bson.D{{"$or", []bson.M{
		{"group_id": bson.M{"$in": groupIDs}},
		{"sender_id": userID},
		{"receiver_id": userID},
	}}}}}

	// Pagination stages
	skipStage := bson.D{{"$skip", (page - 1) * limit}}
	limitStage := bson.D{{"$limit", limit}}

	// Sorting by text search score
	sortStage := bson.D{{"$sort", bson.D{{"score", bson.D{{"$meta", "textScore"}}}}}}

	pipeline := mongo.Pipeline{textSearchStage, conversationFilter, sortStage, skipStage, limitStage}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var messages []models.Message
	if err = cursor.All(ctx, &messages); err != nil {
		return nil, err
	}

	return messages, nil
}
