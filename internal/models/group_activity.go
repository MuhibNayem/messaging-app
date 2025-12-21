package models

import (
	"time"

	"github.com/gocql/gocql"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ActivityType represents the type of group activity
type ActivityType string

const (
	ActivityGroupCreated  ActivityType = "CREATED"
	ActivityMemberAdded   ActivityType = "MEMBER_ADDED"
	ActivityMemberLeft    ActivityType = "MEMBER_LEFT"
	ActivityMemberRemoved ActivityType = "MEMBER_REMOVED"
	ActivityNameChanged   ActivityType = "NAME_CHANGED"
	ActivityAvatarChanged ActivityType = "AVATAR_CHANGED"
	ActivityAdminAdded    ActivityType = "ADMIN_ADDED"
	ActivityAdminRemoved  ActivityType = "ADMIN_REMOVED"
)

// GroupActivity represents a system activity/event in a group
type GroupActivity struct {
	GroupID      primitive.ObjectID  `json:"group_id"`
	ActivityID   gocql.UUID          `json:"activity_id"`
	ActivityType ActivityType        `json:"activity_type"`
	ActorID      primitive.ObjectID  `json:"actor_id"`
	ActorName    string              `json:"actor_name"`
	TargetID     *primitive.ObjectID `json:"target_id,omitempty"` // Optional: for MEMBER_ADDED/REMOVED
	TargetName   string              `json:"target_name,omitempty"`
	Metadata     string              `json:"metadata,omitempty"` // JSON for future extensibility
	CreatedAt    time.Time           `json:"created_at"`
}

// FormatActivity converts an activity to a human-readable string for inbox display
func (a *GroupActivity) FormatActivity() string {
	switch a.ActivityType {
	case ActivityGroupCreated:
		return a.ActorName + " created the group"
	case ActivityMemberAdded:
		if a.TargetName != "" {
			return a.ActorName + " added " + a.TargetName
		}
		return a.ActorName + " added a member"
	case ActivityMemberLeft:
		return a.ActorName + " left the group"
	case ActivityMemberRemoved:
		if a.TargetName != "" {
			return a.ActorName + " removed " + a.TargetName
		}
		return a.ActorName + " removed a member"
	case ActivityNameChanged:
		return a.ActorName + " changed the group name"
	case ActivityAvatarChanged:
		return a.ActorName + " changed the group photo"
	case ActivityAdminAdded:
		if a.TargetName != "" {
			return a.ActorName + " made " + a.TargetName + " an admin"
		}
		return a.ActorName + " added an admin"
	case ActivityAdminRemoved:
		if a.TargetName != "" {
			return a.ActorName + " removed " + a.TargetName + " as admin"
		}
		return a.ActorName + " removed an admin"
	default:
		return "Group activity"
	}
}
