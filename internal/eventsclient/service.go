package eventsclient

import (
	"context"

	"messaging-app/internal/services"

	"gitlab.com/spydotech-group/shared-entity/models"
	eventspb "gitlab.com/spydotech-group/shared-entity/proto/events/v1"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	_ services.EventServiceContract               = (*Client)(nil)
	_ services.EventRecommendationServiceContract = (*Client)(nil)
)

func (c *Client) CreateEvent(ctx context.Context, userID primitive.ObjectID, req models.CreateEventRequest) (*models.Event, error) {
	resp, err := c.client.CreateEvent(ctx, &eventspb.CreateEventRequest{
		UserId: userID.Hex(),
		Event:  toProtoEventInputFromCreate(req),
	})
	if err != nil {
		return nil, err
	}
	return toModelEvent(resp.GetEvent())
}

func (c *Client) GetEvent(ctx context.Context, id primitive.ObjectID, viewerID primitive.ObjectID) (*models.EventResponse, error) {
	resp, err := c.client.GetEvent(ctx, &eventspb.GetEventRequest{
		EventId:  id.Hex(),
		ViewerId: hexOrEmpty(viewerID),
	})
	if err != nil {
		return nil, err
	}
	event := toModelEventResponse(resp.GetEvent())
	return &event, nil
}

func (c *Client) UpdateEvent(ctx context.Context, id, userID primitive.ObjectID, req models.UpdateEventRequest) (*models.EventResponse, error) {
	resp, err := c.client.UpdateEvent(ctx, &eventspb.UpdateEventRequest{
		UserId:  userID.Hex(),
		EventId: id.Hex(),
		Event:   toProtoEventInputFromUpdate(req),
	})
	if err != nil {
		return nil, err
	}
	event := toModelEventResponse(resp.GetEvent())
	return &event, nil
}

func (c *Client) DeleteEvent(ctx context.Context, id, userID primitive.ObjectID) error {
	_, err := c.client.DeleteEvent(ctx, &eventspb.DeleteEventRequest{
		UserId:  userID.Hex(),
		EventId: id.Hex(),
	})
	return err
}

func (c *Client) ListEvents(ctx context.Context, userID primitive.ObjectID, limit, page int64, query, category, period string) ([]models.EventResponse, int64, error) {
	resp, err := c.client.ListEvents(ctx, &eventspb.ListEventsRequest{
		UserId:   hexOrEmpty(userID),
		Limit:    limit,
		Page:     page,
		Query:    query,
		Category: category,
		Period:   period,
	})
	if err != nil {
		return nil, 0, err
	}
	return toModelEventResponses(resp.GetEvents()), resp.GetTotal(), nil
}

func (c *Client) GetUserEvents(ctx context.Context, userID primitive.ObjectID, limit, page int64) ([]models.EventResponse, error) {
	resp, err := c.client.GetMyEvents(ctx, &eventspb.GetMyEventsRequest{
		UserId: userID.Hex(),
		Limit:  limit,
		Page:   page,
	})
	if err != nil {
		return nil, err
	}
	return toModelEventResponses(resp.GetEvents()), nil
}

func (c *Client) GetFriendBirthdays(ctx context.Context, userID primitive.ObjectID) (*models.BirthdayResponse, error) {
	resp, err := c.client.GetBirthdays(ctx, &eventspb.GetBirthdaysRequest{
		UserId: userID.Hex(),
	})
	if err != nil {
		return nil, err
	}
	return toModelBirthdays(resp), nil
}

func (c *Client) RSVP(ctx context.Context, eventID primitive.ObjectID, userID primitive.ObjectID, status models.RSVPStatus) error {
	_, err := c.client.RSVP(ctx, &eventspb.RSVPRequest{
		UserId:  userID.Hex(),
		EventId: eventID.Hex(),
		Status:  string(status),
	})
	return err
}

func (c *Client) InviteFriends(ctx context.Context, eventID, inviterID primitive.ObjectID, friendIDs []string, message string) error {
	_, err := c.client.InviteFriends(ctx, &eventspb.InviteFriendsRequest{
		UserId:    inviterID.Hex(),
		EventId:   eventID.Hex(),
		FriendIds: friendIDs,
		Message:   message,
	})
	return err
}

func (c *Client) GetUserInvitations(ctx context.Context, userID primitive.ObjectID, limit, page int64) ([]models.EventInvitationResponse, int64, error) {
	resp, err := c.client.GetInvitations(ctx, &eventspb.GetInvitationsRequest{
		UserId: userID.Hex(),
		Limit:  limit,
		Page:   page,
	})
	if err != nil {
		return nil, 0, err
	}

	invitations := make([]models.EventInvitationResponse, 0, len(resp.GetInvitations()))
	for _, inv := range resp.GetInvitations() {
		invitations = append(invitations, toModelInvitation(inv))
	}
	return invitations, resp.GetTotal(), nil
}

func (c *Client) RespondToInvitation(ctx context.Context, invitationID, userID primitive.ObjectID, accept bool) error {
	_, err := c.client.RespondToInvitation(ctx, &eventspb.RespondToInvitationRequest{
		UserId:       userID.Hex(),
		InvitationId: invitationID.Hex(),
		Accept:       accept,
	})
	return err
}

func (c *Client) CreatePost(ctx context.Context, eventID, authorID primitive.ObjectID, req models.CreateEventPostRequest) (*models.EventPostResponse, error) {
	resp, err := c.client.CreatePost(ctx, &eventspb.CreatePostRequest{
		UserId:    authorID.Hex(),
		EventId:   eventID.Hex(),
		Content:   req.Content,
		MediaUrls: req.MediaURLs,
	})
	if err != nil {
		return nil, err
	}
	post := toModelPost(resp)
	return &post, nil
}

func (c *Client) GetPosts(ctx context.Context, eventID primitive.ObjectID, limit, page int64) ([]models.EventPostResponse, int64, error) {
	resp, err := c.client.GetPosts(ctx, &eventspb.GetPostsRequest{
		EventId: eventID.Hex(),
		Limit:   limit,
		Page:    page,
	})
	if err != nil {
		return nil, 0, err
	}

	posts := make([]models.EventPostResponse, 0, len(resp.GetPosts()))
	for _, pbPost := range resp.GetPosts() {
		posts = append(posts, toModelPost(pbPost))
	}
	return posts, resp.GetTotal(), nil
}

func (c *Client) DeletePost(ctx context.Context, eventID, postID, userID primitive.ObjectID) error {
	_, err := c.client.DeletePost(ctx, &eventspb.DeletePostRequest{
		UserId:  userID.Hex(),
		EventId: eventID.Hex(),
		PostId:  postID.Hex(),
	})
	return err
}

func (c *Client) ReactToPost(ctx context.Context, postID, userID primitive.ObjectID, emoji string) error {
	_, err := c.client.ReactToPost(ctx, &eventspb.ReactToPostRequest{
		UserId:   userID.Hex(),
		PostId:   postID.Hex(),
		Reaction: emoji,
	})
	return err
}

func (c *Client) GetAttendees(ctx context.Context, eventID primitive.ObjectID, status models.RSVPStatus, limit, page int64) (*models.AttendeesListResponse, error) {
	resp, err := c.client.GetAttendees(ctx, &eventspb.GetAttendeesRequest{
		EventId: eventID.Hex(),
		Status:  string(status),
		Limit:   limit,
		Page:    page,
	})
	if err != nil {
		return nil, err
	}
	return toModelAttendeesResponse(resp), nil
}

func (c *Client) AddCoHost(ctx context.Context, eventID, userID, coHostID primitive.ObjectID) error {
	_, err := c.client.AddCoHost(ctx, &eventspb.CoHostRequest{
		UserId:       userID.Hex(),
		EventId:      eventID.Hex(),
		TargetUserId: coHostID.Hex(),
	})
	return err
}

func (c *Client) RemoveCoHost(ctx context.Context, eventID, userID, coHostID primitive.ObjectID) error {
	_, err := c.client.RemoveCoHost(ctx, &eventspb.CoHostRequest{
		UserId:       userID.Hex(),
		EventId:      eventID.Hex(),
		TargetUserId: coHostID.Hex(),
	})
	return err
}

func (c *Client) GetCategories(ctx context.Context) ([]models.EventCategory, error) {
	resp, err := c.client.GetCategories(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return toModelCategories(resp.GetCategories()), nil
}

func (c *Client) SearchEvents(ctx context.Context, req models.SearchEventsRequest, userID primitive.ObjectID) ([]models.EventResponse, int64, error) {
	resp, err := c.client.SearchEvents(ctx, toProtoSearchRequest(req, userID))
	if err != nil {
		return nil, 0, err
	}
	return toModelEventResponses(resp.GetEvents()), resp.GetTotal(), nil
}

func (c *Client) ShareEvent(ctx context.Context, eventID primitive.ObjectID) error {
	_, err := c.client.ShareEvent(ctx, &eventspb.ShareEventRequest{
		EventId: eventID.Hex(),
	})
	return err
}

func (c *Client) GetNearbyEvents(ctx context.Context, lat, lng, radiusKm float64, limit, page int64, userID primitive.ObjectID) ([]models.EventResponse, int64, error) {
	resp, err := c.client.GetNearbyEvents(ctx, &eventspb.NearbyEventsRequest{
		Latitude:  lat,
		Longitude: lng,
		RadiusKm:  radiusKm,
		Limit:     limit,
		Page:      page,
		UserId:    hexOrEmpty(userID),
	})
	if err != nil {
		return nil, 0, err
	}
	return toModelEventResponses(resp.GetEvents()), resp.GetTotal(), nil
}

func (c *Client) GetRecommendations(ctx context.Context, userID primitive.ObjectID, limit int) ([]services.EventRecommendation, error) {
	resp, err := c.client.GetRecommendations(ctx, &eventspb.RecommendationRequest{
		UserId: userID.Hex(),
		Limit:  int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return toModelRecommendations(resp.GetRecommendations()), nil
}

func (c *Client) GetTrendingEvents(ctx context.Context, limit int) ([]services.TrendingScore, error) {
	resp, err := c.client.GetTrending(ctx, &eventspb.TrendingEventsRequest{
		Limit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return toModelTrending(resp.GetTrending()), nil
}
