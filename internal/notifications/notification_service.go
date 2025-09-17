package notifications

import (
	"context"
	"errors"
	"fmt"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type NotificationService struct {
	notificationRepo *repositories.NotificationRepository
	userRepo         *repositories.UserRepository
}

func NewNotificationService(nr *repositories.NotificationRepository, ur *repositories.UserRepository) *NotificationService {
	return &NotificationService{
		notificationRepo: nr,
		userRepo:         ur,
	}
}

func (s *NotificationService) CreateNotification(ctx context.Context, req *models.CreateNotificationRequest) (*models.Notification, error) {
	// Basic validation: ensure recipient and sender exist
	if _, err := s.userRepo.FindUserByID(ctx, req.RecipientID); err != nil {
		return nil, errors.New("recipient user not found")
	}
	if _, err := s.userRepo.FindUserByID(ctx, req.SenderID); err != nil {
		return nil, errors.New("sender user not found")
	}

	notification := &models.Notification{
		RecipientID: req.RecipientID,
		SenderID:    req.SenderID,
		Type:        req.Type,
		TargetID:    req.TargetID,
		TargetType:  req.TargetType,
		Content:     req.Content,
		Read:        false, // New notifications are unread by default
	}

	createdNotification, err := s.notificationRepo.CreateNotification(ctx, notification)
	if err != nil {
		return nil, fmt.Errorf("failed to create notification: %w", err)
	}

	return createdNotification, nil
}

func (s *NotificationService) GetNotificationByID(ctx context.Context, notificationID primitive.ObjectID) (*models.Notification, error) {
	return s.notificationRepo.GetNotificationByID(ctx, notificationID)
}

func (s *NotificationService) ListNotifications(ctx context.Context, userID primitive.ObjectID, page, limit int64, readStatus *bool) (*models.NotificationListResponse, error) {
	filter := bson.M{}
	if readStatus != nil {
		filter["read"] = *readStatus
	}

	opts := options.Find().
		SetSkip((page - 1) * limit).
		SetLimit(limit).
		SetSort(bson.D{{Key: "created_at", Value: -1}})

	notifications, err := s.notificationRepo.ListNotifications(ctx, userID, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list notifications: %w", err)
	}

	total, err := s.notificationRepo.CountNotifications(ctx, userID, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count notifications: %w", err)
	}

	return &models.NotificationListResponse{
		Notifications: notifications,
		Total:         total,
		Page:          page,
		Limit:         limit,
	}, nil
}

func (s *NotificationService) MarkNotificationAsRead(ctx context.Context, notificationID, userID primitive.ObjectID) error {
	notification, err := s.notificationRepo.GetNotificationByID(ctx, notificationID)
	if err != nil {
		return errors.New("notification not found")
	}

	if notification.RecipientID != userID {
		return errors.New("unauthorized to mark this notification as read")
	}

	update := bson.M{"read": true}
	_, err = s.notificationRepo.UpdateNotification(ctx, notificationID, update)
	if err != nil {
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}

	return nil
}

func (s *NotificationService) DeleteNotification(ctx context.Context, notificationID, userID primitive.ObjectID) error {
	notification, err := s.notificationRepo.GetNotificationByID(ctx, notificationID)
	if err != nil {
		return errors.New("notification not found")
	}

	if notification.RecipientID != userID {
		return errors.New("unauthorized to delete this notification")
	}

	return s.notificationRepo.DeleteNotification(ctx, notificationID)
}
