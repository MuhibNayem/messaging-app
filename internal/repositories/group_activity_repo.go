package repositories

import (
	"context"
	"fmt"
	"log"
	"messaging-app/internal/db"
	"gitlab.com/spydotech-group/shared-entity/models"
	"sort"
	"time"

	"github.com/gocql/gocql"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type GroupActivityRepository struct {
	client *db.CassandraClient
}

func NewGroupActivityRepository(client *db.CassandraClient) *GroupActivityRepository {
	return &GroupActivityRepository{client: client}
}

// CreateActivity inserts a new group activity
func (r *GroupActivityRepository) CreateActivity(ctx context.Context, activity *models.GroupActivity) error {
	if r.client == nil || r.client.Session == nil {
		return fmt.Errorf("cassandra client not initialized")
	}

	// Generate TimeUUID for activity ID
	activity.ActivityID = gocql.TimeUUID()

	// Set created_at if not set
	if activity.CreatedAt.IsZero() {
		activity.CreatedAt = time.Now()
	}

	query := `INSERT INTO group_activities (
		group_id, activity_id, activity_type, actor_id, actor_name,
		target_id, target_name, metadata, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	targetID := ""
	if activity.TargetID != nil {
		targetID = activity.TargetID.Hex()
	}

	// DEBUG: Trace activity creation
	log.Printf("[DEBUG] Creating activity: Type=%s, GroupID=%s, Actor=%s, Target=%s",
		activity.ActivityType, activity.GroupID.Hex(), activity.ActorName, activity.TargetName)

	err := r.client.Session.Query(query,
		activity.GroupID.Hex(),
		activity.ActivityID,
		string(activity.ActivityType),
		activity.ActorID.Hex(),
		activity.ActorName,
		targetID,
		activity.TargetName,
		activity.Metadata,
		activity.CreatedAt,
	).Exec()

	if err != nil {
		log.Printf("[ERROR] Failed to insert group activity into Cassandra: %v", err)
		return err
	}

	return nil
}

// GetActivities retrieves activities for a group with pagination
func (r *GroupActivityRepository) GetActivities(ctx context.Context, groupID primitive.ObjectID, limit int) ([]*models.GroupActivity, error) {
	if r.client == nil || r.client.Session == nil {
		return nil, fmt.Errorf("cassandra client not initialized")
	}

	if limit <= 0 {
		limit = 50
	}

	query := `SELECT group_id, activity_id, activity_type, actor_id, actor_name,
	          target_id, target_name, metadata, created_at
	          FROM group_activities WHERE group_id = ? LIMIT ?`

	iter := r.client.Session.Query(query, groupID.Hex(), limit).Iter()

	// Pre-allocate with capacity to avoid slice growth allocations
	activities := make([]*models.GroupActivity, 0, limit)
	var gID, actorID, targetID, actorName, targetName, metadata string
	var activityType string
	var activityID gocql.UUID
	var createdAt time.Time

	for iter.Scan(&gID, &activityID, &activityType, &actorID, &actorName, &targetID, &targetName, &metadata, &createdAt) {
		parsedGroupID, _ := primitive.ObjectIDFromHex(gID)
		parsedActorID, _ := primitive.ObjectIDFromHex(actorID)

		var parsedTargetID *primitive.ObjectID
		if targetID != "" {
			tid, err := primitive.ObjectIDFromHex(targetID)
			if err == nil {
				parsedTargetID = &tid
			}
		}

		activities = append(activities, &models.GroupActivity{
			GroupID:      parsedGroupID,
			ActivityID:   activityID,
			ActivityType: models.ActivityType(activityType),
			ActorID:      parsedActorID,
			ActorName:    actorName,
			TargetID:     parsedTargetID,
			TargetName:   targetName,
			Metadata:     metadata,
			CreatedAt:    createdAt,
		})
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	// Sort activities by CreatedAt (oldest first) since Cassandra doesn't support ORDER BY with this schema
	sort.Slice(activities, func(i, j int) bool {
		return activities[i].CreatedAt.Before(activities[j].CreatedAt)
	})

	return activities, nil
}
