package services

import (
	"context"
	"errors"
	"time"

	"messaging-app/internal/models"
	"messaging-app/internal/repositories"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type EventService struct {
	eventRepo      *repositories.EventRepository
	userRepo       *repositories.UserRepository
	eventGraphRepo *repositories.EventGraphRepository
}

func NewEventService(eventRepo *repositories.EventRepository, userRepo *repositories.UserRepository, eventGraphRepo *repositories.EventGraphRepository) *EventService {
	return &EventService{
		eventRepo:      eventRepo,
		userRepo:       userRepo,
		eventGraphRepo: eventGraphRepo,
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

	return &models.EventResponse{
		ID:          event.ID.Hex(),
		Title:       event.Title,
		Description: event.Description,
		StartDate:   event.StartDate,
		EndDate:     event.EndDate,
		Location:    event.Location,
		IsOnline:    event.IsOnline,
		Privacy:     event.Privacy,
		Category:    event.Category,
		CoverImage:  event.CoverImage,
		Creator:     creatorShort,
		Stats:       event.Stats,
		MyStatus:    myStatus,
		IsHost:      event.CreatorID == viewerID,
		CreatedAt:   event.CreatedAt,
	}, nil
}
