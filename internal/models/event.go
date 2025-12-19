package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type EventPrivacy string

const (
	EventPrivacyPublic  EventPrivacy = "public"
	EventPrivacyPrivate EventPrivacy = "private"
	EventPrivacyFriends EventPrivacy = "friends"
)

type RSVPStatus string

const (
	RSVPStatusGoing      RSVPStatus = "going"
	RSVPStatusInterested RSVPStatus = "interested"
	RSVPStatusInvited    RSVPStatus = "invited"
	RSVPStatusNotGoing   RSVPStatus = "not_going"
)

type EventAttendee struct {
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	Status    RSVPStatus         `bson:"status" json:"status"`
	Timestamp time.Time          `bson:"timestamp" json:"timestamp"`
}

type Event struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title       string             `bson:"title" json:"title"`
	Description string             `bson:"description" json:"description"`
	StartDate   time.Time          `bson:"start_date" json:"start_date"`
	EndDate     time.Time          `bson:"end_date" json:"end_date"`
	Location    string             `bson:"location" json:"location"` // Simple string for now, could be GeoJSON later
	Coordinates []float64          `bson:"coordinates,omitempty" json:"coordinates,omitempty"`
	IsOnline    bool               `bson:"is_online" json:"is_online"`
	Privacy     EventPrivacy       `bson:"privacy" json:"privacy"`
	Category    string             `bson:"category" json:"category"`
	CoverImage  string             `bson:"cover_image" json:"cover_image"`
	CreatorID   primitive.ObjectID `bson:"creator_id" json:"creator_id"`
	Attendees   []EventAttendee    `bson:"attendees" json:"attendees"`
	Stats       EventStats         `bson:"stats" json:"stats"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

type EventStats struct {
	GoingCount      int64 `bson:"going_count" json:"going_count"`
	InterestedCount int64 `bson:"interested_count" json:"interested_count"`
	InvitedCount    int64 `bson:"invited_count" json:"invited_count"`
	ShareCount      int64 `bson:"share_count" json:"share_count"`
}

// APIs

type CreateEventRequest struct {
	Title       string       `json:"title" binding:"required"`
	Description string       `json:"description" binding:"required"`
	StartDate   time.Time    `json:"start_date" binding:"required"`
	EndDate     time.Time    `json:"end_date"` // Optional
	Location    string       `json:"location"`
	IsOnline    bool         `json:"is_online"`
	Privacy     EventPrivacy `json:"privacy" binding:"required,oneof=public private friends"`
	Category    string       `json:"category"`
	CoverImage  string       `json:"cover_image"`
}

type UpdateEventRequest struct {
	Title       string       `json:"title"`
	Description string       `json:"description"`
	StartDate   *time.Time   `json:"start_date"`
	EndDate     *time.Time   `json:"end_date"`
	Location    string       `json:"location"`
	IsOnline    *bool        `json:"is_online"`
	Privacy     EventPrivacy `json:"privacy,omitempty" binding:"omitempty,oneof=public private friends"`
	Category    string       `json:"category"`
	CoverImage  string       `json:"cover_image"`
}

type RSVPRequest struct {
	Status RSVPStatus `json:"status" binding:"required,oneof=going interested not_going"`
}

type EventResponse struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	StartDate   time.Time    `json:"start_date"`
	EndDate     time.Time    `json:"end_date"`
	Location    string       `json:"location"`
	IsOnline    bool         `json:"is_online"`
	Privacy     EventPrivacy `json:"privacy"`
	Category    string       `json:"category"`
	CoverImage  string       `json:"cover_image"`
	Creator     UserShort    `json:"creator"` // Reusing UserShort if available, or just ID/Name/Avatar
	Stats       EventStats   `json:"stats"`
	MyStatus    RSVPStatus   `json:"my_status,omitempty"` // User's RSVP status
	IsHost      bool         `json:"is_host"`
	CreatedAt   time.Time    `json:"created_at"`
}

type UserShort struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	FullName string `json:"full_name"`
	Avatar   string `json:"avatar"`
}

type BirthdayUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	FullName string `json:"full_name"`
	Avatar   string `json:"avatar"`
	Age      int    `json:"age"`
	Date     string `json:"date"` // "Today" or "April 20"
}

type BirthdayResponse struct {
	Today    []BirthdayUser `json:"today"`
	Upcoming []BirthdayUser `json:"upcoming"`
}
