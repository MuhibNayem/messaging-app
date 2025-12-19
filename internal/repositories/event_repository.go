package repositories

import (
	"context"
	"errors"
	"time"

	"messaging-app/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type EventRepository struct {
	collection *mongo.Collection
}

func NewEventRepository(db *mongo.Database) *EventRepository {
	return &EventRepository{
		collection: db.Collection("events"),
	}
}

func (r *EventRepository) Create(ctx context.Context, event *models.Event) error {
	event.CreatedAt = time.Now()
	event.UpdatedAt = time.Now()
	event.Stats = models.EventStats{
		GoingCount:      0,
		InterestedCount: 0,
		InvitedCount:    0,
	}
	// Initialize attendees as empty slice to avoid null in DB
	if event.Attendees == nil {
		event.Attendees = []models.EventAttendee{}
	}

	result, err := r.collection.InsertOne(ctx, event)
	if err != nil {
		return err
	}

	event.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *EventRepository) GetByID(ctx context.Context, id primitive.ObjectID) (*models.Event, error) {
	var event models.Event
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&event)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.New("event not found")
		}
		return nil, err
	}
	return &event, nil
}

func (r *EventRepository) Update(ctx context.Context, event *models.Event) error {
	event.UpdatedAt = time.Now()
	_, err := r.collection.ReplaceOne(ctx, bson.M{"_id": event.ID}, event)
	return err
}

func (r *EventRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *EventRepository) List(ctx context.Context, limit, page int64, filter bson.M) ([]models.Event, int64, error) {
	skip := (page - 1) * limit
	opts := options.Find().SetLimit(limit).SetSkip(skip).SetSort(bson.M{"start_date": 1}) // Sort by soonest

	if filter == nil {
		filter = bson.M{}
	}

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var events []models.Event
	if err = cursor.All(ctx, &events); err != nil {
		return nil, 0, err
	}

	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return events, total, nil
}

func (r *EventRepository) AddOrUpdateAttendee(ctx context.Context, eventID primitive.ObjectID, attendee models.EventAttendee) error {
	// First pull existing attendee record if any (to avoid duplicates or update status)
	// This is a bit inefficient (pull then push), but simpler given the document structure.
	// A better way for pure status update would be arrayFilters, but we might want to update timestamp too.
	// Actually, let's try to update if exists, otherwise push.

	filter := bson.M{"_id": eventID, "attendees.user_id": attendee.UserID}
	update := bson.M{
		"$set": bson.M{
			"attendees.$.status":    attendee.Status,
			"attendees.$.timestamp": attendee.Timestamp,
			"updated_at":            time.Now(),
		},
	}
	res, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		// Does not exist, push it
		update = bson.M{
			"$push": bson.M{"attendees": attendee},
			"$set":  bson.M{"updated_at": time.Now()},
		}
		_, err = r.collection.UpdateOne(ctx, bson.M{"_id": eventID}, update)
		return err
	}

	return nil
}

func (r *EventRepository) RemoveAttendee(ctx context.Context, eventID, userID primitive.ObjectID) error {
	filter := bson.M{"_id": eventID}
	update := bson.M{
		"$pull": bson.M{"attendees": bson.M{"user_id": userID}},
		"$set":  bson.M{"updated_at": time.Now()},
	}
	_, err := r.collection.UpdateOne(ctx, filter, update)
	return err
}

func (r *EventRepository) UpdateStats(ctx context.Context, eventID primitive.ObjectID, stats models.EventStats) error {
	// Usually stats should be re-calculated or atomically incremented.
	// For simplicity in this "best way" request, we might want to recalculate counts
	// but atomic $inc is better for concurrency.
	// Let's rely on the service to calculate delta or refetch counts.
	// For now, let's provide a SET for stats to sync them.

	update := bson.M{
		"$set": bson.M{
			"stats": stats,
		},
	}
	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": eventID}, update)
	return err
}

func (r *EventRepository) GetUserEvents(ctx context.Context, userID primitive.ObjectID, limit, page int64) ([]models.Event, error) {
	// Events where user is creator OR attendee (going/interested)
	filter := bson.M{
		"$or": []bson.M{
			{"creator_id": userID},
			{"attendees": bson.M{"$elemMatch": bson.M{"user_id": userID, "status": bson.M{"$ne": models.RSVPStatusNotGoing}}}},
		},
	}
	skip := (page - 1) * limit
	opts := options.Find().SetLimit(limit).SetSkip(skip).SetSort(bson.M{"start_date": -1}) // Past events relevant too

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var events []models.Event
	if err = cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	return events, nil
}
