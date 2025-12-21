package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"messaging-app/internal/db"
	"messaging-app/internal/kafka"
	"gitlab.com/spydotech-group/shared-entity/models"
	"messaging-app/internal/repositories"
	"time"

	"github.com/gocql/gocql"
	"github.com/redis/go-redis/v9"
	kafkago "github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type GroupService struct {
	groupRepo       *repositories.GroupRepository
	userRepo        *repositories.UserRepository
	activityRepo    *repositories.GroupActivityRepository
	cassandraClient *db.CassandraClient
	producer        *kafka.MessageProducer
	redisClient     *redis.ClusterClient
	groupGraphRepo  *repositories.GroupGraphRepository
}

func NewGroupService(groupRepo *repositories.GroupRepository, userRepo *repositories.UserRepository, activityRepo *repositories.GroupActivityRepository, cassandraClient *db.CassandraClient, producer *kafka.MessageProducer, redisClient *redis.ClusterClient, groupGraphRepo *repositories.GroupGraphRepository) *GroupService {
	return &GroupService{
		groupRepo:       groupRepo,
		userRepo:        userRepo,
		activityRepo:    activityRepo,
		cassandraClient: cassandraClient,
		producer:        producer,
		redisClient:     redisClient,
		groupGraphRepo:  groupGraphRepo,
	}
}

// invalidateActivityCache deletes the cached activities for a group
// Call this after any activity is created to ensure fresh data
func (s *GroupService) invalidateActivityCache(ctx context.Context, groupID primitive.ObjectID) {
	if s.redisClient != nil {
		cacheKey := "group_activities:" + groupID.Hex()
		s.redisClient.Del(ctx, cacheKey)
	}
}

// invalidateMembershipCache deletes the cached member set for a group
// Call this after any membership change (add/remove/leave)
func (s *GroupService) invalidateMembershipCache(ctx context.Context, groupID primitive.ObjectID) {
	if s.redisClient != nil {
		cacheKey := "group_members:" + groupID.Hex()
		s.redisClient.Del(ctx, cacheKey)
	}
}

// IsMember checks if a user is a member of a group (for IDOR authorization)
// Uses hybrid Redis cache + Neo4j graph for O(1) lookups
func (s *GroupService) IsMember(ctx context.Context, groupID, userID primitive.ObjectID) (bool, error) {
	cacheKey := "group_members:" + groupID.Hex()

	// 1. Try Redis SET membership check (O(1) lookup, sub-ms)
	if s.redisClient != nil {
		isMember, err := s.redisClient.SIsMember(ctx, cacheKey, userID.Hex()).Result()
		if err == nil {
			// Check if key exists (SIsMember returns false for non-existent keys)
			exists, _ := s.redisClient.Exists(ctx, cacheKey).Result()
			if exists > 0 {
				return isMember, nil
			}
		}
	}

	// 2. Cache miss - try Neo4j graph (O(1) pattern match)
	if s.groupGraphRepo != nil {
		isMember, err := s.groupGraphRepo.IsMember(ctx, userID, groupID)
		if err == nil {
			// Populate Redis cache async for future requests
			go s.populateMembersCacheFromNeo4j(groupID)
			return isMember, nil
		}
	}

	// 3. Fallback to MongoDB (defensive, should rarely hit)
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return false, err
	}

	// Populate Redis cache async
	if s.redisClient != nil && len(group.Members) > 0 {
		go func(key string, members []primitive.ObjectID) {
			memberHexes := make([]interface{}, len(members))
			for i, m := range members {
				memberHexes[i] = m.Hex()
			}
			s.redisClient.SAdd(context.Background(), key, memberHexes...)
			s.redisClient.Expire(context.Background(), key, 10*time.Minute)
		}(cacheKey, group.Members)
	}

	for _, memberID := range group.Members {
		if memberID == userID {
			return true, nil
		}
	}
	return false, nil
}

// populateMembersCacheFromNeo4j fetches members from Neo4j and caches in Redis
func (s *GroupService) populateMembersCacheFromNeo4j(groupID primitive.ObjectID) {
	if s.groupGraphRepo == nil || s.redisClient == nil {
		return
	}
	cacheKey := "group_members:" + groupID.Hex()
	members, err := s.groupGraphRepo.GetMembers(context.Background(), groupID)
	if err == nil && len(members) > 0 {
		memberInterfaces := make([]interface{}, len(members))
		for i, m := range members {
			memberInterfaces[i] = m
		}
		s.redisClient.SAdd(context.Background(), cacheKey, memberInterfaces...)
		s.redisClient.Expire(context.Background(), cacheKey, 10*time.Minute)
	}
}

func (s *GroupService) CreateGroup(ctx context.Context, creatorID primitive.ObjectID, name string, avatar string, memberIDs []primitive.ObjectID) (*models.Group, error) {
	// Verify creator exists
	if _, err := s.userRepo.FindUserByID(ctx, creatorID); err != nil {
		return nil, fmt.Errorf("creator user not found")
	}

	// Verify all members exist
	for _, memberID := range memberIDs {
		if _, err := s.userRepo.FindUserByID(ctx, memberID); err != nil {
			return nil, fmt.Errorf("member %s not found", memberID.Hex())
		}
	}

	// Include creator as admin and member
	members := append([]primitive.ObjectID{}, memberIDs...)
	if !containsID(members, creatorID) {
		members = append(members, creatorID)
	}

	group := &models.Group{
		Name:      name,
		Avatar:    avatar,
		CreatorID: creatorID,
		Members:   members,
		Admins:    []primitive.ObjectID{creatorID},
	}

	// Create group in repository (MongoDB - primary source of truth)
	createdGroup, err := s.groupRepo.CreateGroup(ctx, group)
	if err != nil {
		return nil, err
	}

	// Sync all members to Neo4j graph (async, non-blocking)
	if s.groupGraphRepo != nil {
		go s.groupGraphRepo.SyncAllMembers(context.Background(), createdGroup.ID, createdGroup.Members)
	}

	// Get creator details for activity
	creator, err := s.userRepo.FindUserByID(ctx, creatorID)
	if err != nil {
		fmt.Printf("Failed to fetch creator details: %v\n", err)
		// Continue without activity
	} else {
		// Create GROUP_CREATED activity
		activity := &models.GroupActivity{
			GroupID:      createdGroup.ID,
			ActivityType: models.ActivityGroupCreated,
			ActorID:      creatorID,
			ActorName:    creator.Username,
			CreatedAt:    time.Now(),
		}

		if err := s.activityRepo.CreateActivity(ctx, activity); err != nil {
			fmt.Printf("Failed to create group activity: %v\n", err)
		} else {
			// Invalidate cache to ensure fresh data is fetched
			s.invalidateActivityCache(ctx, createdGroup.ID)
		}

		// Update inbox for all members
		s.updateInboxForMembers(ctx, createdGroup, activity)
	}

	// Publish GROUP_CREATED event
	if err := s.publishGroupEvent(ctx, createdGroup.ID, "GROUP_CREATED"); err != nil {
		fmt.Printf("Failed to publish group created event: %v\n", err)
	}
	return createdGroup, nil
}

func (s *GroupService) GetGroup(ctx context.Context, id primitive.ObjectID) (*models.Group, error) {
	return s.groupRepo.GetGroup(ctx, id)
}

func (s *GroupService) AddMember(ctx context.Context, groupID, requesterID, newMemberID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	// Check if requester is admin
	if !containsID(group.Admins, requesterID) {
		return errors.New("only admins can add members")
	}

	// Check if user is already a member
	if containsID(group.Members, newMemberID) {
		return errors.New("user is already a group member")
	}

	// Verify new member exists
	newMember, err := s.userRepo.FindUserByID(ctx, newMemberID)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Add member to group (MongoDB - primary source of truth)
	if err := s.groupRepo.AddMember(ctx, groupID, newMemberID); err != nil {
		return err
	}

	// Sync to Neo4j graph (async, non-blocking)
	if s.groupGraphRepo != nil {
		go s.groupGraphRepo.AddMember(context.Background(), newMemberID, groupID)
	}

	// Get requester details for activity
	requester, err := s.userRepo.FindUserByID(ctx, requesterID)
	if err != nil {
		fmt.Printf("Failed to fetch requester details: %v\n", err)
		return nil // Member added successfully, activity creation is optional
	}

	// Create MEMBER_ADDED activity
	activity := &models.GroupActivity{
		GroupID:      groupID,
		ActivityType: models.ActivityMemberAdded,
		ActorID:      requesterID,
		ActorName:    requester.Username,
		TargetID:     &newMemberID,
		TargetName:   newMember.Username,
		CreatedAt:    time.Now(),
	}

	if err := s.activityRepo.CreateActivity(ctx, activity); err != nil {
		fmt.Printf("Failed to create member added activity: %v\n", err)
	} else {
		// Invalidate cache to ensure fresh data is fetched
		s.invalidateActivityCache(ctx, groupID)
	}

	// Update inbox for all members (including newly added member)
	updatedGroup, err := s.groupRepo.GetGroup(ctx, groupID)
	if err == nil {
		// Ensure new member is included in the update list even if DB read is stale
		if !containsID(updatedGroup.Members, newMemberID) {
			updatedGroup.Members = append(updatedGroup.Members, newMemberID)
		}
		s.updateInboxForMembers(ctx, updatedGroup, activity)
	}

	// Invalidate membership cache
	s.invalidateMembershipCache(ctx, groupID)

	return s.publishGroupEvent(ctx, groupID, "GROUP_UPDATED")
}

func (s *GroupService) AddAdmin(ctx context.Context, groupID, requesterID, newAdminID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	// Check if requester is admin
	if !containsID(group.Admins, requesterID) {
		return errors.New("only admins can add other admins")
	}

	// Check if user is already an admin
	if containsID(group.Admins, newAdminID) {
		return errors.New("user is already an admin")
	}

	// Check if user is a member
	if !containsID(group.Members, newAdminID) {
		return errors.New("user must be a member before becoming an admin")
	}

	return s.groupRepo.AddAdmin(ctx, groupID, newAdminID)
}

func (s *GroupService) RemoveMember(ctx context.Context, groupID, requesterID, memberID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	// Determine if this is a self-leave or admin removal
	isSelfLeave := requesterID == memberID

	// If not self-leave, check if requester is admin
	if !isSelfLeave && !containsID(group.Admins, requesterID) {
		return errors.New("only admins can remove members")
	}

	// Check if trying to remove last admin
	if containsID(group.Admins, memberID) && len(group.Admins) == 1 {
		return errors.New("cannot remove the last admin")
	}

	// Get member details before removal
	member, err := s.userRepo.FindUserByID(ctx, memberID)
	if err != nil {
		return fmt.Errorf("member not found")
	}

	// Remove member from group (MongoDB - primary source of truth)
	if err := s.groupRepo.RemoveMember(ctx, groupID, memberID); err != nil {
		return err
	}

	// Sync to Neo4j graph (async, non-blocking)
	if s.groupGraphRepo != nil {
		go s.groupGraphRepo.RemoveMember(context.Background(), memberID, groupID)
	}

	// Create activity based on who is removing
	var activity *models.GroupActivity
	if isSelfLeave {
		// Member left on their own
		activity = &models.GroupActivity{
			GroupID:      groupID,
			ActivityType: models.ActivityMemberLeft,
			ActorID:      memberID,
			ActorName:    member.Username,
			CreatedAt:    time.Now(),
		}
	} else {
		// Admin removed the member
		requester, err := s.userRepo.FindUserByID(ctx, requesterID)
		if err != nil {
			fmt.Printf("Failed to fetch requester details: %v\n", err)
			return nil // Member removed successfully, activity creation is optional
		}

		activity = &models.GroupActivity{
			GroupID:      groupID,
			ActivityType: models.ActivityMemberRemoved,
			ActorID:      requesterID,
			ActorName:    requester.Username,
			TargetID:     &memberID,
			TargetName:   member.Username,
			CreatedAt:    time.Now(),
		}
	}

	if err := s.activityRepo.CreateActivity(ctx, activity); err != nil {
		fmt.Printf("Failed to create member removal activity: %v\n", err)
	} else {
		// Invalidate cache to ensure fresh data is fetched
		s.invalidateActivityCache(ctx, groupID)
	}

	// Update inbox for remaining members AND the removed member
	updatedGroup, err := s.groupRepo.GetGroup(ctx, groupID)
	if err == nil {
		// Explicitly add the removed member to the list so their inbox gets updated too
		updatedGroup.Members = append(updatedGroup.Members, memberID)
		s.updateInboxForMembers(ctx, updatedGroup, activity)
	}

	// Invalidate membership cache
	s.invalidateMembershipCache(ctx, groupID)

	return s.publishGroupEvent(ctx, groupID, "GROUP_UPDATED")
}

func (s *GroupService) UpdateGroup(ctx context.Context, groupID, requesterID primitive.ObjectID, updates map[string]interface{}) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	// Check if requester is admin
	if !containsID(group.Admins, requesterID) {
		return errors.New("only admins can update group")
	}

	// Filter allowed fields to update
	allowedFields := map[string]bool{
		"name":       true,
		"avatar":     true,
		"updated_at": true,
	}

	filteredUpdates := bson.M{}
	for key, value := range updates {
		if allowedFields[key] {
			filteredUpdates[key] = value
		}
	}

	if len(filteredUpdates) == 0 {
		return errors.New("no valid fields to update")
	}

	if err := s.groupRepo.UpdateGroup(ctx, groupID, filteredUpdates); err != nil {
		return err
	}

	// Fetch updated group to broadcast
	updatedGroup, err := s.groupRepo.GetGroup(ctx, groupID)
	if err == nil {
		// Broadcast update
		event := map[string]interface{}{
			"type": "GROUP_UPDATED",
			"data": updatedGroup,
		}

		eventBytes, err := json.Marshal(event)
		if err == nil {
			msg := kafkago.Message{
				Key:   []byte(groupID.Hex()),
				Value: eventBytes,
			}
			if err := s.producer.ProduceMessage(ctx, msg); err != nil {
				fmt.Printf("Failed to publish group update event: %v\n", err)
			}
		}
	}

	return nil
}

func (s *GroupService) GetActivities(ctx context.Context, groupID primitive.ObjectID, requesterID primitive.ObjectID, limit int) ([]*models.GroupActivity, error) {
	// Verify group exists
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("group not found")
	}

	// Check if requester is a member
	if !containsID(group.Members, requesterID) {
		return nil, errors.New("only group members can view activities")
	}

	// Fetch activities from repository
	return s.activityRepo.GetActivities(ctx, groupID, limit)
}

func (s *GroupService) GetUserGroups(ctx context.Context, userID primitive.ObjectID) ([]*models.Group, error) {
	if _, err := s.userRepo.FindUserByID(ctx, userID); err != nil {
		return nil, fmt.Errorf("user not found")
	}
	groups, err := s.groupRepo.GetUserGroups(ctx, userID)
	return groups, err
}

func (s *GroupService) InviteMember(ctx context.Context, groupID, inviterID, inviteeID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	// Check if inviter is member
	if !containsID(group.Members, inviterID) {
		return errors.New("inviter must be a group member")
	}

	// Check if invitee is already member
	if containsID(group.Members, inviteeID) {
		return errors.New("user is already a member")
	}
	// Check if invitee is already pending
	if containsID(group.PendingMembers, inviteeID) {
		return errors.New("user is already pending approval")
	}

	// Logic:
	// If Inviter is Admin -> Add Immediate
	// If Settings.RequiresApproval is FALSE -> Add Immediate
	// Else -> Add Pending

	isInviterAdmin := containsID(group.Admins, inviterID)
	requiresApproval := group.Settings.RequiresApproval

	if isInviterAdmin || !requiresApproval {
		return s.groupRepo.AddMember(ctx, groupID, inviteeID)
	}

	// Add to pending
	return s.groupRepo.AddPendingMember(ctx, groupID, inviteeID)
}

func (s *GroupService) ApproveMember(ctx context.Context, groupID, adminID, targetUserID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	if !containsID(group.Admins, adminID) {
		return errors.New("only admins can approve members")
	}

	if !containsID(group.PendingMembers, targetUserID) {
		return errors.New("user is not in pending list")
	}

	// Remove from pending
	if err := s.groupRepo.RemovePendingMember(ctx, groupID, targetUserID); err != nil {
		return err
	}
	// Add to members
	if err := s.groupRepo.AddMember(ctx, groupID, targetUserID); err != nil {
		return err
	}

	// Sync to Neo4j graph (async, non-blocking)
	if s.groupGraphRepo != nil {
		go s.groupGraphRepo.AddMember(context.Background(), targetUserID, groupID)
	}

	// Create MEMBER_ADDED activity
	// We need admin details (actor) and target user details
	adminUser, err := s.userRepo.FindUserByID(ctx, adminID)
	if err == nil {
		targetUser, err := s.userRepo.FindUserByID(ctx, targetUserID)
		if err == nil {
			activity := &models.GroupActivity{
				GroupID:      groupID,
				ActivityType: models.ActivityMemberAdded, // Or specific "APPROVED"? standardizing on ADDED for now
				ActorID:      adminID,
				ActorName:    adminUser.Username,
				TargetID:     &targetUserID,
				TargetName:   targetUser.Username,
				CreatedAt:    time.Now(),
			}

			if err := s.activityRepo.CreateActivity(ctx, activity); err != nil {
				fmt.Printf("Failed to create member approved activity: %v\n", err)
			} else {
				s.invalidateActivityCache(ctx, groupID)
			}

			// Update inbox for all members (including newly added member)
			updatedGroup, err := s.groupRepo.GetGroup(ctx, groupID)
			if err == nil {
				// Ensure new member is included even if DB read stale
				if !containsID(updatedGroup.Members, targetUserID) {
					updatedGroup.Members = append(updatedGroup.Members, targetUserID)
				}
				s.updateInboxForMembers(ctx, updatedGroup, activity)
			}
		}
	}

	// Invalidate membership cache
	s.invalidateMembershipCache(ctx, groupID)

	return s.publishGroupEvent(ctx, groupID, "GROUP_UPDATED")
}

func (s *GroupService) RejectMember(ctx context.Context, groupID, adminID, targetUserID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	if !containsID(group.Admins, adminID) {
		return errors.New("only admins can reject members")
	}

	if err := s.groupRepo.RemovePendingMember(ctx, groupID, targetUserID); err != nil {
		return err
	}

	return s.publishGroupEvent(ctx, groupID, "GROUP_UPDATED")
}

func (s *GroupService) RemoveAdmin(ctx context.Context, groupID, requesterID, adminID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	if !containsID(group.Admins, requesterID) {
		return errors.New("only admins can remove admins")
	}

	if len(group.Admins) <= 1 {
		return errors.New("cannot remove the last admin")
	}

	// To remove admin (demote), we assume the repository has a RemoveAdmin method
	// NOTE: existing RemoveMember removes from BOTH.
	// We need a specific RemoveAdmin (demote) repo function or custom update.
	// Since we can't change Repo structure easily repeatedly, let's implement demote logic here via generic Update if possible?
	// But Repo UpdateGroup is generic.
	// Better: Add RemoveAdminRole to Repo?
	// Or use generic UpdateGroup with $pull from admins array.
	// Actually GroupRepo has UpdateGroup. we can use that.

	// Use the dedicated RemoveAdmin repository method
	if err := s.groupRepo.RemoveAdmin(ctx, groupID, adminID); err != nil {
		return err
	}

	// Publish update event
	return s.publishGroupEvent(ctx, groupID, "GROUP_UPDATED")
}

func (s *GroupService) publishGroupEvent(ctx context.Context, groupID primitive.ObjectID, eventType string) error {
	// 1. Fetch latest group state
	updatedGroup, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("failed to fetch group for broadcast: %w", err)
	}

	// 2. Fetch User Details for Enrichment
	// We need to construct models.GroupResponse

	// Helper to fetch user details safely
	getUserShort := func(uid primitive.ObjectID) (models.UserShortResponse, error) {
		u, err := s.userRepo.FindUserByID(ctx, uid)
		if err != nil {
			return models.UserShortResponse{}, err
		}
		return models.UserShortResponse{
			ID:       u.ID,
			Username: u.Username,
			Email:    u.Email,
			Avatar:   u.Avatar,
		}, nil
	}

	// A. Creator
	creator, err := getUserShort(updatedGroup.CreatorID)
	if err != nil {
		// Log error but proceed? Or fail? Better to fail or send partial?
		// For broadcast, maybe better to proceed with empty or fail.
		// Let's try to be robust.
		fmt.Printf("Error fetching creator for broadcast: %v\n", err)
	}

	// B. Members
	var members []models.UserShortResponse
	for _, mid := range updatedGroup.Members {
		if u, err := getUserShort(mid); err == nil {
			members = append(members, u)
		}
	}

	// C. Pending Members
	var pendingMembers []models.UserShortResponse
	for _, pid := range updatedGroup.PendingMembers {
		if u, err := getUserShort(pid); err == nil {
			pendingMembers = append(pendingMembers, u)
		}
	}

	// D. Admins
	var admins []models.UserShortResponse
	for _, aid := range updatedGroup.Admins {
		if u, err := getUserShort(aid); err == nil {
			admins = append(admins, u)
		}
	}

	// 3. Construct Response
	response := models.GroupResponse{
		ID:             updatedGroup.ID,
		Name:           updatedGroup.Name,
		Avatar:         updatedGroup.Avatar,
		Creator:        creator,
		Members:        members,
		PendingMembers: pendingMembers,
		Admins:         admins,
		Settings:       updatedGroup.Settings,
		CreatedAt:      updatedGroup.CreatedAt,
		UpdatedAt:      updatedGroup.UpdatedAt,
	}

	// 4. Publish event to Kafka
	// Log the admins list (enriched) to verify
	// fmt.Printf("Broadcasting ENRICHED %s for group %s. Admins Count: %d\n", eventType, updatedGroup.ID.Hex(), len(admins))

	eventPayload := map[string]interface{}{
		"type": eventType, // Dynamic event type
		"data": response,  // The enriched response object
	}

	eventBytes, err := json.Marshal(eventPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal group event: %w", err)
	}

	// Produce to "feed" topic which ws.go consumes
	msg := kafkago.Message{
		Key:   []byte(groupID.Hex()),
		Value: eventBytes,
		Time:  time.Now(),
	}

	if err := s.producer.ProduceMessage(ctx, msg); err != nil {
		fmt.Printf("Failed to publish group event: %v\n", err)
		return err
	}
	return nil
}

func (s *GroupService) UpdateGroupSettings(ctx context.Context, groupID, requesterID primitive.ObjectID, settings models.GroupSettings) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	if !containsID(group.Admins, requesterID) {
		return errors.New("only admins can update settings")
	}

	if err := s.groupRepo.UpdateGroupSettings(ctx, groupID, settings); err != nil {
		return err
	}

	return s.publishGroupEvent(ctx, groupID, "GROUP_UPDATED")
}

// updateInboxForMembers updates user_inbox for all group members with activity
// Uses Cassandra BATCH for O(1) network roundtrip (Facebook-scale optimization)
func (s *GroupService) updateInboxForMembers(ctx context.Context, group *models.Group, activity *models.GroupActivity) {
	if len(group.Members) == 0 {
		return
	}

	fmt.Printf("[DEBUG] updateInboxForMembers started for GroupID=%s, ActivityType=%s, MembersCount=%d\n",
		group.ID.Hex(), activity.ActivityType, len(group.Members))

	activityText := activity.FormatActivity()
	conversationID := "group_" + group.ID.Hex()
	now := time.Now()

	query := `INSERT INTO user_inbox (
		user_id, conversation_id, conversation_name, conversation_avatar,
		is_group, is_marketplace, last_message_content,
		last_message_sender_id, last_message_sender_name, last_message_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// Use UnloggedBatch for maximum performance
	// Unlogged is safe here because these are independent writes to different partitions
	// CHUNKING: Split into batches of 50 to support groups with 10k+ members
	// Cassandra recommends keeping batches < 5KB or < 100 statements
	batchSize := 50
	totalUpdated := 0

	for i := 0; i < len(group.Members); i += batchSize {
		end := i + batchSize
		if end > len(group.Members) {
			end = len(group.Members)
		}

		batch := s.cassandraClient.Session.NewBatch(gocql.UnloggedBatch)
		chunk := group.Members[i:end]

		for _, memberID := range chunk {
			batch.Query(query,
				memberID.Hex(),
				conversationID,
				group.Name,
				group.Avatar,
				true,  // is_group
				false, // is_marketplace
				activityText,
				activity.ActorID.Hex(),
				activity.ActorName,
				now,
			)
		}

		// Execute chunk
		if err := s.cassandraClient.Session.ExecuteBatch(batch); err != nil {
			fmt.Printf("[ERROR] Failed to execute batch chunk %d-%d for group %s: %v\n", i, end, group.ID.Hex(), err)
		} else {
			totalUpdated += len(chunk)
		}
	}

	fmt.Printf("[DEBUG] Successfully updated inbox for %d/%d members (Chunked)\n", totalUpdated, len(group.Members))
}

func containsID(ids []primitive.ObjectID, id primitive.ObjectID) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}
