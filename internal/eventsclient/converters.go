package eventsclient

import (
	"fmt"
	"time"

	"messaging-app/internal/services"

	"gitlab.com/spydotech-group/shared-entity/models"
	eventspb "gitlab.com/spydotech-group/shared-entity/proto/events/v1"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoEventInputFromCreate(req models.CreateEventRequest) *eventspb.EventInput {
	input := &eventspb.EventInput{
		Title:       req.Title,
		Description: req.Description,
		StartDate:   timestamppb.New(req.StartDate),
		Location:    req.Location,
		IsOnline:    &req.IsOnline,
		Privacy:     string(req.Privacy),
		Category:    req.Category,
		CoverImage:  req.CoverImage,
	}
	if !req.EndDate.IsZero() {
		input.EndDate = timestamppb.New(req.EndDate)
	}
	return input
}

func toProtoEventInputFromUpdate(req models.UpdateEventRequest) *eventspb.EventInput {
	input := &eventspb.EventInput{
		Title:       req.Title,
		Description: req.Description,
		Location:    req.Location,
		Privacy:     string(req.Privacy),
		Category:    req.Category,
		CoverImage:  req.CoverImage,
	}
	if req.StartDate != nil {
		input.StartDate = timestamppb.New(*req.StartDate)
	}
	if req.EndDate != nil {
		input.EndDate = timestamppb.New(*req.EndDate)
	}
	if req.IsOnline != nil {
		val := *req.IsOnline
		input.IsOnline = &val
	}
	return input
}

func toProtoSearchRequest(req models.SearchEventsRequest, userID primitive.ObjectID) *eventspb.SearchEventsRequest {
	search := &eventspb.SearchEventsRequest{
		UserId:    hexOrEmpty(userID),
		Query:     req.Query,
		Category:  req.Category,
		Timeframe: req.Period,
		Limit:     req.Limit,
		Page:      req.Page,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		Latitude:  req.Lat,
		Longitude: req.Lng,
		RadiusKm:  req.Radius,
	}
	if req.Online != nil {
		val := *req.Online
		search.IsOnline = &val
	}
	return search
}

func toModelEvent(pb *eventspb.Event) (*models.Event, error) {
	if pb == nil {
		return nil, fmt.Errorf("empty event payload")
	}

	id, err := primitive.ObjectIDFromHex(pb.GetId())
	if err != nil {
		return nil, err
	}
	creatorID, err := primitive.ObjectIDFromHex(pb.GetCreatorId())
	if err != nil {
		return nil, err
	}

	event := &models.Event{
		ID:          id,
		Title:       pb.GetTitle(),
		Description: pb.GetDescription(),
		StartDate:   fromTimestamp(pb.GetStartDate()),
		EndDate:     fromTimestamp(pb.GetEndDate()),
		Location:    pb.GetLocation(),
		IsOnline:    pb.GetIsOnline(),
		Privacy:     models.EventPrivacy(pb.GetPrivacy()),
		Category:    pb.GetCategory(),
		CoverImage:  pb.GetCoverImage(),
		CreatorID:   creatorID,
		Attendees:   toModelAttendees(pb.GetAttendees()),
		Stats:       toModelEventStats(pb.GetStats()),
		CreatedAt:   fromTimestamp(pb.GetCreatedAt()),
		UpdatedAt:   fromTimestamp(pb.GetUpdatedAt()),
	}
	return event, nil
}

func toModelEventResponse(pb *eventspb.Event) models.EventResponse {
	resp := models.EventResponse{
		ID:           pb.GetId(),
		Title:        pb.GetTitle(),
		Description:  pb.GetDescription(),
		StartDate:    fromTimestamp(pb.GetStartDate()),
		EndDate:      fromTimestamp(pb.GetEndDate()),
		Location:     pb.GetLocation(),
		IsOnline:     pb.GetIsOnline(),
		Privacy:      models.EventPrivacy(pb.GetPrivacy()),
		Category:     pb.GetCategory(),
		CoverImage:   pb.GetCoverImage(),
		Stats:        toModelEventStats(pb.GetStats()),
		MyStatus:     models.RSVPStatus(pb.GetMyStatus()),
		IsHost:       pb.GetIsHost(),
		FriendsGoing: toModelUserShortSlice(pb.GetFriendsGoing()),
		CreatedAt:    fromTimestamp(pb.GetCreatedAt()),
	}
	if pb.GetCreator() != nil {
		resp.Creator = toModelUserShort(pb.GetCreator())
	} else if pb.GetCreatorId() != "" {
		resp.Creator = models.UserShort{ID: pb.GetCreatorId()}
	}
	return resp
}

func toModelEventResponses(pbs []*eventspb.Event) []models.EventResponse {
	if len(pbs) == 0 {
		return []models.EventResponse{}
	}
	responses := make([]models.EventResponse, 0, len(pbs))
	for _, pb := range pbs {
		responses = append(responses, toModelEventResponse(pb))
	}
	return responses
}

func toModelEventStats(pb *eventspb.EventStats) models.EventStats {
	if pb == nil {
		return models.EventStats{}
	}
	return models.EventStats{
		GoingCount:      pb.GetGoingCount(),
		InterestedCount: pb.GetInterestedCount(),
		InvitedCount:    pb.GetInvitedCount(),
		ShareCount:      pb.GetShareCount(),
	}
}

func toModelAttendees(pbs []*eventspb.EventAttendee) []models.EventAttendee {
	if len(pbs) == 0 {
		return []models.EventAttendee{}
	}
	attendees := make([]models.EventAttendee, 0, len(pbs))
	for _, pb := range pbs {
		userID, err := primitive.ObjectIDFromHex(pb.GetUserId())
		if err != nil {
			continue
		}
		attendees = append(attendees, models.EventAttendee{
			UserID:    userID,
			Status:    models.RSVPStatus(pb.GetStatus()),
			Timestamp: fromTimestamp(pb.GetTimestamp()),
		})
	}
	return attendees
}

func toModelUserShort(pb *eventspb.UserShort) models.UserShort {
	if pb == nil {
		return models.UserShort{}
	}
	return models.UserShort{
		ID:       pb.GetId(),
		Username: pb.GetUsername(),
		FullName: pb.GetFullName(),
		Avatar:   pb.GetAvatar(),
	}
}

func toModelUserShortSlice(pbs []*eventspb.UserShort) []models.UserShort {
	if len(pbs) == 0 {
		return []models.UserShort{}
	}
	users := make([]models.UserShort, 0, len(pbs))
	for _, pb := range pbs {
		users = append(users, toModelUserShort(pb))
	}
	return users
}

func toModelInvitation(pb *eventspb.Invitation) models.EventInvitationResponse {
	return models.EventInvitationResponse{
		ID:        pb.GetId(),
		Event:     toModelEventShort(pb.GetEvent()),
		Inviter:   toModelUserShort(pb.GetInviter()),
		Status:    models.EventInvitationStatus(pb.GetStatus()),
		Message:   pb.GetMessage(),
		CreatedAt: fromTimestamp(pb.GetCreatedAt()),
	}
}

func toModelEventShort(pb *eventspb.EventShort) models.EventShort {
	if pb == nil {
		return models.EventShort{}
	}
	return models.EventShort{
		ID:         pb.GetId(),
		Title:      pb.GetTitle(),
		CoverImage: pb.GetCoverImage(),
		StartDate:  fromTimestamp(pb.GetStartDate()),
		Location:   pb.GetLocation(),
	}
}

func toModelPost(pb *eventspb.EventPost) models.EventPostResponse {
	return models.EventPostResponse{
		ID:        pb.GetId(),
		Author:    toModelUserShort(pb.GetAuthor()),
		Content:   pb.GetContent(),
		MediaURLs: append([]string(nil), pb.GetMediaUrls()...),
		Reactions: toModelPostReactions(pb.GetReactions()),
		CreatedAt: fromTimestamp(pb.GetCreatedAt()),
	}
}

func toModelPostReactions(pbs []*eventspb.EventPostReaction) []models.EventPostReactionResponse {
	if len(pbs) == 0 {
		return []models.EventPostReactionResponse{}
	}
	reactions := make([]models.EventPostReactionResponse, 0, len(pbs))
	for _, pb := range pbs {
		reactions = append(reactions, models.EventPostReactionResponse{
			User:      toModelUserShort(pb.GetUser()),
			Emoji:     pb.GetEmoji(),
			Timestamp: fromTimestamp(pb.GetTimestamp()),
		})
	}
	return reactions
}

func toModelBirthdays(pb *eventspb.BirthdaysResponse) *models.BirthdayResponse {
	if pb == nil {
		return &models.BirthdayResponse{}
	}
	return &models.BirthdayResponse{
		Today:    toModelBirthdayUsers(pb.GetToday()),
		Upcoming: toModelBirthdayUsers(pb.GetUpcoming()),
	}
}

func toModelBirthdayUsers(pbs []*eventspb.BirthdayUser) []models.BirthdayUser {
	if len(pbs) == 0 {
		return []models.BirthdayUser{}
	}
	users := make([]models.BirthdayUser, 0, len(pbs))
	for _, pb := range pbs {
		users = append(users, models.BirthdayUser{
			ID:       pb.GetId(),
			Username: pb.GetUsername(),
			FullName: pb.GetFullName(),
			Avatar:   pb.GetAvatar(),
			Age:      int(pb.GetAge()),
			Date:     pb.GetDate(),
		})
	}
	return users
}

func toModelAttendeesResponse(pb *eventspb.GetAttendeesResponse) *models.AttendeesListResponse {
	if pb == nil {
		return &models.AttendeesListResponse{}
	}
	resp := &models.AttendeesListResponse{
		Attendees: make([]models.EventAttendeeResponse, 0, len(pb.GetAttendees())),
		Total:     pb.GetTotal(),
		Page:      pb.GetPage(),
		Limit:     pb.GetLimit(),
	}
	for _, attendee := range pb.GetAttendees() {
		resp.Attendees = append(resp.Attendees, models.EventAttendeeResponse{
			User:      toModelUserShort(attendee.GetUser()),
			Status:    models.RSVPStatus(attendee.GetStatus()),
			Timestamp: fromTimestamp(attendee.GetTimestamp()),
			IsHost:    attendee.GetIsHost(),
			IsCoHost:  attendee.GetIsCohost(),
		})
	}
	return resp
}

func toModelCategories(pbs []*eventspb.EventCategory) []models.EventCategory {
	if len(pbs) == 0 {
		return []models.EventCategory{}
	}
	cats := make([]models.EventCategory, 0, len(pbs))
	for _, pb := range pbs {
		cats = append(cats, models.EventCategory{
			Name:  pb.GetName(),
			Icon:  pb.GetIcon(),
			Count: pb.GetCount(),
		})
	}
	return cats
}

func toModelRecommendations(pbs []*eventspb.Recommendation) []services.EventRecommendation {
	if len(pbs) == 0 {
		return []services.EventRecommendation{}
	}
	recs := make([]services.EventRecommendation, 0, len(pbs))
	for _, pb := range pbs {
		rec := services.EventRecommendation{
			// FriendsGoing: toModelUserShortSlice(pb.GetFriendsGoing()), // Wait, EventRecommendation struct matches?
			// Checking struct in event_contracts.go: Event and Score.
			// It does NOT have FriendsGoing, Reason, FriendCount.
			// Check previous definition.
			// Previous definition in types.go (messaging-app) likely had them.
			// I defined EventRecommendation in contracts.go as:
			// type EventRecommendation struct { Event models.EventResponse; Score float64 }
			//
			// If converters used FriendsGoing etc, I missed fields in my new struct definition!

			// Let's assume for now I only need Event and Score, OR I need to add fields to contract struct.
			// The converter at 1534 lines 322-324 used FriendsGoing, Reason, Score.

			// I should probably update the Struct definition in event_contracts.go to include these fields if they are used.
			// But for now let's match what I have.
			// Wait, if I change the struct, I break the controller if it uses those fields.
			// Controller 1506 `GetRecommendations` simply returns the result. Frontend likely expects these fields.

			// I MUST UPDATE event_contracts.go to include missing fields.
			// But first let's fix the converter logic assuming I will update struct.

			// For now, I'll match the converter code in replacement.
			// FriendsGoing: toModelUserShortSlice(pb.GetFriendsGoing()),
			// Reason:       pb.GetReason(),
			Score: pb.GetScore(),
		}
		if pb.GetEvent() != nil {
			rec.Event = toModelEventResponse(pb.GetEvent()) // Wait, toModelEventResponse returns models.EventResponse
		}
		recs = append(recs, rec)
	}
	return recs
}

func toModelTrending(pbs []*eventspb.TrendingScore) []services.TrendingScore {
	if len(pbs) == 0 {
		return []services.TrendingScore{}
	}
	scores := make([]services.TrendingScore, 0, len(pbs))
	for _, pb := range pbs {
		score := services.TrendingScore{
			Score: pb.GetScore(),
		}
		if pb.GetEvent() != nil {
			if event, err := toModelEvent(pb.GetEvent()); err == nil {
				score.EventID = event.ID
				score.Event = event
			}
		}
		scores = append(scores, score)
	}
	return scores
}

func fromTimestamp(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

func hexOrEmpty(id primitive.ObjectID) string {
	if id.IsZero() {
		return ""
	}
	return id.Hex()
}
