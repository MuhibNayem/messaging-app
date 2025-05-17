package services

import (
	"context"
	"errors"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	userRepo *repositories.UserRepository
}

func NewUserService(userRepo *repositories.UserRepository) *UserService {
	return &UserService{userRepo: userRepo}
}

func (s *UserService) GetUserByID(ctx context.Context, id primitive.ObjectID) (*models.User, error) {
	return s.userRepo.FindUserByID(ctx, id)
}

func (s *UserService) UpdateUser(ctx context.Context, id primitive.ObjectID, update *models.UserUpdateRequest) (*models.User, error) {
	updateData := bson.M{
		"updated_at": time.Now(),
	}

	if update.Username != "" {
		existingUserUsername, _ := s.userRepo.FindUserByUserName(ctx, update.Username)
		if existingUserUsername != nil {
			return nil, errors.New("username already exists")
		}
		updateData["username"] = update.Username
	}

	if update.Email != "" {
		existingUserEmail, _ := s.userRepo.FindUserByEmail(ctx, update.Email)
		if existingUserEmail != nil {
			return nil, errors.New("user email already exists")
		}
		updateData["email"] = update.Email
	}

	// Only update password if new password provided
	if update.CurrentPassword != "" && update.NewPassword != "" {
		user, err := s.userRepo.FindUserByID(ctx, id)
		if err != nil {
			return nil, err
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(update.CurrentPassword)); err != nil {
			return nil, errors.New("current password is incorrect")
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(update.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		updateData["password"] = string(hashedPassword)
	}

	updatedUser, err := s.userRepo.UpdateUser(ctx, id, updateData)
	if err != nil {
		return nil, err
	}

	// Clear password before returning
	updatedUser.Password = ""
	return updatedUser, nil
}

func (s *UserService) ListUsers(ctx context.Context, page, limit int64, search string) (*models.UserListResponse, error) {
	filter := bson.M{}
	if search != "" {
		filter["$or"] = []bson.M{
			{"username": bson.M{"$regex": search, "$options": "i"}},
			{"email": bson.M{"$regex": search, "$options": "i"}},
		}
	}

	// Get total count
	total, err := s.userRepo.CountUsers(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Pagination options
	opts := options.Find().
		SetSkip((page - 1) * limit).
		SetLimit(limit).
		SetSort(bson.D{{Key: "username", Value: 1}})

	users, err := s.userRepo.FindUsers(ctx, filter, opts)
	if err != nil {
		return nil, err
	}

	// Clear passwords
	for i := range users {
		users[i].Password = ""
	}

	return &models.UserListResponse{
		Users: users,
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}