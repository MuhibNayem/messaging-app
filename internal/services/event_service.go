package services

import (
	"context"
	"errors"
	"time"

	"messaging-app/internal/cache"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// EventBroadcaster defines interface for broadcasting event updates
type EventBroadcaster interface {
	BroadcastRSVP(event models.EventRSVPEvent)
}

type EventService struct {
	eventRepo        *repositories.EventRepository
	userRepo         *repositories.UserRepository
	eventGraphRepo   *repositories.EventGraphRepository
	invitationRepo   *repositories.EventInvitationRepository
	postRepo         *repositories.EventPostRepository
	notificationRepo *repositories.NotificationRepository
	eventCache       *cache.EventCache
	broadcaster      EventBroadcaster
}

func NewEventService(
	eventRepo *repositories.EventRepository,
	userRepo *repositories.UserRepository,
	eventGraphRepo *repositories.EventGraphRepository,
	invitationRepo *repositories.EventInvitationRepository,
	postRepo *repositories.EventPostRepository,
	notificationRepo *repositories.NotificationRepository,
	eventCache *cache.EventCache,
	broadcaster EventBroadcaster,
) *EventService {
	return &EventService{
		eventRepo:        eventRepo,
		userRepo:         userRepo,
		eventGraphRepo:   eventGraphRepo,
		invitationRepo:   invitationRepo,
		postRepo:         postRepo,
		notificationRepo: notificationRepo,
		eventCache:       eventCache,
		broadcaster:      broadcaster,
	}
}

func (s *EventService) CreateEvent(ctx context.Context, userID primitive.ObjectID, req models.CreateEventRequest) (*models.Event, error) {
	event := &models.Event{
		Title:       req.Title,
		Description: req.Description,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Location:    req.Location,
		IsOnline:    req.IsOnline,
		Privacy:     req.Privacy,
		Category:    req.Category,
		CoverImage:  req.CoverImage,
		CreatorID:   userID,
	}

	// Creator is automatically going
	event.Attendees = []models.EventAttendee{
		{
			UserID:    userID,
			Status:    models.RSVPStatusGoing,
			Timestamp: time.Now(),
		},
	}
	event.Stats.GoingCount = 1

	if err := s.eventRepo.Create(ctx, event); err != nil {
		return nil, err
	}

	// Graph: Add Creator as Attendee
	if s.eventGraphRepo != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.eventGraphRepo.AddAttendee(ctx, userID, event.ID)
		}()
	}

	return event, nil
}

func (s *EventService) GetEvent(ctx context.Context, id primitive.ObjectID, viewerID primitive.ObjectID) (*models.EventResponse, error) {
	event, err := s.eventRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.mapToResponse(ctx, event, viewerID)
}

func (s *EventService) UpdateEvent(ctx context.Context, id, userID primitive.ObjectID, req models.UpdateEventRequest) (*models.EventResponse, error) {
	event, err := s.eventRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if event.CreatorID != userID {
		return nil, errors.New("unauthorized: only creator can update event")
	}

	if req.Title != "" {
		event.Title = req.Title
	}
	if req.Description != "" {
		event.Description = req.Description
	}
	if req.StartDate != nil {
		event.StartDate = *req.StartDate
	}
	if req.EndDate != nil {
		event.EndDate = *req.EndDate
	}
	if req.Location != "" {
		event.Location = req.Location
	}
	if req.IsOnline != nil {
		event.IsOnline = *req.IsOnline
	}
	if req.Privacy != "" {
		event.Privacy = req.Privacy
	}
	if req.Category != "" {
		event.Category = req.Category
	}
	if req.CoverImage != "" {
		event.CoverImage = req.CoverImage
	}

	if err := s.eventRepo.Update(ctx, event); err != nil {
		return nil, err
	}

	return s.mapToResponse(ctx, event, userID)
}

func (s *EventService) DeleteEvent(ctx context.Context, id, userID primitive.ObjectID) error {
	event, err := s.eventRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if event.CreatorID != userID {
		return errors.New("unauthorized")
	}

	return s.eventRepo.Delete(ctx, id)
}

func (s *EventService) ListEvents(ctx context.Context, userID primitive.ObjectID, limit, page int64, query, category, period string) ([]models.EventResponse, int64, error) {
	filter := bson.M{}

	// Privacy and Visibility
	// Show Public events OR Friend events (if logic implemented) OR Events I created/attending
	// complex visibility logic. For now, let's just return Public events + My events.
	// Or simpler: Just return public events by default for Discover.
	filter["privacy"] = models.EventPrivacyPublic

	if query != "" {
		filter["$text"] = bson.M{"$search": query} // Assumes text index, fallback to regex if none
	}

	if category != "" {
		filter["category"] = category
	}

	// Period: today, week, weekend
	now := time.Now()
	if period == "today" {
		tomorrow := now.Add(24 * time.Hour)
		filter["start_date"] = bson.M{"$gte": now, "$lt": tomorrow}
	} else if period == "week" {
		nextWeek := now.Add(7 * 24 * time.Hour)
		filter["start_date"] = bson.M{"$gte": now, "$lt": nextWeek}
	} else if period == "past" {
		filter["start_date"] = bson.M{"$lt": now}
	} else {
		// Default upcoming
		filter["start_date"] = bson.M{"$gte": now}
	}

	events, total, err := s.eventRepo.List(ctx, limit, page, filter)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]models.EventResponse, len(events))
	for i, event := range events {
		resp, _ := s.mapToResponse(ctx, &event, userID)
		responses[i] = *resp
	}

	return responses, total, nil
}

func (s *EventService) GetUserEvents(ctx context.Context, userID primitive.ObjectID, limit, page int64) ([]models.EventResponse, error) {
	events, err := s.eventRepo.GetUserEvents(ctx, userID, limit, page)
	if err != nil {
		return nil, err
	}

	responses := make([]models.EventResponse, len(events))
	for i, event := range events {
		resp, _ := s.mapToResponse(ctx, &event, userID)
		responses[i] = *resp
	}

	return responses, nil
}

func (s *EventService) GetFriendBirthdays(ctx context.Context, userID primitive.ObjectID) (*models.BirthdayResponse, error) {
	// 1. Get current user to find friends
	currentUser, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(currentUser.Friends) == 0 {
		return &models.BirthdayResponse{
			Today:    []models.BirthdayUser{},
			Upcoming: []models.BirthdayUser{},
		}, nil
	}

	// 2. Fetch friend birthdays efficiently
	todayUsers, upcomingUsers, err := s.userRepo.FindFriendBirthdays(ctx, currentUser.Friends)
	if err != nil {
		return nil, err
	}

	response := &models.BirthdayResponse{
		Today:    []models.BirthdayUser{},
		Upcoming: []models.BirthdayUser{},
	}

	now := time.Now()

	// process Today
	for _, f := range todayUsers {
		dob := *f.DateOfBirth
		age := now.Year() - dob.Year()
		// If today is birthday, age is exactly Year - Year

		response.Today = append(response.Today, models.BirthdayUser{
			ID:       f.ID.Hex(),
			Username: f.Username,
			FullName: f.FullName,
			Avatar:   f.Avatar,
			Age:      age,
			Date:     "Today",
		})
	}

	// process Upcoming
	for _, f := range upcomingUsers {
		dob := *f.DateOfBirth
		age := now.Year() - dob.Year()
		if now.YearDay() < dob.YearDay() {
			age--
		}
		// Age will be turning age? Usually users want "Turning X"
		// If birthday hasn't happened yet this year, they are age. On birthday they will be age+1.
		// Let's display the age they WILL be.
		age++

		// Format date
		thisYearBday := time.Date(now.Year(), dob.Month(), dob.Day(), 0, 0, 0, 0, now.Location())
		if thisYearBday.Before(now) {
			thisYearBday = thisYearBday.AddDate(1, 0, 0)
		}

		response.Upcoming = append(response.Upcoming, models.BirthdayUser{
			ID:       f.ID.Hex(),
			Username: f.Username,
			FullName: f.FullName,
			Avatar:   f.Avatar,
			Age:      age,
			Date:     thisYearBday.Format("January 02"),
		})
	}

	return response, nil
}

func (s *EventService) RSVP(ctx context.Context, eventID primitive.ObjectID, userID primitive.ObjectID, status models.RSVPStatus) error {
	_, err := s.eventRepo.GetByID(ctx, eventID)
	if err != nil {
		return err
	}

	// Update RSVP
	attendee := models.EventAttendee{
		UserID:    userID,
		Status:    status,
		Timestamp: time.Now(),
	}

	if err := s.eventRepo.AddOrUpdateAttendee(ctx, eventID, attendee); err != nil {
		return err
	}

	// Recalculate stats
	// This is heavy but mostly accurate.
	// Optimally we'd do this incrementally or async.
	updatedEvent, _ := s.eventRepo.GetByID(ctx, eventID)
	if updatedEvent != nil {
		var going, interested, invited int64
		for _, a := range updatedEvent.Attendees {
			switch a.Status {
			case models.RSVPStatusGoing:
				going++
			case models.RSVPStatusInterested:
				interested++
			case models.RSVPStatusInvited:
				invited++
			}
		}
		stats := models.EventStats{
			GoingCount:      going,
			InterestedCount: interested,
			InvitedCount:    invited,
			ShareCount:      updatedEvent.Stats.ShareCount,
		}
		s.eventRepo.UpdateStats(ctx, eventID, stats)

		// Cache the updated stats
		if s.eventCache != nil {
			s.eventCache.SetEventStats(ctx, eventID.Hex(), &stats)
			// Invalidate user's RSVP status cache
			s.eventCache.InvalidateUserRSVPStatus(ctx, userID.Hex(), eventID.Hex())
			// Cache the new RSVP status
			s.eventCache.SetUserRSVPStatus(ctx, userID.Hex(), eventID.Hex(), status)
		}

		// Broadcast RSVP update
		if s.broadcaster != nil {
			s.broadcaster.BroadcastRSVP(models.EventRSVPEvent{
				EventID:   eventID.Hex(),
				UserID:    userID.Hex(),
				Status:    status,
				Timestamp: time.Now(),
				Stats:     stats,
			})
		}
	}

	// Dual Write to Graph (if enabled)
	if s.eventGraphRepo != nil {
		go func() {
			// Run in background to not block main request
			// Ideally use a detached context or with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if status == models.RSVPStatusGoing {
				s.eventGraphRepo.AddAttendee(ctx, userID, eventID)
			} else {
				s.eventGraphRepo.RemoveAttendee(ctx, userID, eventID)
			}
		}()
	}

	return nil
}

func (s *EventService) mapToResponse(ctx context.Context, event *models.Event, viewerID primitive.ObjectID) (*models.EventResponse, error) {
	// Fetch Creator info
	creator, _ := s.userRepo.FindUserByID(ctx, event.CreatorID)
	creatorShort := models.UserShort{
		ID:       event.CreatorID.Hex(),
		Username: "Unknown",
	}
	if creator != nil {
		creatorShort.Username = creator.Username
		creatorShort.FullName = creator.FullName
		creatorShort.Avatar = creator.Avatar
	}

	// Determine MyStatus and IsHost
	var myStatus models.RSVPStatus
	for _, attendee := range event.Attendees {
		if attendee.UserID == viewerID {
			myStatus = attendee.Status
			break
		}
	}

	// Fetch friends going (from Neo4j)
	var friendsGoing []models.UserShort
	if s.eventGraphRepo != nil && !viewerID.IsZero() {
		friendIDs, err := s.eventGraphRepo.GetFriendsGoing(ctx, viewerID, event.ID)
		if err == nil && len(friendIDs) > 0 {
			// Limit to first 5 friends for display
			limit := 5
			if len(friendIDs) < limit {
				limit = len(friendIDs)
			}
			for i := 0; i < limit; i++ {
				friendOID, err := primitive.ObjectIDFromHex(friendIDs[i])
				if err != nil {
					continue
				}
				friend, err := s.userRepo.FindUserByID(ctx, friendOID)
				if err == nil && friend != nil {
					friendsGoing = append(friendsGoing, models.UserShort{
						ID:       friend.ID.Hex(),
						Username: friend.Username,
						FullName: friend.FullName,
						Avatar:   friend.Avatar,
					})
				}
			}
		}
	}

	return &models.EventResponse{
		ID:           event.ID.Hex(),
		Title:        event.Title,
		Description:  event.Description,
		StartDate:    event.StartDate,
		EndDate:      event.EndDate,
		Location:     event.Location,
		IsOnline:     event.IsOnline,
		Privacy:      event.Privacy,
		Category:     event.Category,
		CoverImage:   event.CoverImage,
		Creator:      creatorShort,
		Stats:        event.Stats,
		MyStatus:     myStatus,
		IsHost:       event.CreatorID == viewerID,
		FriendsGoing: friendsGoing,
		CreatedAt:    event.CreatedAt,
	}, nil
}

// ===============================
// Invitation Methods
// ===============================

// InviteFriends sends invitations to multiple friends
func (s *EventService) InviteFriends(ctx context.Context, eventID, inviterID primitive.ObjectID, friendIDs []string, message string) error {
	// Verify event exists and inviter has permission
	event, err := s.eventRepo.GetByID(ctx, eventID)
	if err != nil {
		return err
	}

	// Only creator, co-hosts, or going attendees can invite
	canInvite := event.CreatorID == inviterID
	if !canInvite {
		for _, coHost := range event.CoHosts {
			if coHost.UserID == inviterID {
				canInvite = true
				break
			}
		}
	}
	if !canInvite {
		for _, attendee := range event.Attendees {
			if attendee.UserID == inviterID && attendee.Status == models.RSVPStatusGoing {
				canInvite = true
				break
			}
		}
	}
	if !canInvite {
		return errors.New("unauthorized: you cannot invite to this event")
	}

	// Create invitations
	var invitations []models.EventInvitation
	for _, friendIDStr := range friendIDs {
		friendID, err := primitive.ObjectIDFromHex(friendIDStr)
		if err != nil {
			continue
		}

		// Check if already invited
		existing, _ := s.invitationRepo.CheckExisting(ctx, eventID, friendID)
		if existing != nil {
			continue
		}

		// Check if already attending
		isAttendee := false
		for _, a := range event.Attendees {
			if a.UserID == friendID {
				isAttendee = true
				break
			}
		}
		if isAttendee {
			continue
		}

		invitations = append(invitations, models.EventInvitation{
			EventID:   eventID,
			InviterID: inviterID,
			InviteeID: friendID,
			Message:   message,
		})
	}

	if len(invitations) > 0 {
		err := s.invitationRepo.CreateMany(ctx, invitations)
		if err != nil {
			return err
		}

		// Create notifications for each invitee
		if s.notificationRepo != nil {
			inviter, _ := s.userRepo.FindUserByID(ctx, inviterID)
			inviterUsername := "Someone"
			inviterAvatar := ""
			if inviter != nil {
				inviterUsername = inviter.Username
				inviterAvatar = inviter.Avatar
			}

			for _, inv := range invitations {
				notification := &models.Notification{
					RecipientID: inv.InviteeID,
					SenderID:    inviterID,
					Type:        models.NotificationTypeEventInvite,
					TargetID:    eventID,
					TargetType:  "event",
					Content:     inviterUsername + " invited you to " + event.Title,
					Data: map[string]interface{}{
						"event_id":        eventID.Hex(),
						"event_title":     event.Title,
						"sender_id":       inviterID.Hex(),
						"sender_username": inviterUsername,
						"sender_avatar":   inviterAvatar,
					},
					Read: false,
				}
				go func(n *models.Notification) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					s.notificationRepo.CreateNotification(ctx, n)
				}(notification)
			}
		}
	}

	return nil
}

// GetUserInvitations returns pending invitations for a user
func (s *EventService) GetUserInvitations(ctx context.Context, userID primitive.ObjectID, limit, page int64) ([]models.EventInvitationResponse, int64, error) {
	invitations, total, err := s.invitationRepo.GetUserInvitations(ctx, userID, models.InvitationStatusPending, limit, page)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]models.EventInvitationResponse, 0, len(invitations))
	for _, inv := range invitations {
		// Get event info
		event, err := s.eventRepo.GetByID(ctx, inv.EventID)
		if err != nil {
			continue
		}

		// Get inviter info
		inviter, _ := s.userRepo.FindUserByID(ctx, inv.InviterID)
		inviterShort := models.UserShort{ID: inv.InviterID.Hex(), Username: "Unknown"}
		if inviter != nil {
			inviterShort.Username = inviter.Username
			inviterShort.FullName = inviter.FullName
			inviterShort.Avatar = inviter.Avatar
		}

		responses = append(responses, models.EventInvitationResponse{
			ID: inv.ID.Hex(),
			Event: models.EventShort{
				ID:         event.ID.Hex(),
				Title:      event.Title,
				CoverImage: event.CoverImage,
				StartDate:  event.StartDate,
				Location:   event.Location,
			},
			Inviter:   inviterShort,
			Status:    inv.Status,
			Message:   inv.Message,
			CreatedAt: inv.CreatedAt,
		})
	}

	return responses, total, nil
}

// RespondToInvitation accepts or declines an invitation
func (s *EventService) RespondToInvitation(ctx context.Context, invitationID, userID primitive.ObjectID, accept bool) error {
	invitation, err := s.invitationRepo.GetByID(ctx, invitationID)
	if err != nil {
		return err
	}

	if invitation.InviteeID != userID {
		return errors.New("unauthorized")
	}

	if invitation.Status != models.InvitationStatusPending {
		return errors.New("invitation already responded")
	}

	// Get event info for notification
	event, err := s.eventRepo.GetByID(ctx, invitation.EventID)
	if err != nil {
		return err
	}

	var newStatus models.EventInvitationStatus
	if accept {
		newStatus = models.InvitationStatusAccepted
		// Add user as going
		if err := s.RSVP(ctx, invitation.EventID, userID, models.RSVPStatusGoing); err != nil {
			return err
		}
	} else {
		newStatus = models.InvitationStatusDeclined
	}

	if err := s.invitationRepo.UpdateStatus(ctx, invitationID, newStatus); err != nil {
		return err
	}

	// Create notification for the inviter
	if s.notificationRepo != nil {
		invitee, _ := s.userRepo.FindUserByID(ctx, userID)
		inviteeUsername := "Someone"
		inviteeAvatar := ""
		if invitee != nil {
			inviteeUsername = invitee.Username
			inviteeAvatar = invitee.Avatar
		}

		var notificationType models.NotificationType
		var content string
		if accept {
			notificationType = models.NotificationTypeEventInviteAccepted
			content = inviteeUsername + " accepted your invitation to " + event.Title
		} else {
			notificationType = models.NotificationTypeEventInviteDeclined
			content = inviteeUsername + " declined your invitation to " + event.Title
		}

		notification := &models.Notification{
			RecipientID: invitation.InviterID,
			SenderID:    userID,
			Type:        notificationType,
			TargetID:    invitation.EventID,
			TargetType:  "event",
			Content:     content,
			Data: map[string]interface{}{
				"event_id":        invitation.EventID.Hex(),
				"event_title":     event.Title,
				"sender_id":       userID.Hex(),
				"sender_username": inviteeUsername,
				"sender_avatar":   inviteeAvatar,
				"accepted":        accept,
			},
			Read: false,
		}
		go func(n *models.Notification) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.notificationRepo.CreateNotification(ctx, n)
		}(notification)
	}

	return nil
}

// ===============================
// Discussion/Post Methods
// ===============================

// CreatePost creates a discussion post on an event
func (s *EventService) CreatePost(ctx context.Context, eventID, authorID primitive.ObjectID, req models.CreateEventPostRequest) (*models.EventPostResponse, error) {
	// Verify event exists
	event, err := s.eventRepo.GetByID(ctx, eventID)
	if err != nil {
		return nil, err
	}

	// Check if user can post (attendee or host)
	canPost := event.CreatorID == authorID
	if !canPost {
		for _, a := range event.Attendees {
			if a.UserID == authorID && a.Status != models.RSVPStatusNotGoing {
				canPost = true
				break
			}
		}
	}
	if !canPost {
		return nil, errors.New("only attendees can post in event discussions")
	}

	post := &models.EventPost{
		EventID:   eventID,
		AuthorID:  authorID,
		Content:   req.Content,
		MediaURLs: req.MediaURLs,
	}

	if err := s.postRepo.Create(ctx, post); err != nil {
		return nil, err
	}

	// Get author info for response
	author, _ := s.userRepo.FindUserByID(ctx, authorID)
	authorShort := models.UserShort{ID: authorID.Hex(), Username: "Unknown"}
	if author != nil {
		authorShort.Username = author.Username
		authorShort.FullName = author.FullName
		authorShort.Avatar = author.Avatar
	}

	return &models.EventPostResponse{
		ID:        post.ID.Hex(),
		Author:    authorShort,
		Content:   post.Content,
		MediaURLs: post.MediaURLs,
		Reactions: []models.EventPostReactionResponse{},
		CreatedAt: post.CreatedAt,
	}, nil
}

// GetPosts returns discussion posts for an event
func (s *EventService) GetPosts(ctx context.Context, eventID primitive.ObjectID, limit, page int64) ([]models.EventPostResponse, int64, error) {
	posts, total, err := s.postRepo.GetByEventID(ctx, eventID, limit, page)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]models.EventPostResponse, 0, len(posts))
	for _, post := range posts {
		// Get author info
		author, _ := s.userRepo.FindUserByID(ctx, post.AuthorID)
		authorShort := models.UserShort{ID: post.AuthorID.Hex(), Username: "Unknown"}
		if author != nil {
			authorShort.Username = author.Username
			authorShort.FullName = author.FullName
			authorShort.Avatar = author.Avatar
		}

		// Map reactions
		reactions := make([]models.EventPostReactionResponse, 0, len(post.Reactions))
		for _, r := range post.Reactions {
			user, _ := s.userRepo.FindUserByID(ctx, r.UserID)
			userShort := models.UserShort{ID: r.UserID.Hex()}
			if user != nil {
				userShort.Username = user.Username
				userShort.Avatar = user.Avatar
			}
			reactions = append(reactions, models.EventPostReactionResponse{
				User:      userShort,
				Emoji:     r.Emoji,
				Timestamp: r.Timestamp,
			})
		}

		responses = append(responses, models.EventPostResponse{
			ID:        post.ID.Hex(),
			Author:    authorShort,
			Content:   post.Content,
			MediaURLs: post.MediaURLs,
			Reactions: reactions,
			CreatedAt: post.CreatedAt,
		})
	}

	return responses, total, nil
}

// DeletePost deletes a discussion post
func (s *EventService) DeletePost(ctx context.Context, eventID, postID, userID primitive.ObjectID) error {
	post, err := s.postRepo.GetByID(ctx, postID)
	if err != nil {
		return err
	}

	if post.EventID != eventID {
		return errors.New("post does not belong to this event")
	}

	// Only author or event host can delete
	event, _ := s.eventRepo.GetByID(ctx, eventID)
	if post.AuthorID != userID && event.CreatorID != userID {
		return errors.New("unauthorized")
	}

	return s.postRepo.Delete(ctx, postID)
}

// ReactToPost adds or updates a reaction on a post
func (s *EventService) ReactToPost(ctx context.Context, postID, userID primitive.ObjectID, emoji string) error {
	reaction := models.EventPostReaction{
		UserID:    userID,
		Emoji:     emoji,
		Timestamp: time.Now(),
	}
	return s.postRepo.AddReaction(ctx, postID, reaction)
}

// ===============================
// Attendees Methods
// ===============================

// GetAttendees returns attendees for an event with pagination
func (s *EventService) GetAttendees(ctx context.Context, eventID primitive.ObjectID, status models.RSVPStatus, limit, page int64) (*models.AttendeesListResponse, error) {
	event, err := s.eventRepo.GetByID(ctx, eventID)
	if err != nil {
		return nil, err
	}

	attendees, total, err := s.eventRepo.GetAttendeesByStatus(ctx, eventID, status, limit, page)
	if err != nil {
		return nil, err
	}

	responses := make([]models.EventAttendeeResponse, 0, len(attendees))
	for _, a := range attendees {
		user, _ := s.userRepo.FindUserByID(ctx, a.UserID)
		userShort := models.UserShort{ID: a.UserID.Hex(), Username: "Unknown"}
		if user != nil {
			userShort.Username = user.Username
			userShort.FullName = user.FullName
			userShort.Avatar = user.Avatar
		}

		isCoHost := false
		for _, ch := range event.CoHosts {
			if ch.UserID == a.UserID {
				isCoHost = true
				break
			}
		}

		responses = append(responses, models.EventAttendeeResponse{
			User:      userShort,
			Status:    a.Status,
			Timestamp: a.Timestamp,
			IsHost:    event.CreatorID == a.UserID,
			IsCoHost:  isCoHost,
		})
	}

	return &models.AttendeesListResponse{
		Attendees: responses,
		Total:     total,
		Page:      page,
		Limit:     limit,
	}, nil
}

// ===============================
// Co-Host Methods
// ===============================

// AddCoHost adds a co-host to an event
func (s *EventService) AddCoHost(ctx context.Context, eventID, userID, coHostID primitive.ObjectID) error {
	event, err := s.eventRepo.GetByID(ctx, eventID)
	if err != nil {
		return err
	}

	if event.CreatorID != userID {
		return errors.New("only the event creator can add co-hosts")
	}

	// Check if user is already a co-host
	for _, ch := range event.CoHosts {
		if ch.UserID == coHostID {
			return errors.New("user is already a co-host")
		}
	}

	coHost := models.EventCoHost{
		UserID:    coHostID,
		AddedAt:   time.Now(),
		AddedByID: userID,
	}

	return s.eventRepo.AddCoHost(ctx, eventID, coHost)
}

// RemoveCoHost removes a co-host from an event
func (s *EventService) RemoveCoHost(ctx context.Context, eventID, userID, coHostID primitive.ObjectID) error {
	event, err := s.eventRepo.GetByID(ctx, eventID)
	if err != nil {
		return err
	}

	if event.CreatorID != userID {
		return errors.New("only the event creator can remove co-hosts")
	}

	return s.eventRepo.RemoveCoHost(ctx, eventID, coHostID)
}

// ===============================
// Categories Methods
// ===============================

// GetCategories returns all event categories with counts
func (s *EventService) GetCategories(ctx context.Context) ([]models.EventCategory, error) {
	return s.eventRepo.GetCategories(ctx)
}

// ===============================
// Share Methods
// ===============================

// ShareEvent increments share count
func (s *EventService) ShareEvent(ctx context.Context, eventID primitive.ObjectID) error {
	return s.eventRepo.IncrementShareCount(ctx, eventID)
}

// ===============================
// Search Methods
// ===============================

// SearchEvents searches events with filters
func (s *EventService) SearchEvents(ctx context.Context, req models.SearchEventsRequest, userID primitive.ObjectID) ([]models.EventResponse, int64, error) {
	filter := bson.M{}

	// Privacy filter - show public events
	filter["privacy"] = models.EventPrivacyPublic

	// Category filter
	if req.Category != "" {
		filter["category"] = req.Category
	}

	// Online filter
	if req.Online != nil {
		filter["is_online"] = *req.Online
	}

	// Period filter
	now := time.Now()
	switch req.Period {
	case "today":
		tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		filter["start_date"] = bson.M{"$gte": now, "$lt": tomorrow}
	case "tomorrow":
		tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		dayAfter := tomorrow.AddDate(0, 0, 1)
		filter["start_date"] = bson.M{"$gte": tomorrow, "$lt": dayAfter}
	case "this_week":
		nextWeek := now.AddDate(0, 0, 7)
		filter["start_date"] = bson.M{"$gte": now, "$lt": nextWeek}
	case "this_weekend":
		// Find next Saturday
		daysUntilSat := (6 - int(now.Weekday()) + 7) % 7
		if daysUntilSat == 0 && now.Hour() >= 12 {
			daysUntilSat = 7
		}
		saturday := time.Date(now.Year(), now.Month(), now.Day()+daysUntilSat, 0, 0, 0, 0, now.Location())
		monday := saturday.AddDate(0, 0, 2)
		filter["start_date"] = bson.M{"$gte": saturday, "$lt": monday}
	default:
		// Default to upcoming
		filter["start_date"] = bson.M{"$gte": now}
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}

	events, total, err := s.eventRepo.Search(ctx, req.Query, filter, limit, page)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]models.EventResponse, 0, len(events))
	for _, event := range events {
		resp, _ := s.mapToResponse(ctx, &event, userID)
		if resp != nil {
			responses = append(responses, *resp)
		}
	}

	return responses, total, nil
}

// GetNearbyEvents returns events near a location
func (s *EventService) GetNearbyEvents(ctx context.Context, lat, lng, radiusKm float64, limit, page int64, userID primitive.ObjectID) ([]models.EventResponse, int64, error) {
	if radiusKm <= 0 {
		radiusKm = 50 // Default 50km radius
	}

	events, total, err := s.eventRepo.GetNearbyEvents(ctx, lat, lng, radiusKm, limit, page)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]models.EventResponse, 0, len(events))
	for _, event := range events {
		resp, _ := s.mapToResponse(ctx, &event, userID)
		if resp != nil {
			responses = append(responses, *resp)
		}
	}

	return responses, total, nil
}
