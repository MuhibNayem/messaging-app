package services

import (
	"context"

	"gitlab.com/spydotech-group/shared-entity/models"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// EventServiceContract defines the functionality the HTTP layer expects for events.
type EventServiceContract interface {
	CreateEvent(ctx context.Context, userID primitive.ObjectID, req models.CreateEventRequest) (*models.Event, error)
	GetEvent(ctx context.Context, id primitive.ObjectID, viewerID primitive.ObjectID) (*models.EventResponse, error)
	UpdateEvent(ctx context.Context, id, userID primitive.ObjectID, req models.UpdateEventRequest) (*models.EventResponse, error)
	DeleteEvent(ctx context.Context, id, userID primitive.ObjectID) error
	ListEvents(ctx context.Context, userID primitive.ObjectID, limit, page int64, query, category, period string) ([]models.EventResponse, int64, error)
	GetUserEvents(ctx context.Context, userID primitive.ObjectID, limit, page int64) ([]models.EventResponse, error)
	GetFriendBirthdays(ctx context.Context, userID primitive.ObjectID) (*models.BirthdayResponse, error)
	RSVP(ctx context.Context, eventID primitive.ObjectID, userID primitive.ObjectID, status models.RSVPStatus) error
	InviteFriends(ctx context.Context, eventID, inviterID primitive.ObjectID, friendIDs []string, message string) error
	GetUserInvitations(ctx context.Context, userID primitive.ObjectID, limit, page int64) ([]models.EventInvitationResponse, int64, error)
	RespondToInvitation(ctx context.Context, invitationID, userID primitive.ObjectID, accept bool) error
	CreatePost(ctx context.Context, eventID, authorID primitive.ObjectID, req models.CreateEventPostRequest) (*models.EventPostResponse, error)
	GetPosts(ctx context.Context, eventID primitive.ObjectID, limit, page int64) ([]models.EventPostResponse, int64, error)
	DeletePost(ctx context.Context, eventID, postID, userID primitive.ObjectID) error
	ReactToPost(ctx context.Context, postID, userID primitive.ObjectID, emoji string) error
	GetAttendees(ctx context.Context, eventID primitive.ObjectID, status models.RSVPStatus, limit, page int64) (*models.AttendeesListResponse, error)
	AddCoHost(ctx context.Context, eventID, userID, coHostID primitive.ObjectID) error
	RemoveCoHost(ctx context.Context, eventID, userID, coHostID primitive.ObjectID) error
	GetCategories(ctx context.Context) ([]models.EventCategory, error)
	SearchEvents(ctx context.Context, req models.SearchEventsRequest, userID primitive.ObjectID) ([]models.EventResponse, int64, error)
	ShareEvent(ctx context.Context, eventID primitive.ObjectID) error
	GetNearbyEvents(ctx context.Context, lat, lng, radiusKm float64, limit, page int64, userID primitive.ObjectID) ([]models.EventResponse, int64, error)
}

type EventRecommendation struct {
	Event models.EventResponse `json:"event"`
	Score float64              `json:"score"`
}

type TrendingScore struct {
	EventID primitive.ObjectID `json:"event_id"`
	Score   float64            `json:"score"`
	Event   *models.Event      `json:"event,omitempty"`
}

// EventRecommendationServiceContract covers recommendation/trending endpoints.
type EventRecommendationServiceContract interface {
	GetRecommendations(ctx context.Context, userID primitive.ObjectID, limit int) ([]EventRecommendation, error)
	GetTrendingEvents(ctx context.Context, limit int) ([]TrendingScore, error)
}
