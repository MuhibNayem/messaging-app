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

type EventInvitationRepository struct {
	collection *mongo.Collection
}

func NewEventInvitationRepository(db *mongo.Database) *EventInvitationRepository {
	collection := db.Collection("event_invitations")

	// Create indexes for optimized invitation queries
	_, err := collection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		// Invitee + status index for user's pending invitations
		{
			Keys:    bson.D{{Key: "invitee_id", Value: 1}, {Key: "status", Value: 1}},
			Options: options.Index(),
		},
		// Event ID index for listing event invitations
		{
			Keys:    bson.D{{Key: "event_id", Value: 1}},
			Options: options.Index(),
		},
		// Inviter ID index
		{
			Keys:    bson.D{{Key: "inviter_id", Value: 1}},
			Options: options.Index(),
		},
		// Compound index for checking existing invitations
		{
			Keys:    bson.D{{Key: "event_id", Value: 1}, {Key: "invitee_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	})
	if err != nil {
		// Log but don't panic - indexes may already exist
	}

	return &EventInvitationRepository{
		collection: collection,
	}
}

// Create creates a new event invitation
func (r *EventInvitationRepository) Create(ctx context.Context, invitation *models.EventInvitation) error {
	invitation.CreatedAt = time.Now()
	invitation.UpdatedAt = time.Now()
	invitation.Status = models.InvitationStatusPending

	result, err := r.collection.InsertOne(ctx, invitation)
	if err != nil {
		return err
	}

	invitation.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

// CreateMany creates multiple invitations at once
func (r *EventInvitationRepository) CreateMany(ctx context.Context, invitations []models.EventInvitation) error {
	if len(invitations) == 0 {
		return nil
	}

	docs := make([]interface{}, len(invitations))
	now := time.Now()
	for i, inv := range invitations {
		inv.CreatedAt = now
		inv.UpdatedAt = now
		inv.Status = models.InvitationStatusPending
		docs[i] = inv
	}

	_, err := r.collection.InsertMany(ctx, docs)
	return err
}

// GetByID retrieves an invitation by ID
func (r *EventInvitationRepository) GetByID(ctx context.Context, id primitive.ObjectID) (*models.EventInvitation, error) {
	var invitation models.EventInvitation
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&invitation)
	if err != nil {
		return nil, err
	}
	return &invitation, nil
}

// GetUserInvitations gets all invitations for a user (as invitee)
func (r *EventInvitationRepository) GetUserInvitations(ctx context.Context, userID primitive.ObjectID, status models.EventInvitationStatus, limit, page int64) ([]models.EventInvitation, int64, error) {
	filter := bson.M{"invitee_id": userID}
	if status != "" {
		filter["status"] = status
	}

	skip := (page - 1) * limit
	opts := options.Find().
		SetLimit(limit).
		SetSkip(skip).
		SetSort(bson.M{"created_at": -1})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var invitations []models.EventInvitation
	if err = cursor.All(ctx, &invitations); err != nil {
		return nil, 0, err
	}

	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return invitations, total, nil
}

// GetEventInvitations gets all invitations for an event
func (r *EventInvitationRepository) GetEventInvitations(ctx context.Context, eventID primitive.ObjectID) ([]models.EventInvitation, error) {
	filter := bson.M{"event_id": eventID}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var invitations []models.EventInvitation
	if err = cursor.All(ctx, &invitations); err != nil {
		return nil, err
	}

	return invitations, nil
}

// UpdateStatus updates the status of an invitation
func (r *EventInvitationRepository) UpdateStatus(ctx context.Context, id primitive.ObjectID, status models.EventInvitationStatus) error {
	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"updated_at": time.Now(),
		},
	}

	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": id}, update)
	return err
}

// CheckExisting checks if an invitation already exists
func (r *EventInvitationRepository) CheckExisting(ctx context.Context, eventID, inviteeID primitive.ObjectID) (*models.EventInvitation, error) {
	var invitation models.EventInvitation
	err := r.collection.FindOne(ctx, bson.M{
		"event_id":   eventID,
		"invitee_id": inviteeID,
	}).Decode(&invitation)

	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &invitation, nil
}

// Delete removes an invitation
func (r *EventInvitationRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// DeleteByEventAndInvitee removes an invitation by event and invitee
func (r *EventInvitationRepository) DeleteByEventAndInvitee(ctx context.Context, eventID, inviteeID primitive.ObjectID) error {
	_, err := r.collection.DeleteOne(ctx, bson.M{
		"event_id":   eventID,
		"invitee_id": inviteeID,
	})
	return err
}
