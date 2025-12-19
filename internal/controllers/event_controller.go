package controllers

import (
	"net/http"
	"strconv"

	"messaging-app/internal/models"
	"messaging-app/internal/services"
	"messaging-app/pkg/utils"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type EventController struct {
	eventService *services.EventService
}

func NewEventController(eventService *services.EventService) *EventController {
	return &EventController{
		eventService: eventService,
	}
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
