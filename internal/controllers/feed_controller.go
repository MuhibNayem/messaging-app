package controllers

import (
	"messaging-app/internal/models"
	"messaging-app/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type FeedController struct {
	feedService    *services.FeedService
	userService    *services.UserService
	privacyService *services.PrivacyService
}

func NewFeedController(feedService *services.FeedService, userService *services.UserService, privacyService *services.PrivacyService) *FeedController {
	return &FeedController{feedService: feedService, userService: userService, privacyService: privacyService}
}

// CreatePost godoc
// @Summary Create a new post
// @Security BearerAuth
// @Tags feed
// @Accept json
// @Produce json
// @Param body body models.CreatePostRequest true "Post creation data"
// @Success 201 {object} models.Post
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/posts [post]
func (c *FeedController) CreatePost(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req models.CreatePostRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	post, err := c.feedService.CreatePost(ctx.Request.Context(), objID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, post)
}

// GetPostByID godoc
// @Summary Get a post by ID
// @Security BearerAuth
// @Tags feed
// @Produce json
// @Param id path string true "Post ID"
// @Success 200 {object} models.Post
// @Failure 400 {object} gin.H
// @Failure 404 {object} gin.H
// @Router /api/posts/{id} [get]
func (c *FeedController) GetPostByID(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	postID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid post ID"})
		return
	}

	post, err := c.feedService.GetPostByID(ctx.Request.Context(), objUserID, postID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
		return
	}

	ctx.JSON(http.StatusOK, post)
}

// UpdatePost godoc
// @Summary Update an existing post
// @Security BearerAuth
// @Tags feed
// @Accept json
// @Produce json
// @Param id path string true "Post ID"
// @Param body body models.UpdatePostRequest true "Post update data"
// @Success 200 {object} models.Post
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/posts/{id} [put]
func (c *FeedController) UpdatePost(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	postID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid post ID"})
		return
	}

	var req models.UpdatePostRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updatedPost, err := c.feedService.UpdatePost(ctx.Request.Context(), objUserID, postID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, updatedPost)
}

// DeletePost godoc
// @Summary Delete a post
// @Security BearerAuth
// @Tags feed
// @Produce json
// @Param id path string true "Post ID"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/posts/{id} [delete]
func (c *FeedController) DeletePost(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	postID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid post ID"})
		return
	}

	err = c.feedService.DeletePost(ctx.Request.Context(), objUserID, postID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}

// ListPosts godoc
// @Summary List posts (paginated)
// @Security BearerAuth
// @Tags feed
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Param sortBy query string false "Sort by field (e.g., created_at, reaction_count, comment_count)" default(created_at)
// @Param sortOrder query string false "Sort order (asc, desc)" default(desc)
// @Success 200 {object} models.FeedResponse
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/posts [get]
func (c *FeedController) ListPosts(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	page, _ := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(ctx.DefaultQuery("limit", "20"), 10, 64)
	sortBy := ctx.DefaultQuery("sortBy", "created_at")
	sortOrder := ctx.DefaultQuery("sortOrder", "desc")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	response, err := c.feedService.ListPosts(ctx.Request.Context(), objUserID, page, limit, sortBy, sortOrder)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, response)
}

// GetPostsByHashtag godoc
// @Summary Get posts by hashtag (paginated)
// @Security BearerAuth
// @Tags feed
// @Produce json
// @Param hashtag path string true "Hashtag to search for"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Success 200 {object} models.FeedResponse
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/hashtags/{hashtag}/posts [get]
func (c *FeedController) GetPostsByHashtag(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	hashtag := ctx.Param("hashtag")
	if hashtag == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "hashtag cannot be empty"})
		return
	}

	page, _ := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(ctx.DefaultQuery("limit", "20"), 10, 64)

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	response, err := c.feedService.GetPostsByHashtag(ctx.Request.Context(), objUserID, hashtag, page, limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, response)
}

// GetCommentsByPostID godoc
// @Summary Get comments for a post
// @Security BearerAuth
// @Tags feed
// @Produce json
// @Param postId path string true "Post ID" d
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Success 200 {object} []models.Comment
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/posts/{postId}/comments [get]
func (c *FeedController) GetCommentsByPostID(ctx *gin.Context) {
	postID, err := primitive.ObjectIDFromHex(ctx.Param("postId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid post ID"})
		return
	}

	page, _ := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(ctx.DefaultQuery("limit", "20"), 10, 64)

	comments, err := c.feedService.GetCommentsByPostID(ctx.Request.Context(), postID, page, limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, comments)
}

// GetRepliesByCommentID godoc
// @Summary Get replies for a comment
// @Security BearerAuth
// @Tags feed
// @Produce json
// @Param commentId path string true "Comment ID"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Success 200 {object} []models.Reply
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/comments/{commentId}/replies [get]
func (c *FeedController) GetRepliesByCommentID(ctx *gin.Context) {
	commentID, err := primitive.ObjectIDFromHex(ctx.Param("commentId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment ID"})
		return
	}

	page, _ := strconv.ParseInt(ctx.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(ctx.DefaultQuery("limit", "20"), 10, 64)

	replies, err := c.feedService.GetRepliesByCommentID(ctx.Request.Context(), commentID, page, limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, replies)
}

// GetReactionsByPostID godoc
// @Summary Get reactions for a post
// @Security BearerAuth
// @Tags feed
// @Produce json
// @Param postId path string true "Post ID"
// @Success 200 {object} []models.Reaction
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/posts/{postId}/reactions [get]
func (c *FeedController) GetReactionsByPostID(ctx *gin.Context) {
	postID, err := primitive.ObjectIDFromHex(ctx.Param("postId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid post ID"})
		return
	}

	reactions, err := c.feedService.GetReactionsByTargetID(ctx.Request.Context(), postID, "post")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, reactions)
}

// GetReactionsByCommentID godoc
// @Summary Get reactions for a comment
// @Security BearerAuth
// @Tags feed
// @Produce json
// @Param commentId path string true "Comment ID"
// @Success 200 {object} []models.Reaction
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/comments/{commentId}/reactions [get]
func (c *FeedController) GetReactionsByCommentID(ctx *gin.Context) {
	commentID, err := primitive.ObjectIDFromHex(ctx.Param("commentId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment ID"})
		return
	}

	reactions, err := c.feedService.GetReactionsByTargetID(ctx.Request.Context(), commentID, "comment")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, reactions)
}

// GetReactionsByReplyID godoc
// @Summary Get reactions for a reply
// @Security BearerAuth
// @Tags feed
// @Produce json
// @Param replyId path string true "Reply ID"
// @Success 200 {object} []models.Reaction
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/replies/{replyId}/reactions [get]
func (c *FeedController) GetReactionsByReplyID(ctx *gin.Context) {
	replyID, err := primitive.ObjectIDFromHex(ctx.Param("replyId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid reply ID"})
		return
	}

	reactions, err := c.feedService.GetReactionsByTargetID(ctx.Request.Context(), replyID, "reply")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, reactions)
}

// CreateComment godoc
// @Summary Create a new comment on a post
// @Security BearerAuth
// @Tags feed
// @Accept json
// @Produce json
// @Param body body models.CreateCommentRequest true "Comment creation data"
// @Success 201 {object} models.Comment
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/comments [post]
func (c *FeedController) CreateComment(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req models.CreateCommentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	comment, err := c.feedService.CreateComment(ctx.Request.Context(), objID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, comment)
}

// UpdateComment godoc
// @Summary Update an existing comment
// @Security BearerAuth
// @Tags feed
// @Accept json
// @Produce json
// @Param id path string true "Comment ID"
// @Param body body models.UpdateCommentRequest true "Comment update data"
// @Success 200 {object} models.Comment
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/comments/{id} [put]
func (c *FeedController) UpdateComment(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	commentID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment ID"})
		return
	}

	var req models.UpdateCommentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updatedComment, err := c.feedService.UpdateComment(ctx.Request.Context(), objUserID, commentID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return	
	}

	ctx.JSON(http.StatusOK, updatedComment)
}

// DeleteComment godoc
// @Summary Delete a comment
// @Security BearerAuth
// @Tags feed
// @Produce json
// @Param postId query string true "Post ID"
// @Param commentId path string true "Comment ID"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/posts/{postId}/comments/{commentId} [delete]
func (c *FeedController) DeleteComment(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	postID, err := primitive.ObjectIDFromHex(ctx.Param("postId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid post ID"})
		return
	}

	commentID, err := primitive.ObjectIDFromHex(ctx.Param("commentId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment ID"})
		return
	}

	err = c.feedService.DeleteComment(ctx.Request.Context(), objUserID, postID, commentID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}

// CreateReply godoc
// @Summary Create a new reply to a comment
// @Security BearerAuth
// @Tags feed
// @Accept json
// @Produce json
// @Param body body models.CreateReplyRequest true "Reply creation data"
// @Success 201 {object} models.Reply
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/comments/{commentId}/replies [post]
func (c *FeedController) CreateReply(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	commentID, err := primitive.ObjectIDFromHex(ctx.Param("commentId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment ID"})
		return
	}

	var req models.CreateReplyRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.CommentID = commentID
	reply, err := c.feedService.CreateReply(ctx.Request.Context(), objID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, reply)
}

// UpdateReply godoc
// @Summary Update an existing reply
// @Security BearerAuth
// @Tags feed
// @Accept json
// @Produce json
// @Param commentId path string true "Comment ID"
// @Param replyId path string true "Reply ID"
// @Param body body models.UpdateReplyRequest true "Reply update data"
// @Success 200 {object} models.Reply
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/comments/{commentId}/replies/{replyId} [put]
func (c *FeedController) UpdateReply(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	commentID, err := primitive.ObjectIDFromHex(ctx.Param("commentId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment ID"})
		return
	}

	replyID, err := primitive.ObjectIDFromHex(ctx.Param("replyId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid reply ID"})
		return
	}

	var req models.UpdateReplyRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.CommentID = commentID
	updatedReply, err := c.feedService.UpdateReply(ctx.Request.Context(), objUserID, replyID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, updatedReply)
}

// DeleteReply godoc
// @Summary Delete a reply
// @Security BearerAuth
// @Tags feed
// @Produce json
// @Param commentId path string true "Comment ID"
// @Param replyId path string true "Reply ID"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/comments/{commentId}/replies/{replyId} [delete]
func (c *FeedController) DeleteReply(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	commentID, err := primitive.ObjectIDFromHex(ctx.Param("commentId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment ID"})
		return
	}

	replyID, err := primitive.ObjectIDFromHex(ctx.Param("replyId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid reply ID"})
		return
	}

	err = c.feedService.DeleteReply(ctx.Request.Context(), objUserID, commentID, replyID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}

// CreateReaction godoc
// @Summary Create a new reaction on a post, comment, or reply
// @Security BearerAuth
// @Tags feed
// @Accept json
// @Produce json
// @Param body body models.CreateReactionRequest true "Reaction creation data"
// @Success 201 {object} models.Reaction
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/reactions [post]
func (c *FeedController) CreateReaction(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req models.CreateReactionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	reaction, err := c.feedService.CreateReaction(ctx.Request.Context(), objID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, reaction)
}

// DeleteReaction godoc
// @Summary Delete a reaction
// @Security BearerAuth
// @Tags feed
// @Produce json
// @Param reactionId path string true "Reaction ID"
// @Param targetId query string true "Target ID (post, comment, or reply)"
// @Param targetType query string true "Target Type (post, comment, or reply)"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/reactions/{reactionId} [delete]
func (c *FeedController) DeleteReaction(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	reactionID, err := primitive.ObjectIDFromHex(ctx.Param("reactionId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid reaction ID"})
		return
	}

	targetID, err := primitive.ObjectIDFromHex(ctx.Query("targetId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid target ID"})
		return
	}
	targetType := ctx.Query("targetType")

	err = c.feedService.DeleteReaction(ctx.Request.Context(), objUserID, reactionID, targetID, targetType)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}