package server

import (
	"context"
	"net/http"
	"time"

	"messaging-app/config"
	"messaging-app/internal/controllers"
	"messaging-app/internal/websocket"
	"messaging-app/pkg/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type routerConfig struct {
	authController         *controllers.AuthController
	userController         *controllers.UserController
	friendshipController   *controllers.FriendshipController
	groupController        *controllers.GroupController
	messageController      *controllers.MessageController
	feedController         *controllers.FeedController
	privacyController      *controllers.PrivacyController
	searchController       *controllers.SearchController
	notificationController *controllers.NotificationController
	conversationController *controllers.ConversationController
	uploadController       *controllers.UploadController
	communityController    *controllers.CommunityController
	storyController        *controllers.StoryController
	reelController         *controllers.ReelController
	marketplaceController  *controllers.MarketplaceController
	eventController        *controllers.EventController
}

func (a *Application) buildRouters(cfg routerConfig) (*gin.Engine, *gin.Engine) {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(config.MetricsMiddleware(a.metrics))

	allowedOrigins := a.cfg.CORSAllowedOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:5173"}
	}

	corsConfig := cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	router.Use(cors.New(corsConfig))
	router.Use(middleware.RateLimiter(a.cfg))

	webSocketRouter := gin.New()
	webSocketRouter.Use(gin.Recovery())
	webSocketRouter.Use(cors.New(corsConfig))

	a.registerHealthRoutes(router)
	a.registerAuthRoutes(router, cfg.authController)
	a.registerAPIRoutes(router, cfg)
	a.registerWebSocketRoutes(webSocketRouter)

	return router, webSocketRouter
}

func (a *Application) registerHealthRoutes(router *gin.Engine) {
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		status := gin.H{"status": "ready"}
		code := http.StatusOK

		if err := a.mongoClient.Ping(ctx, nil); err != nil {
			status["mongo"] = "unavailable"
			code = http.StatusServiceUnavailable
		} else {
			status["mongo"] = "available"
		}

		if !a.redisClient.IsAvailable(ctx) {
			status["redis"] = "unavailable"
			code = http.StatusServiceUnavailable
		} else {
			status["redis"] = "available"
		}

		c.JSON(code, status)
	})
}

func (a *Application) registerAuthRoutes(router *gin.Engine, ctrl *controllers.AuthController) {
	authRoutes := router.Group("/api/auth")
	{
		authRoutes.POST("/register", ctrl.Register)
		authRoutes.POST("/login", ctrl.Login)
		authRoutes.POST("/refresh", ctrl.Refresh)
		authRoutes.POST("/logout", ctrl.Logout)
	}
}

func (a *Application) registerAPIRoutes(router *gin.Engine, cfg routerConfig) {
	authMiddleware := middleware.AuthMiddleware(a.cfg.JWTSecret, a.redisClient.GetClient())
	api := router.Group("/api", authMiddleware)

	api.POST("/upload", cfg.uploadController.Upload)

	userRoutes := api.Group("/users")
	{
		userRoutes.GET("/me", cfg.userController.GetUser)
		userRoutes.PUT("/me", cfg.userController.UpdateUser)
		userRoutes.PUT("/me/email", cfg.userController.UpdateEmail)
		userRoutes.PUT("/me/password", cfg.userController.UpdatePassword)
		userRoutes.PUT("/me/2fa", cfg.userController.ToggleTwoFactor)
		userRoutes.PUT("/me/deactivate", cfg.userController.DeactivateAccount)
		userRoutes.PUT("/me/privacy", cfg.userController.UpdatePrivacySettings)
		userRoutes.PUT("/me/notifications", cfg.userController.UpdateNotificationSettings)
		userRoutes.PUT("/me/keys", cfg.userController.UpdatePublicKey)
		userRoutes.GET("/me/groups", cfg.groupController.GetUserGroups)

		userRoutes.GET("", cfg.userController.ListUsers)
		userRoutes.GET("/presence", cfg.userController.GetUsersPresence)
		userRoutes.GET("/:id", cfg.userController.GetUserByID)
		userRoutes.GET("/:id/status", cfg.userController.GetUserStatus)
		userRoutes.GET("/:id/albums", cfg.feedController.GetUserAlbums)
	}

	feedRoutes := api.Group("")
	{
		feedRoutes.POST("/posts", cfg.feedController.CreatePost)
		feedRoutes.GET("/posts", cfg.feedController.ListPosts)
		feedRoutes.GET("/posts/:id", cfg.feedController.GetPostByID)
		feedRoutes.PUT("/posts/:id", cfg.feedController.UpdatePost)
		feedRoutes.PUT("/posts/:id/status", cfg.feedController.UpdatePostStatus)
		feedRoutes.DELETE("/posts/:id", cfg.feedController.DeletePost)
		feedRoutes.GET("/posts/:id/comments", cfg.feedController.GetCommentsByPostID)
		feedRoutes.GET("/posts/:id/reactions", cfg.feedController.GetReactionsByPostID)

		feedRoutes.GET("/hashtags/:hashtag/posts", cfg.feedController.GetPostsByHashtag)

		feedRoutes.POST("/comments", cfg.feedController.CreateComment)
		feedRoutes.PUT("/comments/:commentId", cfg.feedController.UpdateComment)
		feedRoutes.DELETE("/posts/:id/comments/:commentId", cfg.feedController.DeleteComment)
		feedRoutes.GET("/comments/:commentId/replies", cfg.feedController.GetRepliesByCommentID)
		feedRoutes.GET("/comments/:commentId/reactions", cfg.feedController.GetReactionsByCommentID)

		feedRoutes.POST("/comments/:commentId/replies", cfg.feedController.CreateReply)
		feedRoutes.PUT("/comments/:commentId/replies/:replyId", cfg.feedController.UpdateReply)
		feedRoutes.DELETE("/comments/:commentId/replies/:replyId", cfg.feedController.DeleteReply)
		feedRoutes.GET("/replies/:replyId/reactions", cfg.feedController.GetReactionsByReplyID)

		feedRoutes.POST("/reactions", cfg.feedController.CreateReaction)
		feedRoutes.DELETE("/reactions/:reactionId", cfg.feedController.DeleteReaction)
	}

	albumRoutes := api.Group("/albums")
	{
		albumRoutes.POST("", cfg.feedController.CreateAlbum)
		albumRoutes.GET("/:id", cfg.feedController.GetAlbum)
		albumRoutes.PUT("/:id", cfg.feedController.UpdateAlbum)
		albumRoutes.POST("/:id/media", cfg.feedController.AddMediaToAlbum)
		albumRoutes.GET("/:id/media", cfg.feedController.GetAlbumMedia)
	}

	privacyRoutes := api.Group("/privacy")
	{
		privacyRoutes.GET("/settings", cfg.privacyController.GetUserPrivacySettings)
		privacyRoutes.PUT("/settings", cfg.privacyController.UpdateUserPrivacySettings)

		privacyRoutes.POST("/lists", cfg.privacyController.CreateCustomPrivacyList)
		privacyRoutes.GET("/lists", cfg.privacyController.GetCustomPrivacyListsByUserID)
		privacyRoutes.GET("/lists/:id", cfg.privacyController.GetCustomPrivacyListByID)
		privacyRoutes.PUT("/lists/:id", cfg.privacyController.UpdateCustomPrivacyList)
		privacyRoutes.DELETE("/lists/:id", cfg.privacyController.DeleteCustomPrivacyList)

		privacyRoutes.POST("/lists/:id/members", cfg.privacyController.AddMemberToCustomPrivacyList)
		privacyRoutes.DELETE("/lists/:id/members/:memberId", cfg.privacyController.RemoveMemberFromCustomPrivacyList)
	}

	conversationRoutes := api.Group("/conversations")
	{
		conversationRoutes.GET("", cfg.conversationController.GetConversationSummaries)
		conversationRoutes.POST("/:id/seen", cfg.messageController.MarkConversationAsSeen)
	}

	messageRoutes := api.Group("/messages")
	{
		messageRoutes.POST("", cfg.messageController.SendMessage)
		messageRoutes.GET("", cfg.messageController.GetMessages)
		messageRoutes.GET("/search", cfg.messageController.SearchMessages)
		messageRoutes.POST("/seen", cfg.messageController.MarkMessagesAsSeen)
		messageRoutes.POST("/delivered", cfg.messageController.MarkMessagesAsDelivered)
		messageRoutes.GET("/unread", cfg.messageController.GetUnreadCount)
		messageRoutes.DELETE("/:id", cfg.messageController.DeleteMessage)
		messageRoutes.POST("/:id/react", cfg.messageController.AddReactionToMessage)
		messageRoutes.DELETE("/:id/react", cfg.messageController.RemoveReactionFromMessage)
		messageRoutes.PUT("/:id", cfg.messageController.EditMessage)
	}

	groupRoutes := api.Group("/groups")
	{
		groupRoutes.POST("", cfg.groupController.CreateGroup)
		groupRoutes.GET("/:id", cfg.groupController.GetGroup)
		groupRoutes.PUT("/:id", cfg.groupController.UpdateGroup)

		groupRoutes.POST("/:id/members", cfg.groupController.AddMember)
		groupRoutes.POST("/:id/invite", cfg.groupController.InviteMember)
		groupRoutes.DELETE("/:id/members/:userId", cfg.groupController.RemoveMember)
		groupRoutes.POST("/:id/approve", cfg.groupController.ApproveMember)
		groupRoutes.POST("/:id/reject", cfg.groupController.RejectMember)
		groupRoutes.PUT("/:id/settings", cfg.groupController.UpdateGroupSettings)
		groupRoutes.GET("/:id/activities", cfg.groupController.GetActivities)
		groupRoutes.POST("/:id/admins", cfg.groupController.AddAdmin)
		groupRoutes.DELETE("/:id/admins/:userId", cfg.groupController.RemoveAdmin)
	}

	friendshipRoutes := api.Group("/friendships")
	{
		friendshipRoutes.POST("/requests", cfg.friendshipController.SendRequest)
		friendshipRoutes.POST("/requests/:id/respond", cfg.friendshipController.RespondToRequest)
		friendshipRoutes.GET("", cfg.friendshipController.ListFriendships)
		friendshipRoutes.GET("/check", cfg.friendshipController.CheckFriendship)
		friendshipRoutes.DELETE("/:id", cfg.friendshipController.Unfriend)
		friendshipRoutes.POST("/block/:userId", cfg.friendshipController.BlockUser)
		friendshipRoutes.DELETE("/block/:userId", cfg.friendshipController.UnblockUser)
		friendshipRoutes.GET("/block/:userId/status", cfg.friendshipController.IsBlocked)
		friendshipRoutes.GET("/blocked", cfg.friendshipController.GetBlockedUsers)
		friendshipRoutes.GET("/search", cfg.friendshipController.SearchFriends)
	}

	searchRoutes := api.Group("/search")
	{
		searchRoutes.GET("", cfg.searchController.Search)
	}

	notificationRoutes := api.Group("/notifications")
	{
		notificationRoutes.GET("", cfg.notificationController.ListNotifications)
		notificationRoutes.PUT("/:id/read", cfg.notificationController.MarkNotificationAsRead)
		notificationRoutes.GET("/unread", cfg.notificationController.GetUnreadNotificationCount)
	}

	communityRoutes := api.Group("/communities")
	{
		communityRoutes.POST("", cfg.communityController.CreateCommunity)
		communityRoutes.GET("", cfg.communityController.ListCommunities)
		communityRoutes.GET("/user/me", cfg.communityController.GetUserCommunities)
		communityRoutes.GET("/user/:userId", cfg.communityController.GetUserCommunities)
		communityRoutes.GET("/:id", cfg.communityController.GetCommunity)
		communityRoutes.PUT("/:id/settings", cfg.communityController.UpdateSettings)
		communityRoutes.POST("/:id/join", cfg.communityController.JoinCommunity)
		communityRoutes.POST("/:id/leave", cfg.communityController.LeaveCommunity)
		communityRoutes.POST("/:id/approve", cfg.communityController.ApproveMember)
		communityRoutes.POST("/:id/reject", cfg.communityController.RejectMember)
		communityRoutes.GET("/:id/members", cfg.communityController.ListMembers)
		communityRoutes.GET("/:id/admins", cfg.communityController.GetAdmins)
		communityRoutes.GET("/:id/pending-members", cfg.communityController.GetPendingMembers)
	}

	storyRoutes := api.Group("/stories")
	{
		storyRoutes.POST("", cfg.storyController.CreateStory)
		storyRoutes.GET("", cfg.storyController.GetStoriesFeed)
		storyRoutes.GET("/user/:id", cfg.storyController.GetUserStories)
		storyRoutes.POST("/:id/view", cfg.storyController.ViewStory)
		storyRoutes.POST("/:id/react", cfg.storyController.ReactToStory)
		storyRoutes.GET("/:id/viewers", cfg.storyController.GetStoryViewers)
		storyRoutes.DELETE("/:id", cfg.storyController.DeleteStory)
	}

	reelRoutes := api.Group("/reels")
	{
		reelRoutes.POST("", cfg.reelController.CreateReel)
		reelRoutes.GET("", cfg.reelController.GetReelsFeed)
		reelRoutes.GET("/user/:id", cfg.reelController.GetUserReels)
		reelRoutes.GET("/:id", cfg.reelController.GetReel)

		strictLimit := middleware.StrictRateLimiter(2, 5)
		reelRoutes.POST("/:id/comments", strictLimit, cfg.reelController.AddComment)
		reelRoutes.GET("/:id/comments", cfg.reelController.GetComments)
		reelRoutes.POST("/:id/comments/:commentId/replies", strictLimit, cfg.reelController.AddReply)
		reelRoutes.POST("/:id/comments/:commentId/react", strictLimit, cfg.reelController.ReactToComment)
		reelRoutes.POST("/:id/react", strictLimit, cfg.reelController.ReactToReel)
		reelRoutes.POST("/:id/view", cfg.reelController.IncrementView)
	}

	marketplaceRoutes := api.Group("/marketplace")
	{
		marketplaceRoutes.GET("/categories", cfg.marketplaceController.GetCategories)
		marketplaceRoutes.POST("/products", cfg.marketplaceController.CreateProduct)
		marketplaceRoutes.GET("/products", cfg.marketplaceController.ListProducts)
		marketplaceRoutes.GET("/products/:id", cfg.marketplaceController.GetProduct)
		marketplaceRoutes.DELETE("/products/:id", cfg.marketplaceController.DeleteProduct)
		marketplaceRoutes.POST("/products/:id/sold", cfg.marketplaceController.MarkSold)
		marketplaceRoutes.POST("/products/:id/save", cfg.marketplaceController.ToggleSave)
		marketplaceRoutes.GET("/conversations", cfg.marketplaceController.GetConversations)
	}

	eventGroup := api.Group("/events")
	{
		eventGroup.POST("", middleware.EventRateLimiter(a.redisClient, middleware.CreateEventRateLimit), cfg.eventController.CreateEvent)
		eventGroup.GET("", cfg.eventController.ListEvents)
		eventGroup.GET("/my-events", cfg.eventController.GetMyEvents)
		eventGroup.GET("/birthdays", cfg.eventController.GetBirthdays)
		eventGroup.GET("/categories", cfg.eventController.GetCategories)
		eventGroup.GET("/recommendations", cfg.eventController.GetRecommendations)
		eventGroup.GET("/trending", cfg.eventController.GetTrending)
		eventGroup.GET("/search", middleware.EventRateLimiter(a.redisClient, middleware.SearchRateLimit), cfg.eventController.SearchEvents)
		eventGroup.GET("/nearby", cfg.eventController.GetNearbyEvents)
		eventGroup.GET("/invitations", cfg.eventController.GetInvitations)
		eventGroup.POST("/invitations/:id/respond", cfg.eventController.RespondToInvitation)
		eventGroup.GET("/:id", cfg.eventController.GetEvent)
		eventGroup.PUT("/:id", cfg.eventController.UpdateEvent)
		eventGroup.DELETE("/:id", cfg.eventController.DeleteEvent)
		eventGroup.POST("/:id/rsvp", middleware.EventRateLimiter(a.redisClient, middleware.RSVPRateLimit), cfg.eventController.RSVP)
		eventGroup.POST("/:id/share", cfg.eventController.ShareEvent)
		eventGroup.POST("/:id/invite", middleware.EventRateLimiter(a.redisClient, middleware.InviteRateLimit), cfg.eventController.InviteFriends)
		eventGroup.GET("/:id/attendees", cfg.eventController.GetAttendees)
		eventGroup.POST("/:id/co-hosts", cfg.eventController.AddCoHost)
		eventGroup.DELETE("/:id/co-hosts/:userId", cfg.eventController.RemoveCoHost)
		eventGroup.POST("/:id/posts", middleware.EventRateLimiter(a.redisClient, middleware.EventPostRateLimit), cfg.eventController.CreatePost)
		eventGroup.GET("/:id/posts", cfg.eventController.GetPosts)
		eventGroup.DELETE("/:id/posts/:postId", cfg.eventController.DeletePost)
		eventGroup.POST("/:id/posts/:postId/react", cfg.eventController.ReactToPost)
	}
}

func (a *Application) registerWebSocketRoutes(router *gin.Engine) {
	wsMiddleware := middleware.WSJwtAuthMiddleware(a.cfg.JWTSecret, a.redisClient.GetClient())
	router.GET("/ws", wsMiddleware, func(c *gin.Context) {
		config.IncWebsocketConnections(a.metrics)
		defer config.DecWebsocketConnections(a.metrics)
		websocket.ServeWs(c, a.hub)
	})
}
