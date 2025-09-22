package repositories

import (
	"context"
	"log"
	"messaging-app/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type ConversationRepository struct {
	db        *mongo.Database
	userRepo  *UserRepository
	groupRepo *GroupRepository
}

func NewConversationRepository(db *mongo.Database, userRepo *UserRepository, groupRepo *GroupRepository) *ConversationRepository {
	return &ConversationRepository{
		db:        db,
		userRepo:  userRepo,
		groupRepo: groupRepo,
	}
}

func (r *ConversationRepository) GetConversationSummaries(ctx context.Context, userID primitive.ObjectID) ([]models.ConversationSummary, error) {
	log.Printf("Repo: GetConversationSummaries for user %s", userID.Hex())
	var summaries []models.ConversationSummary

	// --- 1. Get Direct Message Conversations (Friends) ---
	log.Printf("Repo: Starting direct message aggregation for user %s", userID.Hex())
	friendshipsCursor, err := r.db.Collection("friendships").Aggregate(ctx, mongo.Pipeline{
		bson.D{{"$match", bson.M{
			"status": models.FriendshipStatusAccepted,
			"$or": []bson.M{
				{"requester_id": userID},
				{"receiver_id": userID},
			},
		}}},
		// Project the other user's ID
		bson.D{{"$project", bson.M{
			"_id": 0,
			"other_user_id": bson.M{
				"$cond": bson.A{bson.M{"$eq": bson.A{"$requester_id", userID}}, "$receiver_id", "$requester_id"},
			},
		}}},
		// Lookup user info for the other user
		bson.D{{"$lookup", bson.M{
			"from":         "users",
			"localField":   "other_user_id",
			"foreignField": "_id",
			"as":           "user_info",
		}}},
		bson.D{{"$unwind", bson.M{"path": "$user_info", "preserveNullAndEmptyArrays": true}}},
		// Find the last message for this direct conversation
		bson.D{{"$lookup", bson.M{
			"from": "messages",
			"let":  bson.M{"u1": userID, "u2": "$other_user_id"},
			"pipeline": bson.A{
				bson.M{"$match": bson.M{
					"$or": bson.A{
						bson.M{"sender_id": "$$u1", "receiver_id": "$$u2"},
						bson.M{"sender_id": "$$u2", "receiver_id": "$$u1"},
					},
				}},
				bson.M{"$sort": bson.M{"created_at": -1}},
				bson.M{"$limit": 1},
			},
			"as": "last_message_dm",
		}}},
		bson.D{{"$unwind", bson.M{"path": "$last_message_dm", "preserveNullAndEmptyArrays": true}}},
		// Project into ConversationSummary format - FIXED: Use inclusion-only projection
		bson.D{{"$project", bson.M{
			"_id":                    "$user_info._id",
			"name":                   "$user_info.username",
			"avatar":                 "$user_info.avatar",
			"is_group":               bson.M{"$literal": false},
			"last_message_content":   "$last_message_dm.content",
			"last_message_timestamp": "$last_message_dm.created_at",
		}}},
	}) // End of direct message aggregation
	if err != nil {
		log.Printf("Repo: Error aggregating direct messages for user %s: %v", userID.Hex(), err)
		return nil, err
	}
	var dmSummaries []models.ConversationSummary
	if err := friendshipsCursor.All(ctx, &dmSummaries); err != nil {
		log.Printf("Repo: Error decoding direct message summaries for user %s: %v", userID.Hex(), err)
		return nil, err
	}
	log.Printf("Repo: Retrieved %d direct message summaries for user %s", len(dmSummaries), userID.Hex())
	summaries = append(summaries, dmSummaries...)

	// --- 2. Get Group Message Conversations ---
	log.Printf("Repo: Starting group message aggregation for user %s", userID.Hex())
	groupsCursor, err := r.db.Collection("groups").Aggregate(ctx, mongo.Pipeline{
		bson.D{{"$match", bson.M{"members": userID}}},
		// Find the last message for this group conversation
		bson.D{{"$lookup", bson.M{
			"from": "messages",
			"let":  bson.M{"groupId": "$_id"},
			"pipeline": bson.A{
				bson.M{"$match": bson.M{"group_id": "$$groupId"}},
				bson.M{"$sort": bson.M{"created_at": -1}},
				bson.M{"$limit": 1},
			},
			"as": "last_message_group",
		}}},
		bson.D{{"$unwind", bson.M{"path": "$last_message_group", "preserveNullAndEmptyArrays": true}}},
		// Project into ConversationSummary format - FIXED: Use inclusion-only projection
		bson.D{{"$project", bson.M{
			"id":                     "$_id",
			"name":                   "$name",
			"avatar":                 "$avatar",
			"is_group":               bson.M{"$literal": true},
			"last_message_content":   "$last_message_group.content",
			"last_message_timestamp": "$last_message_group.created_at",
		}}},
	}) // End of group message aggregation
	if err != nil {
		log.Printf("Repo: Error aggregating group messages for user %s: %v", userID.Hex(), err)
		return nil, err
	}
	var groupSummaries []models.ConversationSummary
	if err := groupsCursor.All(ctx, &groupSummaries); err != nil {
		log.Printf("Repo: Error decoding group message summaries for user %s: %v", userID.Hex(), err)
		return nil, err
	}
	log.Printf("Repo: Retrieved %d group message summaries for user %s", len(groupSummaries), userID.Hex())
	summaries = append(summaries, groupSummaries...)

	log.Printf("Repo: Finished GetConversationSummaries for user %s. Total summaries: %d", userID.Hex(), len(summaries))
	return summaries, nil
}
