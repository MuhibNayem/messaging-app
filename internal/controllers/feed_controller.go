package controllers

import (
	"messaging-app/internal/models"
	"messaging-app/internal/services"
	"messaging-app/pkg/utils"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type FeedController struct {
	feedService    *services.FeedService
	userService    *services.UserService
	privacyService *services.PrivacyService
	storageService *services.StorageService
}

func NewFeedController(feedService *services.FeedService, userService *services.UserService, privacyService *services.PrivacyService, storageService *services.StorageService) *FeedController {
	return &FeedController{feedService: feedService, userService: userService, privacyService: privacyService, storageService: storageService}
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

	// Check if this is a multipart request
	contentType := ctx.GetHeader("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") {
		// Parse multipart form
		form, err := ctx.MultipartForm()
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse multipart form: " + err.Error()})
			return
		}

		// Extract content and privacy from form fields
		req.Content = ctx.PostForm("content")
		req.Location = ctx.PostForm("location")
		req.CommunityID = ctx.PostForm("community_id")
		privacyStr := ctx.PostForm("privacy")

		// Extract mentions (assuming []string of hex IDs)
		mentionIDsStr := ctx.PostFormArray("mentions[]")
		for _, idStr := range mentionIDsStr {
			if id, err := primitive.ObjectIDFromHex(idStr); err == nil {
				req.Mentions = append(req.Mentions, id)
			}
		}

		// Validate required fields
		if req.Content == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
			return
		}
		if privacyStr == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "privacy is required"})
			return
		}

		req.Privacy = models.PrivacySettingType(privacyStr) // You might want to validate this enum

		// Handle file uploads
		files := form.File["files[]"]
		if len(files) > 0 {
			mediaItems, err := c.storageService.UploadFiles(ctx.Request.Context(), files)
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload files: " + err.Error()})
				return
			}
			req.Media = mediaItems
		}
	} else {
		// Handle standard JSON request (no files)
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
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
	filterUserID := ctx.Query("user_id")
	communityID := ctx.Query("community_id")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	hasMedia := ctx.Query("has_media") == "true"
	mediaType := ctx.Query("media_type")
	status := ctx.Query("status")

	response, err := c.feedService.ListPosts(ctx.Request.Context(), objUserID, filterUserID, communityID, page, limit, sortBy, sortOrder, hasMedia, mediaType, status)
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

// UpdatePostStatus handles updating a post's status (moderation)
func (c *FeedController) UpdatePostStatus(ctx *gin.Context) {
	userID, err := utils.GetUserIDFromContext(ctx)
	if err != nil {
		utils.RespondWithError(ctx, http.StatusUnauthorized, "Authentication required")
		return
	}

	postID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, "Invalid post ID")
		return
	}

	var req struct {
		Status models.PostStatus `json:"status" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := c.feedService.UpdatePostStatus(ctx.Request.Context(), postID, userID, req.Status); err != nil {
		utils.RespondWithError(ctx, utils.GetStatusCode(err), err.Error())
		return
	}

	ctx.Status(http.StatusOK)
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
	postID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
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
	postID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
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

// ----------------------------- AlbumsEndPoints -----------------------------

// CreateAlbum godoc
// @Summary Create a new album
// @Security BearerAuth
// @Tags albums
// @Accept json
// @Produce json
// @Param body body models.CreateAlbumRequest true "Album creation data"
// @Success 201 {object} models.Album
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/albums [post]
func (c *FeedController) CreateAlbum(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req models.CreateAlbumRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	album, err := c.feedService.CreateAlbum(ctx.Request.Context(), objID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, album)
}

// UpdateAlbum godoc
// @Summary Update an album's properties (name, description, cover, privacy)
// @Security BearerAuth
// @Tags albums
// @Accept json
// @Produce json
// @Param id path string true "Album ID"
// @Param body body models.UpdateAlbumRequest true "Album update data"
// @Success 200 {object} models.Album
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 403 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/albums/{id} [put]
func (c *FeedController) UpdateAlbum(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	albumID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid album ID"})
		return
	}

	var req models.UpdateAlbumRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	album, err := c.feedService.UpdateAlbum(ctx.Request.Context(), objUserID, albumID, &req)
	if err != nil {
		if err.Error() == "cannot modify system albums" {
			ctx.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, album)
}

// GetUserAlbums godoc
// @Summary Get all albums for a user
// @Security BearerAuth
// @Tags albums
// @Produce json
// @Param userId path string true "User ID"
// @Success 200 {object} []models.Album
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/users/{id}/albums [get]
func (c *FeedController) GetUserAlbums(ctx *gin.Context) {
	userID := ctx.Param("id")
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	limit, _ := strconv.ParseInt(ctx.Query("limit"), 10, 64)
	if limit == 0 {
		limit = 20
	}
	offset, _ := strconv.ParseInt(ctx.Query("offset"), 10, 64)

	albums, err := c.feedService.GetUserAlbums(ctx.Request.Context(), objUserID, limit, offset)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, albums)
}

// GetAlbum godoc
// @Summary Get an album by ID
// @Security BearerAuth
// @Tags albums
// @Produce json
// @Param id path string true "Album ID"
// @Success 200 {object} models.Album
// @Failure 400 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/albums/{id} [get]
func (c *FeedController) GetAlbum(ctx *gin.Context) {
	albumID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid album ID"})
		return
	}

	// SECURITY: Get current user ID for authorization check
	userIDValue, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	userIDStr, ok := userIDValue.(string)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID format"})
		return
	}
	userID, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	album, err := c.feedService.GetAlbum(ctx.Request.Context(), albumID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// SECURITY: Comprehensive privacy check (IDOR protection)
	// Access rules:
	// 1. Owner can always see their own albums
	// 2. Profile and Cover albums are always public (system albums)
	// 3. PUBLIC privacy albums are visible to everyone
	// 4. FRIENDS privacy albums are visible to friends
	// 5. ONLY_ME privacy albums are only visible to owner

	isOwner := album.UserID == userID
	isSystemPublicAlbum := album.Type == models.AlbumTypeProfile || album.Type == models.AlbumTypeCover
	isPublic := album.Privacy == models.PrivacySettingPublic

	if !isOwner && !isSystemPublicAlbum && !isPublic {
		// Check if friends privacy and user is a friend
		if album.Privacy == models.PrivacySettingFriends {
			// Get album owner's info to check friends list
			owner, err := c.userService.GetUserByID(ctx.Request.Context(), album.UserID)
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify access"})
				return
			}
			// Check if requester is in owner's friends list
			isFriend := false
			for _, friendID := range owner.Friends {
				if friendID == userID {
					isFriend = true
					break
				}
			}
			if !isFriend {
				ctx.JSON(http.StatusForbidden, gin.H{"error": "this album is only visible to friends"})
				return
			}
		} else {
			// ONLY_ME or other private settings - only owner can see
			ctx.JSON(http.StatusForbidden, gin.H{"error": "you don't have permission to view this album"})
			return
		}
	}

	ctx.JSON(http.StatusOK, album)
}

// AddMediaToAlbum godoc
// @Summary Add media to an album
// @Security BearerAuth
// @Tags albums
// @Accept json
// @Produce json
// @Param id path string true "Album ID"
// @Param body body models.AddMediaToAlbumRequest true "Media to add"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} gin.H
// @Failure 401 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/albums/{id}/media [post]
func (c *FeedController) AddMediaToAlbum(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	objUserID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	albumID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid album ID"})
		return
	}

	var req models.AddMediaToAlbumRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = c.feedService.AddMediaToAlbum(ctx.Request.Context(), objUserID, albumID, req.Media)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.SuccessResponse{Success: true})
}

// GetAlbumMedia godoc
// @Summary Get media items for an album with pagination
// @Security BearerAuth
// @Tags albums
// @Produce json
// @Param id path string true "Album ID"
// @Param limit query int false "Limit"
// @Param offset query int false "Offset"
// @Success 200 {object} []models.AlbumMedia
// @Failure 400 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /api/albums/{id}/media [get]
func (c *FeedController) GetAlbumMedia(ctx *gin.Context) {
	albumID, err := primitive.ObjectIDFromHex(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid album ID"})
		return
	}

	limit, _ := strconv.ParseInt(ctx.Query("limit"), 10, 64)
	if limit == 0 {
		limit = 20
	}
	page, _ := strconv.ParseInt(ctx.Query("page"), 10, 64)
	if page < 1 {
		page = 1
	}
	mediaType := ctx.Query("type")

	// Calculate offset from page
	offset := (page - 1) * limit

	media, total, err := c.feedService.GetAlbumMedia(ctx.Request.Context(), albumID, limit, offset, mediaType)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"media": media,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}
