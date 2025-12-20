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
	collection := db.Collection("events")

	// Create indexes for optimized event queries
	_, err := collection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		// Creator index for "Your Events" queries
		{
			Keys:    bson.D{{Key: "creator_id", Value: 1}},
			Options: options.Index(),
		},
		// Start date index for chronological sorting
		{
			Keys:    bson.D{{Key: "start_date", Value: 1}},
			Options: options.Index(),
		},
		// Category + start date compound index for filtered discovery
		{
			Keys:    bson.D{{Key: "category", Value: 1}, {Key: "start_date", Value: 1}},
			Options: options.Index(),
		},
		// Privacy + start date for public event listings
		{
			Keys:    bson.D{{Key: "privacy", Value: 1}, {Key: "start_date", Value: 1}},
			Options: options.Index(),
		},
		// Attendee user ID for "events I'm attending" queries
		{
			Keys:    bson.D{{Key: "attendees.user_id", Value: 1}},
			Options: options.Index(),
		},
		// Attendee status for filtering going/interested
		{
			Keys:    bson.D{{Key: "attendees.status", Value: 1}},
			Options: options.Index(),
		},
		// Text index for event search
		{
			Keys:    bson.D{{Key: "title", Value: "text"}, {Key: "description", Value: "text"}},
			Options: options.Index().SetName("event_text_search"),
		},
	})
	if err != nil {
		// Log but don't panic - indexes may already exist
		// In production, consider logging this properly
	}

	return &EventRepository{
		collection: collection,
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

// GetAttendeesByStatus returns attendees filtered by status with pagination
func (r *EventRepository) GetAttendeesByStatus(ctx context.Context, eventID primitive.ObjectID, status models.RSVPStatus, limit, page int64) ([]models.EventAttendee, int64, error) {
	event, err := r.GetByID(ctx, eventID)
	if err != nil {
		return nil, 0, err
	}

	// Filter attendees by status
	var filtered []models.EventAttendee
	for _, a := range event.Attendees {
		if status == "" || a.Status == status {
			filtered = append(filtered, a)
		}
	}

	total := int64(len(filtered))

	// Apply pagination
	start := (page - 1) * limit
	end := start + limit
	if start > total {
		return []models.EventAttendee{}, total, nil
	}
	if end > total {
		end = total
	}

	return filtered[start:end], total, nil
}

// GetCategories returns distinct categories with counts
func (r *EventRepository) GetCategories(ctx context.Context) ([]models.EventCategory, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"start_date": bson.M{"$gte": time.Now()}}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$category",
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.M{"count": -1}}},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID    string `bson:"_id"`
		Count int64  `bson:"count"`
	}
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	categories := make([]models.EventCategory, len(results))
	for i, r := range results {
		categories[i] = models.EventCategory{
			Name:  r.ID,
			Count: r.Count,
		}
	}

	return categories, nil
}

// IncrementShareCount increments the share count for an event
func (r *EventRepository) IncrementShareCount(ctx context.Context, eventID primitive.ObjectID) error {
	update := bson.M{
		"$inc": bson.M{"stats.share_count": 1},
		"$set": bson.M{"updated_at": time.Now()},
	}
	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": eventID}, update)
	return err
}

// AddCoHost adds a co-host to an event
func (r *EventRepository) AddCoHost(ctx context.Context, eventID primitive.ObjectID, coHost models.EventCoHost) error {
	update := bson.M{
		"$push": bson.M{"co_hosts": coHost},
		"$set":  bson.M{"updated_at": time.Now()},
	}
	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": eventID}, update)
	return err
}

// RemoveCoHost removes a co-host from an event
func (r *EventRepository) RemoveCoHost(ctx context.Context, eventID, userID primitive.ObjectID) error {
	update := bson.M{
		"$pull": bson.M{"co_hosts": bson.M{"user_id": userID}},
		"$set":  bson.M{"updated_at": time.Now()},
	}
	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": eventID}, update)
	return err
}

// IsCoHost checks if a user is a co-host of the event
func (r *EventRepository) IsCoHost(ctx context.Context, eventID, userID primitive.ObjectID) (bool, error) {
	count, err := r.collection.CountDocuments(ctx, bson.M{
		"_id":              eventID,
		"co_hosts.user_id": userID,
	})
	return count > 0, err
}

// Search performs text search on events
func (r *EventRepository) Search(ctx context.Context, query string, filter bson.M, limit, page int64) ([]models.Event, int64, error) {
	if filter == nil {
		filter = bson.M{}
	}

	// Add text search if query provided
	if query != "" {
		// Use regex for partial matching if no text index
		filter["$or"] = []bson.M{
			{"title": bson.M{"$regex": query, "$options": "i"}},
			{"description": bson.M{"$regex": query, "$options": "i"}},
			{"location": bson.M{"$regex": query, "$options": "i"}},
		}
	}

	skip := (page - 1) * limit
	opts := options.Find().SetLimit(limit).SetSkip(skip).SetSort(bson.M{"start_date": 1})

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

// GetNearbyEvents finds events near a location (requires 2dsphere index on coordinates)
func (r *EventRepository) GetNearbyEvents(ctx context.Context, lat, lng, radiusKm float64, limit, page int64) ([]models.Event, int64, error) {
	// Convert km to meters for MongoDB $nearSphere
	radiusMeters := radiusKm * 1000

	filter := bson.M{
		"coordinates": bson.M{
			"$nearSphere": bson.M{
				"$geometry": bson.M{
					"type":        "Point",
					"coordinates": []float64{lng, lat}, // MongoDB uses [lng, lat] order
				},
				"$maxDistance": radiusMeters,
			},
		},
		"start_date": bson.M{"$gte": time.Now()},
	}

	skip := (page - 1) * limit
	opts := options.Find().SetLimit(limit).SetSkip(skip)

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		// Fall back to no location filter if geospatial fails
		return r.List(ctx, limit, page, bson.M{"start_date": bson.M{"$gte": time.Now()}})
	}
	defer cursor.Close(ctx)

	var events []models.Event
	if err = cursor.All(ctx, &events); err != nil {
		return nil, 0, err
	}

	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		total = int64(len(events))
	}

	return events, total, nil
}
