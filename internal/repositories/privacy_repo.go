package repositories

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"gitlab.com/spydotech-group/shared-entity/models"
)

// PrivacyRepository defines methods for interacting with privacy-related data
type PrivacyRepository interface {
	// User Privacy Settings
	CreateOrUpdateUserPrivacySettings(ctx context.Context, settings *models.UserPrivacySettings) error
	GetUserPrivacySettings(ctx context.Context, userID primitive.ObjectID) (*models.UserPrivacySettings, error)

	// Custom Privacy Lists
	CreateCustomPrivacyList(ctx context.Context, list *models.CustomPrivacyList) error
	GetCustomPrivacyListByID(ctx context.Context, listID primitive.ObjectID) (*models.CustomPrivacyList, error)
	GetCustomPrivacyListsByUserID(ctx context.Context, userID primitive.ObjectID) ([]models.CustomPrivacyList, error)
	UpdateCustomPrivacyList(ctx context.Context, listID primitive.ObjectID, update bson.M) error
	DeleteCustomPrivacyList(ctx context.Context, listID primitive.ObjectID) error

	// Custom Privacy List Members
	AddMemberToCustomPrivacyList(ctx context.Context, listID, memberUserID primitive.ObjectID) error
	RemoveMemberFromCustomPrivacyList(ctx context.Context, listID, memberUserID primitive.ObjectID) error
	IsUserInCustomPrivacyList(ctx context.Context, listID, userID primitive.ObjectID) (bool, error)
}

type privacyRepositoryMongo struct {
	userPrivacySettingsCollection *mongo.Collection
	customPrivacyListsCollection  *mongo.Collection
}

// NewPrivacyRepository creates a new instance of PrivacyRepository
func NewPrivacyRepository(db *mongo.Database) PrivacyRepository {
	return &privacyRepositoryMongo{
		userPrivacySettingsCollection: db.Collection("user_privacy_settings"),
		customPrivacyListsCollection:  db.Collection("custom_privacy_lists"),
	}
}

// CreateOrUpdateUserPrivacySettings implements PrivacyRepository
func (r *privacyRepositoryMongo) CreateOrUpdateUserPrivacySettings(ctx context.Context, settings *models.UserPrivacySettings) error {
	filter := bson.M{"user_id": settings.UserID}
	update := bson.M{"$set": settings}
	opts := options.Update().SetUpsert(true)
	_, err := r.userPrivacySettingsCollection.UpdateOne(ctx, filter, update, opts)
	return err
}

// GetUserPrivacySettings implements PrivacyRepository
func (r *privacyRepositoryMongo) GetUserPrivacySettings(ctx context.Context, userID primitive.ObjectID) (*models.UserPrivacySettings, error) {
	var settings models.UserPrivacySettings
	err := r.userPrivacySettingsCollection.FindOne(ctx, bson.M{"user_id": userID}).Decode(&settings)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Settings not found
		}
		return nil, err
	}
	return &settings, nil
}

// CreateCustomPrivacyList implements PrivacyRepository
func (r *privacyRepositoryMongo) CreateCustomPrivacyList(ctx context.Context, list *models.CustomPrivacyList) error {
	_, err := r.customPrivacyListsCollection.InsertOne(ctx, list)
	return err
}

// GetCustomPrivacyListByID implements PrivacyRepository
func (r *privacyRepositoryMongo) GetCustomPrivacyListByID(ctx context.Context, listID primitive.ObjectID) (*models.CustomPrivacyList, error) {
	var list models.CustomPrivacyList
	err := r.customPrivacyListsCollection.FindOne(ctx, bson.M{"_id": listID}).Decode(&list)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // List not found
		}
		return nil, err
	}
	return &list, nil
}

// GetCustomPrivacyListsByUserID implements PrivacyRepository
func (r *privacyRepositoryMongo) GetCustomPrivacyListsByUserID(ctx context.Context, userID primitive.ObjectID) ([]models.CustomPrivacyList, error) {
	cursor, err := r.customPrivacyListsCollection.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var lists []models.CustomPrivacyList
	if err = cursor.All(ctx, &lists); err != nil {
		return nil, err
	}
	return lists, nil
}

// UpdateCustomPrivacyList implements PrivacyRepository
func (r *privacyRepositoryMongo) UpdateCustomPrivacyList(ctx context.Context, listID primitive.ObjectID, update bson.M) error {
	update["$set"].(bson.M)["updated_at"] = time.Now()
	_, err := r.customPrivacyListsCollection.UpdateOne(ctx, bson.M{"_id": listID}, update)
	return err
}

// DeleteCustomPrivacyList implements PrivacyRepository
func (r *privacyRepositoryMongo) DeleteCustomPrivacyList(ctx context.Context, listID primitive.ObjectID) error {
	_, err := r.customPrivacyListsCollection.DeleteOne(ctx, bson.M{"_id": listID})
	return err
}

// AddMemberToCustomPrivacyList implements PrivacyRepository
func (r *privacyRepositoryMongo) AddMemberToCustomPrivacyList(ctx context.Context, listID, memberUserID primitive.ObjectID) error {
	update := bson.M{"$addToSet": bson.M{"members": memberUserID}}
	_, err := r.customPrivacyListsCollection.UpdateOne(ctx, bson.M{"_id": listID}, update)
	return err
}

// RemoveMemberFromCustomPrivacyList implements PrivacyRepository
func (r *privacyRepositoryMongo) RemoveMemberFromCustomPrivacyList(ctx context.Context, listID, memberUserID primitive.ObjectID) error {
	update := bson.M{"$pull": bson.M{"members": memberUserID}}
	_, err := r.customPrivacyListsCollection.UpdateOne(ctx, bson.M{"_id": listID}, update)
	return err
}

// IsUserInCustomPrivacyList implements PrivacyRepository
func (r *privacyRepositoryMongo) IsUserInCustomPrivacyList(ctx context.Context, listID, userID primitive.ObjectID) (bool, error) {
	filter := bson.M{"_id": listID, "members": userID}
	count, err := r.customPrivacyListsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
