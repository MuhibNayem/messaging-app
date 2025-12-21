package repositories

import (
	"context"
	"log"
	"gitlab.com/spydotech-group/shared-entity/models"
	"sort"

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
	summaries := make([]models.ConversationSummary, 0)

	// --- 1. Get Direct Message Conversations (Friends) ---
	log.Printf("Repo: Starting direct message aggregation for user %s", userID.Hex())
	friendshipsCursor, err := r.db.Collection("friendships").Aggregate(ctx, mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{
			"status": models.FriendshipStatusAccepted,
			"$or": []bson.M{
				{"requester_id": userID},
				{"receiver_id": userID},
			},
		}}},
		// Project the other user's ID
		bson.D{{Key: "$project", Value: bson.M{
			"_id": 0,
			"other_user_id": bson.M{
				"$cond": bson.A{bson.M{"$eq": bson.A{"$requester_id", userID}}, "$receiver_id", "$requester_id"},
			},
		}}},
		// Lookup user info for the other user
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "other_user_id",
			"foreignField": "_id",
			"as":           "user_info",
		}}},
		bson.D{{Key: "$unwind", Value: bson.M{"path": "$user_info", "preserveNullAndEmptyArrays": true}}},
		// Find the last message for this direct conversation (excluding marketplace messages)
		bson.D{{Key: "$lookup", Value: bson.M{
			"from": "messages",
			"let":  bson.M{"u1": userID, "u2": "$other_user_id"},
			"pipeline": bson.A{
				bson.M{"$match": bson.M{
					"$expr": bson.M{
						"$and": bson.A{
							bson.M{"$or": bson.A{
								bson.M{"$and": bson.A{
									bson.M{"$eq": bson.A{"$sender_id", "$$u1"}},
									bson.M{"$eq": bson.A{"$receiver_id", "$$u2"}},
								}},
								bson.M{"$and": bson.A{
									bson.M{"$eq": bson.A{"$sender_id", "$$u2"}},
									bson.M{"$eq": bson.A{"$receiver_id", "$$u1"}},
								}},
							}},
							// Exclude marketplace messages
							bson.M{"$ne": bson.A{"$is_marketplace", true}},
						},
					},
				}},
				bson.M{"$sort": bson.M{"created_at": -1}},
				bson.M{"$limit": 1},
			},
			"as": "last_message_dm",
		}}},
		bson.D{{Key: "$unwind", Value: bson.M{"path": "$last_message_dm", "preserveNullAndEmptyArrays": true}}},
		// Count unread messages for this direct conversation (excluding marketplace messages)
		bson.D{{Key: "$lookup", Value: bson.M{
			"from": "messages",
			"let":  bson.M{"u1": userID, "u2": "$other_user_id"},
			"pipeline": bson.A{
				bson.M{"$match": bson.M{
					"$expr": bson.M{
						"$and": bson.A{
							bson.M{"$or": bson.A{
								bson.M{"$and": bson.A{
									bson.M{"$eq": bson.A{"$sender_id", "$$u2"}},
									bson.M{"$eq": bson.A{"$receiver_id", "$$u1"}},
								}},
							}},
							bson.M{"$not": bson.M{"$in": bson.A{"$$u1", "$seen_by"}}},
							// Exclude marketplace messages
							bson.M{"$ne": bson.A{"$is_marketplace", true}},
						},
					},
				}},
				bson.M{"$count": "unread"},
			},
			"as": "unread_count_result",
		}}},
		// Project into ConversationSummary format
		bson.D{{Key: "$project", Value: bson.M{
			"_id":      "$user_info._id",
			"name":     "$user_info.username",
			"avatar":   "$user_info.avatar",
			"is_group": bson.M{"$literal": false},
			"last_message_content": bson.M{
				"$cond": bson.A{
					bson.M{"$and": bson.A{
						bson.M{"$ne": bson.A{"$last_message_dm.content", ""}},
						bson.M{"$ne": bson.A{"$last_message_dm.content", nil}},
					}},
					"$last_message_dm.content",
					bson.M{"$switch": bson.M{
						"branches": []bson.M{
							{"case": bson.M{"$eq": bson.A{"$last_message_dm.content_type", "image"}}, "then": "Sent a photo"},
							{"case": bson.M{"$eq": bson.A{"$last_message_dm.content_type", "video"}}, "then": "Sent a video"},
							{"case": bson.M{"$eq": bson.A{"$last_message_dm.content_type", "file"}}, "then": "Sent a file"},
							{"case": bson.M{"$eq": bson.A{"$last_message_dm.content_type", "multiple"}}, "then": "Sent multiple items"},
						},
						"default": "",
					}},
				},
			},
			"last_message_timestamp":    "$last_message_dm.created_at",
			"last_message_sender_id":    "$last_message_dm.sender_id",
			"last_message_is_encrypted": bson.M{"$ifNull": bson.A{"$last_message_dm.is_encrypted", false}},
			"unread_count": bson.M{
				"$ifNull": bson.A{
					bson.M{"$arrayElemAt": bson.A{"$unread_count_result.unread", 0}},
					0,
				},
			},
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
		bson.D{{Key: "$match", Value: bson.M{"members": userID}}},
		// Find the last message for this group conversation
		bson.D{{Key: "$lookup", Value: bson.M{
			"from": "messages",
			"let":  bson.M{"groupId": "$_id"},
			"pipeline": bson.A{
				bson.M{"$match": bson.M{
					"$expr": bson.M{"$eq": bson.A{"$group_id", "$$groupId"}},
				}},
				bson.M{"$sort": bson.M{"created_at": -1}},
				bson.M{"$limit": 1},
			},
			"as": "last_message_group",
		}}},
		bson.D{{Key: "$unwind", Value: bson.M{"path": "$last_message_group", "preserveNullAndEmptyArrays": true}}},
		// Lookup sender info for the last message
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "last_message_group.sender_id",
			"foreignField": "_id",
			"as":           "last_message_sender_info",
		}}},
		bson.D{{Key: "$unwind", Value: bson.M{"path": "$last_message_sender_info", "preserveNullAndEmptyArrays": true}}},
		// Count unread messages for this group conversation
		bson.D{{Key: "$lookup", Value: bson.M{
			"from": "messages",
			"let":  bson.M{"groupId": "$_id", "userId": userID},
			"pipeline": bson.A{
				bson.M{"$match": bson.M{
					"$expr": bson.M{
						"$and": bson.A{
							bson.M{"$eq": bson.A{"$group_id", "$$groupId"}},
							bson.M{"$not": bson.M{"$in": bson.A{"$$userId", "$seen_by"}}},
						},
					},
				}},
				bson.M{"$count": "unread"},
			},
			"as": "unread_count_result",
		}}},
		// Project into ConversationSummary format
		bson.D{{Key: "$project", Value: bson.M{
			"id":       "$_id",
			"name":     "$name",
			"avatar":   "$avatar",
			"is_group": bson.M{"$literal": true},
			"last_message_content": bson.M{
				"$cond": bson.A{
					bson.M{"$and": bson.A{
						bson.M{"$ne": bson.A{"$last_message_group.content", ""}},
						bson.M{"$ne": bson.A{"$last_message_group.content", nil}},
					}},
					"$last_message_group.content",
					bson.M{"$switch": bson.M{
						"branches": []bson.M{
							{"case": bson.M{"$eq": bson.A{"$last_message_group.content_type", "image"}}, "then": "Sent a photo"},
							{"case": bson.M{"$eq": bson.A{"$last_message_group.content_type", "video"}}, "then": "Sent a video"},
							{"case": bson.M{"$eq": bson.A{"$last_message_group.content_type", "file"}}, "then": "Sent a file"},
							{"case": bson.M{"$eq": bson.A{"$last_message_group.content_type", "multiple"}}, "then": "Sent multiple items"},
						},
						"default": "",
					}},
				},
			},
			"last_message_timestamp":    "$last_message_group.created_at",
			"last_message_sender_id":    "$last_message_group.sender_id",
			"last_message_sender_name":  "$last_message_sender_info.username",
			"last_message_is_encrypted": bson.M{"$ifNull": bson.A{"$last_message_group.is_encrypted", false}},
			"unread_count": bson.M{
				"$ifNull": bson.A{
					bson.M{"$arrayElemAt": bson.A{"$unread_count_result.unread", 0}},
					0,
				},
			},
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

	// Sort summaries by last message timestamp (descending)
	sort.Slice(summaries, func(i, j int) bool {
		t1 := summaries[i].LastMessageTimestamp
		t2 := summaries[j].LastMessageTimestamp
		if t1 == nil && t2 == nil {
			return false
		}
		if t1 == nil {
			return false // t1 is technically "older" (non-existent) so it should be after t2
		}
		if t2 == nil {
			return true // t2 is "older", so t1 comes first
		}
		return t1.After(*t2)
	})

	log.Printf("Repo: Finished GetConversationSummaries for user %s. Total summaries: %d", userID.Hex(), len(summaries))
	return summaries, nil
}
