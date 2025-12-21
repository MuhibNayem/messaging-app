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
	CoHosts     []EventCoHost      `bson:"co_hosts" json:"co_hosts"`
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
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	StartDate    time.Time    `json:"start_date"`
	EndDate      time.Time    `json:"end_date"`
	Location     string       `json:"location"`
	IsOnline     bool         `json:"is_online"`
	Privacy      EventPrivacy `json:"privacy"`
	Category     string       `json:"category"`
	CoverImage   string       `json:"cover_image"`
	Creator      UserShort    `json:"creator"` // Reusing UserShort if available, or just ID/Name/Avatar
	Stats        EventStats   `json:"stats"`
	MyStatus     RSVPStatus   `json:"my_status,omitempty"` // User's RSVP status
	IsHost       bool         `json:"is_host"`
	FriendsGoing []UserShort  `json:"friends_going,omitempty"` // Friends who are going to this event
	CreatedAt    time.Time    `json:"created_at"`
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

// ===============================
// Event Invitation Models
// ===============================

type EventInvitationStatus string

const (
	InvitationStatusPending  EventInvitationStatus = "pending"
	InvitationStatusAccepted EventInvitationStatus = "accepted"
	InvitationStatusDeclined EventInvitationStatus = "declined"
)

type EventInvitation struct {
	ID        primitive.ObjectID    `bson:"_id,omitempty" json:"id"`
	EventID   primitive.ObjectID    `bson:"event_id" json:"event_id"`
	InviterID primitive.ObjectID    `bson:"inviter_id" json:"inviter_id"`
	InviteeID primitive.ObjectID    `bson:"invitee_id" json:"invitee_id"`
	Status    EventInvitationStatus `bson:"status" json:"status"`
	Message   string                `bson:"message,omitempty" json:"message,omitempty"`
	CreatedAt time.Time             `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time             `bson:"updated_at" json:"updated_at"`
}

type InviteFriendsRequest struct {
	FriendIDs []string `json:"friend_ids" binding:"required"`
	Message   string   `json:"message,omitempty"`
}

type InvitationRespondRequest struct {
	Accept bool `json:"accept"`
}

// EventRSVPEvent represents a WebSocket event for RSVP updates
type EventRSVPEvent struct {
	EventID   string     `json:"event_id"`
	UserID    string     `json:"user_id"`
	Status    RSVPStatus `json:"status"`
	Timestamp time.Time  `json:"timestamp"`
	Stats     EventStats `json:"stats,omitempty"` // Included to update counts
}

type EventInvitationResponse struct {
	ID        string                `json:"id"`
	Event     EventShort            `json:"event"`
	Inviter   UserShort             `json:"inviter"`
	Status    EventInvitationStatus `json:"status"`
	Message   string                `json:"message,omitempty"`
	CreatedAt time.Time             `json:"created_at"`
}

type EventShort struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	CoverImage string    `json:"cover_image"`
	StartDate  time.Time `json:"start_date"`
	Location   string    `json:"location"`
}

// ===============================
// Event Discussion/Posts Models
// ===============================

type EventPost struct {
	ID        primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	EventID   primitive.ObjectID  `bson:"event_id" json:"event_id"`
	AuthorID  primitive.ObjectID  `bson:"author_id" json:"author_id"`
	Content   string              `bson:"content" json:"content"`
	MediaURLs []string            `bson:"media_urls,omitempty" json:"media_urls,omitempty"`
	Reactions []EventPostReaction `bson:"reactions" json:"reactions"`
	CreatedAt time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time           `bson:"updated_at" json:"updated_at"`
}

type EventPostReaction struct {
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	Emoji     string             `bson:"emoji" json:"emoji"`
	Timestamp time.Time          `bson:"timestamp" json:"timestamp"`
}

type CreateEventPostRequest struct {
	Content   string   `json:"content" binding:"required"`
	MediaURLs []string `json:"media_urls,omitempty"`
}

type EventPostResponse struct {
	ID        string                      `json:"id"`
	Author    UserShort                   `json:"author"`
	Content   string                      `json:"content"`
	MediaURLs []string                    `json:"media_urls,omitempty"`
	Reactions []EventPostReactionResponse `json:"reactions"`
	CreatedAt time.Time                   `json:"created_at"`
}

type EventPostReactionResponse struct {
	User      UserShort `json:"user"`
	Emoji     string    `json:"emoji"`
	Timestamp time.Time `json:"timestamp"`
}

type ReactToPostRequest struct {
	Emoji string `json:"emoji" binding:"required"`
}

// ===============================
// Event Co-Host Models
// ===============================

type EventCoHost struct {
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	AddedAt   time.Time          `bson:"added_at" json:"added_at"`
	AddedByID primitive.ObjectID `bson:"added_by_id" json:"added_by_id"`
}

type AddCoHostRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

// ===============================
// Event Attendees Response
// ===============================

type EventAttendeeResponse struct {
	User      UserShort  `json:"user"`
	Status    RSVPStatus `json:"status"`
	Timestamp time.Time  `json:"timestamp"`
	IsHost    bool       `json:"is_host"`
	IsCoHost  bool       `json:"is_co_host"`
}

type AttendeesListResponse struct {
	Attendees []EventAttendeeResponse `json:"attendees"`
	Total     int64                   `json:"total"`
	Page      int64                   `json:"page"`
	Limit     int64                   `json:"limit"`
}

// ===============================
// Event Categories
// ===============================

type EventCategory struct {
	Name  string `json:"name"`
	Icon  string `json:"icon,omitempty"`
	Count int64  `json:"count"`
}

// ===============================
// Event Search
// ===============================

type SearchEventsRequest struct {
	Query     string  `form:"q"`
	Category  string  `form:"category"`
	Period    string  `form:"period"` // today, tomorrow, this_week, this_weekend, next_week
	StartDate string  `form:"start_date"`
	EndDate   string  `form:"end_date"`
	Lat       float64 `form:"lat"`
	Lng       float64 `form:"lng"`
	Radius    float64 `form:"radius"` // in km
	Online    *bool   `form:"online"`
	Page      int64   `form:"page"`
	Limit     int64   `form:"limit"`
}

// ===============================
// Enhanced Event Model with Co-Hosts
// ===============================

// Update Event struct to include CoHosts field (add to existing Event struct)
// CoHosts []EventCoHost `bson:"co_hosts" json:"co_hosts"`

// ===============================
// Event Notification Types
// ===============================

type EventNotificationType string

const (
	EventNotificationInvitation    EventNotificationType = "invitation"
	EventNotificationReminder      EventNotificationType = "reminder"
	EventNotificationUpdate        EventNotificationType = "update"
	EventNotificationNewPost       EventNotificationType = "new_post"
	EventNotificationFriendGoing   EventNotificationType = "friend_going"
	EventNotificationEventStarting EventNotificationType = "event_starting"
)
