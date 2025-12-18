package controllers

import (
	"net/http"
	"strconv"

	"messaging-app/internal/services"
	"messaging-app/pkg/utils"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SearchController struct {
	searchService *services.SearchService
}

func NewSearchController(searchService *services.SearchService) *SearchController {
	return &SearchController{
		searchService: searchService,
	}
}

// Search handles the search request
// @Summary Search across users and posts
// @Description Performs a full-text search across user profiles and post content.
// @Tags Search
// @Accept json
// @Produce json
// @Param query query string true "Search query string"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Number of results per page" default(10)
// @Security BearerAuth
// @Success 200 {object} services.SearchResult
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /search [get]
func (c *SearchController) Search(ctx *gin.Context) {
	query := ctx.Query("query")
	if query == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Search query cannot be empty"})
		return
	}

	page, err := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	if err != nil || page < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page number"})
		return
	}

	limit, err := strconv.ParseInt(ctx.DefaultQuery("limit", "10"), 10, 64)
	if err != nil || limit < 1 || limit > 100 { // Max limit 100
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit"})
		return
	}

	// Get current user ID from context (for friendship status)
	var currentUserID *primitive.ObjectID
	if userID, err := utils.GetUserIDFromContext(ctx); err == nil {
		currentUserID = &userID
	}

	searchResult, err := c.searchService.Search(ctx.Request.Context(), query, page, limit, currentUserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, searchResult)
}
