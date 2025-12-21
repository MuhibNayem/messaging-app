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
		// Index for marketplace partner queries (presence broadcast)
		{
			Keys: bson.D{
				{Key: "is_marketplace", Value: 1},
				{Key: "sender_id", Value: 1},
				{Key: "receiver_id", Value: 1},
			},
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

	// Convert SenderID to ObjectID first
	senderID, err := primitive.ObjectIDFromHex(query.SenderID)
	if err != nil {
		return nil, errors.New("invalid sender ID")
	}

	// Build user filter (who is involved in conversation)
	var userFilter bson.M
	if query.GroupID != "" {
		groupID, err := primitive.ObjectIDFromHex(query.GroupID)
		if err != nil {
			return nil, errors.New("invalid group ID")
		}
		userFilter = bson.M{"group_id": groupID}
	} else if query.ReceiverID != "" {
		receiverID, err := primitive.ObjectIDFromHex(query.ReceiverID)
		if err != nil {
			return nil, errors.New("invalid receiver ID")
		}
		// Find messages where the current user (senderID) and the other user (receiverID) are involved
		userFilter = bson.M{"$or": []bson.M{
			{"sender_id": senderID, "receiver_id": receiverID},
			{"sender_id": receiverID, "receiver_id": senderID},
		}}
	} else {
		return nil, errors.New("either groupID or receiverID must be provided")
	}

	// Build marketplace filter based on context
	var marketplaceFilter bson.M
	if query.Marketplace {
		// Only include messages marked as marketplace context
		marketplaceFilter = bson.M{"is_marketplace": true}
	} else if query.ReceiverID != "" {
		// Exclude marketplace messages from regular DMs (only for direct messages, not groups)
		marketplaceFilter = bson.M{"$or": []bson.M{
			{"is_marketplace": bson.M{"$exists": false}},
			{"is_marketplace": false},
		}}
	}

	// Combine filters with $and
	if marketplaceFilter != nil {
		filter["$and"] = []bson.M{userFilter, marketplaceFilter}
	} else {
		filter = userFilter
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		// Lookup sender info
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "sender_id",
			"foreignField": "_id",
			"as":           "sender_info",
		}}},
		bson.D{{Key: "$unwind", Value: bson.M{"path": "$sender_info", "preserveNullAndEmptyArrays": true}}},
		// Lookup product info (for messages with product_id)
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "products",
			"localField":   "product_id",
			"foreignField": "_id",
			"as":           "product_info",
		}}},
		bson.D{{Key: "$unwind", Value: bson.M{"path": "$product_info", "preserveNullAndEmptyArrays": true}}},
		// Project fields to match models.Message struct
		bson.D{{Key: "$project", Value: bson.M{
			"_id":         1,
			"sender_id":   1,
			"sender_name": "$sender_info.username", // Populate sender_name
			"sender": bson.M{
				"_id":       "$sender_info._id",
				"username":  "$sender_info.username",
				"email":     "$sender_info.email",
				"avatar":    "$sender_info.avatar",
				"full_name": "$sender_info.full_name",
				"bio":       "$sender_info.bio",
			},
			"receiver_id":         1,
			"group_id":            1,
			"group_name":          1,
			"content":             1,
			"content_type":        1,
			"media_urls":          1,
			"seen_by":             1,
			"delivered_to":        1,
			"is_deleted":          1,
			"deleted_at":          1,
			"original_content":    1,
			"is_edited":           1,
			"edited_at":           1,
			"reactions":           1,
			"reply_to_message_id": 1,
			"product_id":          1, // Marketplace product link (optional metadata)
			"is_marketplace":      1, // Marketplace context flag
			// Embedded product data (lightweight, for display)
			"product": bson.M{
				"_id":      "$product_info._id",
				"title":    "$product_info.title",
				"price":    "$product_info.price",
				"currency": "$product_info.currency",
				"images":   "$product_info.images",
				"status":   "$product_info.status",
			},
			"created_at":   1,
			"updated_at":   1,
			"is_encrypted": 1,
			"iv":           1,
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}}}}, // Sort by creation time descending
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

	log.Printf("Fetched %d messages with filter: %+v", len(messages), filter)
	return messages, nil
}

func (r *MessageRepository) CreateMessage(ctx context.Context, msg *models.Message) (*models.Message, error) {
	msg.CreatedAt = time.Now()
	msg.UpdatedAt = time.Now()
	// Ensure SeenBy and DeliveredTo are initialized as empty slices
	if msg.SeenBy == nil {
		msg.SeenBy = []primitive.ObjectID{}
	}
	if msg.DeliveredTo == nil {
		msg.DeliveredTo = []primitive.ObjectID{}
	}

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

func (r *MessageRepository) MarkConversationAsSeen(ctx context.Context, conversationID primitive.ObjectID, userID primitive.ObjectID, timestamp time.Time, isGroup bool) error {
	filter := bson.M{
		"created_at": bson.M{"$lte": timestamp},
		"seen_by":    bson.M{"$ne": userID},
	}

	if isGroup {
		filter["group_id"] = conversationID
	} else {
		filter["$or"] = []bson.M{
			{"sender_id": userID, "receiver_id": conversationID},
			{"sender_id": conversationID, "receiver_id": userID},
		}
	}

	update := bson.M{
		"$addToSet": bson.M{"seen_by": userID},
		"$set":      bson.M{"updated_at": time.Now()},
	}

	_, err := r.collection.UpdateMany(ctx, filter, update)
	return err
}

func (r *MessageRepository) MarkMessagesAsDelivered(ctx context.Context, userID primitive.ObjectID, messageIDs []primitive.ObjectID) error {
	_, err := r.collection.UpdateMany(
		ctx,
		bson.M{"_id": bson.M{"$in": messageIDs}},
		bson.M{
			"$addToSet": bson.M{"delivered_to": userID},
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
	currentUserID primitive.ObjectID,
) (int64, error) {
	filter := bson.M{}

	if isGroup {
		filter["group_id"] = conversationID
	} else {
		// For direct messages, count messages between currentUserID and conversationID (the other user)
		filter["$or"] = []bson.M{
			{"sender_id": currentUserID, "receiver_id": conversationID},
			{"sender_id": conversationID, "receiver_id": currentUserID},
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

	// First, fetch the message to check ownership and creation time
	var existingMessage models.Message
	err := r.collection.FindOne(ctx, bson.M{
		"_id":       messageID,
		"sender_id": requesterID,
	}).Decode(&existingMessage)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.New("message not found or not owned by user")
		}
		return nil, err
	}

	// Check if message is within 7-day deletion window
	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour)
	if existingMessage.CreatedAt.Before(sevenDaysAgo) {
		return nil, errors.New("message can only be deleted within 7 days of creation")
	}

	var deletedMessage models.Message
	err = r.collection.FindOneAndUpdate(
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

// EditMessage updates the content of a message
func (r *MessageRepository) EditMessage(
	ctx context.Context,
	messageID primitive.ObjectID,
	requesterID primitive.ObjectID,
	newContent string,
) (*models.Message, error) {
	// First, fetch the message to check ownership and creation time
	var existingMessage models.Message
	err := r.collection.FindOne(ctx, bson.M{
		"_id":        messageID,
		"sender_id":  requesterID,
		"is_deleted": false,
	}).Decode(&existingMessage)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.New("message not found, not owned by user, or already deleted")
		}
		return nil, err
	}

	// Check if message is within 1-hour edit window
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	if existingMessage.CreatedAt.Before(oneHourAgo) {
		return nil, errors.New("message can only be edited within 1 hour of creation")
	}

	var updatedMessage models.Message
	now := time.Now()
	err = r.collection.FindOneAndUpdate(
		ctx,
		bson.M{
			"_id":        messageID,
			"sender_id":  requesterID,
			"is_deleted": false, // Cannot edit a deleted message
		},
		bson.M{
			"$set": bson.M{
				"content":    newContent,
				"is_edited":  true,
				"edited_at":  &now,
				"updated_at": now,
			},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updatedMessage)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.New("message not found, not owned by user, or already deleted")
		}
		return nil, err
	}
	return &updatedMessage, nil
}

func (r *MessageRepository) SearchMessages(ctx context.Context, userID primitive.ObjectID, query string, groupIDs []primitive.ObjectID, page, limit int64) ([]models.Message, error) {
	// Define the text search stage
	textSearchStage := bson.D{{Key: "$match", Value: bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: query}}}}}}

	// Define the filter to only include user's conversations
	conversationFilter := bson.D{{Key: "$match", Value: bson.D{{Key: "$or", Value: []bson.M{
		{"group_id": bson.M{"$in": groupIDs}},
		{"sender_id": userID},
		{"receiver_id": userID},
	}}}}}

	// Pagination stages
	skipStage := bson.D{{Key: "$skip", Value: (page - 1) * limit}}
	limitStage := bson.D{{Key: "$limit", Value: limit}}

	// Sorting by text search score
	sortStage := bson.D{{Key: "$sort", Value: bson.D{{Key: "score", Value: bson.D{{Key: "$meta", Value: "textScore"}}}}}}

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

// AddReaction adds a reaction to a message
func (r *MessageRepository) AddReaction(ctx context.Context, messageID, userID primitive.ObjectID, emoji string) error {
	filter := bson.M{"_id": messageID, "reactions.user_id": bson.M{"$ne": userID}, "reactions.emoji": bson.M{"$ne": emoji}}
	update := bson.M{
		"$push": bson.M{
			"reactions": models.MessageReaction{
				UserID:    userID,
				Emoji:     emoji,
				Timestamp: time.Now(),
			},
		},
		"$set": bson.M{"updated_at": time.Now()},
	}

	res, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to add reaction: %w", err)
	}
	if res.ModifiedCount == 0 {
		return errors.New("message not found or reaction already exists")
	}
	return nil
}

// RemoveReaction removes a reaction from a message
func (r *MessageRepository) RemoveReaction(ctx context.Context, messageID, userID primitive.ObjectID, emoji string) error {
	filter := bson.M{"_id": messageID}
	update := bson.M{
		"$pull": bson.M{
			"reactions": bson.M{
				"user_id": userID,
				"emoji":   emoji,
			},
		},
		"$set": bson.M{"updated_at": time.Now()},
	}

	res, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to remove reaction: %w", err)
	}
	if res.ModifiedCount == 0 {
		return errors.New("message not found or reaction not present")
	}
	return nil
}

func (r *MessageRepository) GetMessageByID(ctx context.Context, messageID primitive.ObjectID) (*models.Message, error) {
	var message models.Message
	err := r.collection.FindOne(ctx, bson.M{"_id": messageID}).Decode(&message)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.New("message not found")
		}
		return nil, err
	}
	return &message, nil
}

// GetMarketplacePartnerIDs returns all unique user IDs that have exchanged marketplace messages with the given user.
// This is used for presence broadcasting to marketplace conversation partners who may not be friends.
func (r *MessageRepository) GetMarketplacePartnerIDs(ctx context.Context, userID primitive.ObjectID) ([]primitive.ObjectID, error) {
	// Find all marketplace messages where user is sender or receiver
	pipeline := mongo.Pipeline{
		// Match marketplace messages involving this user
		bson.D{{Key: "$match", Value: bson.M{
			"is_marketplace": true,
			"$or": []bson.M{
				{"sender_id": userID},
				{"receiver_id": userID},
			},
		}}},
		// Project to get the "other" user ID
		bson.D{{Key: "$project", Value: bson.M{
			"partner_id": bson.M{
				"$cond": bson.A{
					bson.M{"$eq": bson.A{"$sender_id", userID}},
					"$receiver_id",
					"$sender_id",
				},
			},
		}}},
		// Get unique partner IDs
		bson.D{{Key: "$group", Value: bson.M{
			"_id": "$partner_id",
		}}},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	partnerIDs := make([]primitive.ObjectID, len(results))
	for i, r := range results {
		partnerIDs[i] = r.ID
	}
	return partnerIDs, nil
}
