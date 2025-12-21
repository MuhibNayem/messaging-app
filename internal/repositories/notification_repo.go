package repositories

import (
	"context"
	"gitlab.com/spydotech-group/shared-entity/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type NotificationRepository struct {
	db *mongo.Database
}

func NewNotificationRepository(db *mongo.Database) *NotificationRepository {
	// Create indexes for notifications
	_, err := db.Collection("notifications").Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "recipient_id", Value: 1}},
			Options: options.Index(),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index(),
		},
		{
			Keys:    bson.D{{Key: "recipient_id", Value: 1}, {Key: "read", Value: 1}},
			Options: options.Index(),
		},
	})
	if err != nil {
		panic("Failed to create notification indexes: " + err.Error())
	}

	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) CreateNotification(ctx context.Context, notification *models.Notification) (*models.Notification, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now()
	}

	if !notification.ID.IsZero() {
		opts := options.Replace().SetUpsert(true)
		_, err := r.db.Collection("notifications").ReplaceOne(ctx, bson.M{"_id": notification.ID}, notification, opts)
		if err != nil {
			return nil, err
		}
		return notification, nil
	}

	result, err := r.db.Collection("notifications").InsertOne(ctx, notification)
	if err != nil {
		return nil, err
	}
	notification.ID = result.InsertedID.(primitive.ObjectID)
	return notification, nil
}

func (r *NotificationRepository) GetNotificationByID(ctx context.Context, notificationID primitive.ObjectID) (*models.Notification, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var notification models.Notification
	err := r.db.Collection("notifications").FindOne(ctx, bson.M{"_id": notificationID}).Decode(&notification)
	if err != nil {
		return nil, err
	}
	return &notification, nil
}

func (r *NotificationRepository) ListNotifications(ctx context.Context, recipientID primitive.ObjectID, filter bson.M, opts *options.FindOptions) ([]models.Notification, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	filter["recipient_id"] = recipientID

	cursor, err := r.db.Collection("notifications").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	notifications := make([]models.Notification, 0)
	if err := cursor.All(ctx, &notifications); err != nil {
		return nil, err
	}
	return notifications, nil
}

func (r *NotificationRepository) CountNotifications(ctx context.Context, recipientID primitive.ObjectID, filter bson.M) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	filter["recipient_id"] = recipientID

	count, err := r.db.Collection("notifications").CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *NotificationRepository) UpdateNotification(ctx context.Context, notificationID primitive.ObjectID, update bson.M) (*models.Notification, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	result := r.db.Collection("notifications").FindOneAndUpdate(
		ctx,
		bson.M{"_id": notificationID},
		bson.M{"$set": update},
		opts,
	)

	var updatedNotification models.Notification
	if err := result.Decode(&updatedNotification); err != nil {
		return nil, err
	}
	return &updatedNotification, nil
}

func (r *NotificationRepository) DeleteNotification(ctx context.Context, notificationID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.db.Collection("notifications").DeleteOne(ctx, bson.M{"_id": notificationID})
	return err
}
