package controllers

import (
	"net/http"
	"strconv"

	"messaging-app/internal/services"

	"gitlab.com/spydotech-group/shared-entity/models"
	"gitlab.com/spydotech-group/shared-entity/utils"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type EventController struct {
	eventService          services.EventServiceContract
	recommendationService services.EventRecommendationServiceContract
}

func NewEventController(eventService services.EventServiceContract, recommendationService services.EventRecommendationServiceContract) *EventController {
	return &EventController{
		eventService:          eventService,
		recommendationService: recommendationService,
	}
}

// GetRecommendations returns personalized event recommendations for the user
func (c *EventController) GetRecommendations(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	limitStr := ctx.DefaultQuery("limit", "10")
	limit, _ := strconv.Atoi(limitStr)

	recommendations, err := c.recommendationService.GetRecommendations(ctx.Request.Context(), userID, limit)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	utils.RespondWithSuccess(ctx, recommendations)
}

// GetTrending returns trending events
func (c *EventController) GetTrending(ctx *gin.Context) {
	limitStr := ctx.DefaultQuery("limit", "10")
	limit, _ := strconv.Atoi(limitStr)

	trending, err := c.recommendationService.GetTrendingEvents(ctx.Request.Context(), limit)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	utils.RespondWithSuccess(ctx, trending)
}

func (c *EventController) CreateEvent(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	var req models.CreateEventRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	event, err := c.eventService.CreateEvent(ctx, userID, req)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusCreated, event)
}

func (c *EventController) GetEvent(ctx *gin.Context) {
	eventID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid event ID")
		return
	}

	userID, _ := utils.GetUserIDFromContext(ctx) // Optional

	response, err := c.eventService.GetEvent(ctx, eventID, userID)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, response)
}

func (c *EventController) UpdateEvent(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	eventID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid event ID")
		return
	}

	var req models.UpdateEventRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response, err := c.eventService.UpdateEvent(ctx, eventID, userID, req)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, response)
}

func (c *EventController) DeleteEvent(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	eventID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid event ID")
		return
	}

	if err := c.eventService.DeleteEvent(ctx, eventID, userID); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.Status(http.StatusOK)
}

func (c *EventController) ListEvents(ctx *gin.Context) {
	page, _ := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(ctx.DefaultQuery("limit", "10"), 10, 64)
	query := ctx.Query("q")
	category := ctx.Query("category")
	period := ctx.Query("period") // today, week, past

	userID, _ := utils.GetUserIDFromContext(ctx)

	events, total, err := c.eventService.ListEvents(ctx, userID, limit, page, query, category, period)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"events": events,
		"total":  total,
		"page":   page,
		"limit":  limit,
	})
}

func (c *EventController) GetMyEvents(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	page, _ := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(ctx.DefaultQuery("limit", "10"), 10, 64)

	events, err := c.eventService.GetUserEvents(ctx, userID, limit, page)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, events)
}

func (c *EventController) RSVP(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	eventID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid event ID")
		return
	}

	var req models.RSVPRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := c.eventService.RSVP(ctx, eventID, userID, req.Status); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.Status(http.StatusOK)
}

func (c *EventController) GetBirthdays(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	response, err := c.eventService.GetFriendBirthdays(ctx, userID)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, response)
}

// ================================
// Invitation Endpoints
// ================================

// InviteFriends invites friends to an event
func (c *EventController) InviteFriends(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	eventID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid event ID")
		return
	}

	var req models.InviteFriendsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := c.eventService.InviteFriends(ctx, eventID, userID, req.FriendIDs, req.Message); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// GetInvitations returns pending invitations for the current user
func (c *EventController) GetInvitations(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	page, _ := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(ctx.DefaultQuery("limit", "10"), 10, 64)

	invitations, total, err := c.eventService.GetUserInvitations(ctx, userID, limit, page)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"invitations": invitations,
		"total":       total,
		"page":        page,
		"limit":       limit,
	})
}

// RespondToInvitation accepts or declines an invitation
func (c *EventController) RespondToInvitation(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	invitationID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid invitation ID")
		return
	}

	var req models.InvitationRespondRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := c.eventService.RespondToInvitation(ctx, invitationID, userID, req.Accept); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// ================================
// Discussion/Post Endpoints
// ================================

// CreatePost creates a discussion post on an event
func (c *EventController) CreatePost(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	eventID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid event ID")
		return
	}

	var req models.CreateEventPostRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	post, err := c.eventService.CreatePost(ctx, eventID, userID, req)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusCreated, post)
}

// GetPosts returns discussion posts for an event
func (c *EventController) GetPosts(ctx *gin.Context) {
	eventID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid event ID")
		return
	}

	page, _ := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(ctx.DefaultQuery("limit", "20"), 10, 64)

	posts, total, err := c.eventService.GetPosts(ctx, eventID, limit, page)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"posts": posts,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// DeletePost deletes a discussion post
func (c *EventController) DeletePost(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	eventID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid event ID")
		return
	}

	postID, err := primitive.ObjectIDFromHex(ctx.Param("postId"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid post ID")
		return
	}

	if err := c.eventService.DeletePost(ctx, eventID, postID, userID); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.Status(http.StatusOK)
}

// ReactToPost adds a reaction to a post
func (c *EventController) ReactToPost(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	postID, err := primitive.ObjectIDFromHex(ctx.Param("postId"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid post ID")
		return
	}

	var req models.ReactToPostRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := c.eventService.ReactToPost(ctx, postID, userID, req.Emoji); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// ================================
// Attendees Endpoints
// ================================

// GetAttendees returns attendees for an event
func (c *EventController) GetAttendees(ctx *gin.Context) {
	eventID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid event ID")
		return
	}

	status := models.RSVPStatus(ctx.Query("status"))
	page, _ := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(ctx.DefaultQuery("limit", "20"), 10, 64)

	response, err := c.eventService.GetAttendees(ctx, eventID, status, limit, page)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, response)
}

// ================================
// Co-Host Endpoints
// ================================

// AddCoHost adds a co-host to an event
func (c *EventController) AddCoHost(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	eventID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid event ID")
		return
	}

	var req models.AddCoHostRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	coHostID, err := primitive.ObjectIDFromHex(req.UserID)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := c.eventService.AddCoHost(ctx, eventID, userID, coHostID); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// RemoveCoHost removes a co-host from an event
func (c *EventController) RemoveCoHost(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	eventID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid event ID")
		return
	}

	coHostID, err := primitive.ObjectIDFromHex(ctx.Param("userId"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := c.eventService.RemoveCoHost(ctx, eventID, userID, coHostID); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.Status(http.StatusOK)
}

// ================================
// Categories Endpoint
// ================================

// GetCategories returns all event categories
func (c *EventController) GetCategories(ctx *gin.Context) {
	categories, err := c.eventService.GetCategories(ctx)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"categories": categories})
}

// ================================
// Search Endpoint
// ================================

// SearchEvents searches events with filters
func (c *EventController) SearchEvents(ctx *gin.Context) {
	userID, _ := utils.GetUserIDFromContext(ctx)

	var req models.SearchEventsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	events, total, err := c.eventService.SearchEvents(ctx, req, userID)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"events": events,
		"total":  total,
		"page":   req.Page,
		"limit":  req.Limit,
	})
}

// ================================
// Share Endpoint
// ================================

// ShareEvent tracks an event share
func (c *EventController) ShareEvent(ctx *gin.Context) {
	eventID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid event ID")
		return
	}

	if err := c.eventService.ShareEvent(ctx, eventID); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// ================================
// Nearby Events Endpoint
// ================================

// GetNearbyEvents returns events near a location
func (c *EventController) GetNearbyEvents(ctx *gin.Context) {
	userID, _ := utils.GetUserIDFromContext(ctx)

	lat, err := strconv.ParseFloat(ctx.Query("lat"), 64)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid latitude")
		return
	}

	lng, err := strconv.ParseFloat(ctx.Query("lng"), 64)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid longitude")
		return
	}

	radius, _ := strconv.ParseFloat(ctx.DefaultQuery("radius", "50"), 64)
	page, _ := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(ctx.DefaultQuery("limit", "20"), 10, 64)

	events, total, err := c.eventService.GetNearbyEvents(ctx, lat, lng, radius, limit, page, userID)
	if err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"events": events,
		"total":  total,
		"page":   page,
		"limit":  limit,
	})
}
