package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"messaging-app/config"
	"messaging-app/internal/controllers"
	cassdb "messaging-app/internal/db"
	"messaging-app/internal/graph"
	"messaging-app/internal/kafka"
	notifications "messaging-app/internal/notifications"
	"messaging-app/internal/redis"
	"messaging-app/internal/repositories"
	"messaging-app/internal/seeds" // Added seeds import
	"messaging-app/internal/services"
	"messaging-app/internal/websocket"
	"messaging-app/pkg/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func createIndexes(ctx context.Context, db *mongo.Database) error {
	log.Println("Creating MongoDB text indexes...")

	// User collection text index
	userIndexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "username", Value: "text"},
			{Key: "email", Value: "text"},
			{Key: "full_name", Value: "text"},
			{Key: "bio", Value: "text"},
			{Key: "location", Value: "text"},
		},
		Options: options.Index().SetName("user_text_index").SetWeights(bson.D{
			{Key: "username", Value: 10},
			{Key: "email", Value: 8},
			{Key: "full_name", Value: 5},
			{Key: "bio", Value: 3},
			{Key: "location", Value: 1},
		}),
	}

	// Post collection text index
	postIndexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "content", Value: "text"},
			{Key: "hashtags", Value: "text"},
			{Key: "community_id", Value: 1}, // Index for efficient filtering by community
		},
		Options: options.Index().SetName("post_text_index_v2").SetWeights(bson.D{
			{Key: "content", Value: 10},
			{Key: "hashtags", Value: 5},
		}),
	}

	// Message collection text index
	messageIndexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "content", Value: "text"}},
		Options: options.Index().SetName("message_text_index"),
	}

	// Create indexes for users collection
	_, err := db.Collection("users").Indexes().CreateOne(ctx, userIndexModel)
	if err != nil {
		return fmt.Errorf("failed to create user text index: %w", err)
	}
	log.Println("User text index created successfully.")

	_, _ = db.Collection("posts").Indexes().DropOne(ctx, "post_text_index")

	_, err = db.Collection("posts").Indexes().CreateOne(ctx, postIndexModel)
	if err != nil {
		return fmt.Errorf("failed to create post text index: %w", err)
	}
	log.Println("Post text index created successfully.")

	// Create indexes for messages collection
	_, err = db.Collection("messages").Indexes().CreateOne(ctx, messageIndexModel)
	if err != nil {
		return fmt.Errorf("failed to create message text index: %w", err)
	}
	log.Println("Message text index created successfully.")

	return nil
}

func main() {
	// Load configuration
	cfg := config.LoadConfig()
	metrics := config.GetMetrics()

	// Initialize MongoDB
	clientOptions := options.Client().
		ApplyURI(cfg.MongoURI).
		SetAuth(options.Credential{
			Username: cfg.MongoUser,
			Password: cfg.MongoPassword,
		}).
		SetMaxPoolSize(100).
		SetSocketTimeout(10 * time.Second)

	mongoClient, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer func() {
		if err := mongoClient.Disconnect(context.Background()); err != nil {
			log.Printf("Error disconnecting MongoDB: %v", err)
		}
	}()

	db := mongoClient.Database(cfg.DBName)

	// Create MongoDB indexes
	if err := createIndexes(context.Background(), db); err != nil {
		log.Fatalf("Failed to create MongoDB indexes: %v", err)
	}

	// Initialize Redis Cluster
	redisClient := redis.NewClusterClient(cfg)
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Printf("Error closing Redis connection: %v", err)
		}
	}()

	// Verify Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if !redisClient.IsAvailable(ctx) {
		log.Fatal("Failed to connect to Redis cluster")
	}

	// Initialize Neo4j
	neo4jClient, err := graph.NewNeo4jClient(cfg.Neo4jURI, cfg.Neo4jUser, cfg.Neo4jPassword)
	if err != nil {
		log.Printf("Warning: Failed to connect to Neo4j: %v", err)
	} else {
		defer neo4jClient.Close(context.Background())
	}

	// Initialize Cassandra
	cassandraClient, err := cassdb.NewCassandraClient(cfg.CassandraHosts, cfg.CassandraKeyspace, cfg.CassandraUser, cfg.CassandraPassword)
	if err != nil {
		log.Printf("Warning: Failed to connect to Cassandra: %v", err)
	} else {
		defer cassandraClient.Close()
	}

	// Initialize Repositories
	userRepo := repositories.NewUserRepository(db)
	messageRepo := repositories.NewMessageRepository(db)
	groupRepo := repositories.NewGroupRepository(db)
	friendshipRepo := repositories.NewFriendshipRepository(db)
	feedRepo := repositories.NewFeedRepository(db)
	privacyRepo := repositories.NewPrivacyRepository(db)
	notificationRepo := repositories.NewNotificationRepository(db)
	conversationRepo := repositories.NewConversationRepository(db, userRepo, groupRepo)

	communityRepo := repositories.NewCommunityRepository(db)
	storyRepo := repositories.NewStoryRepository(db)
	reelRepo := repositories.NewReelRepository(db)
	marketplaceRepo := repositories.NewMarketplaceRepository(db)
	eventRepo := repositories.NewEventRepository(db)

	// Graph Repositories (Only if Neo4j is connected)
	var userGraphRepo *repositories.UserGraphRepository
	var eventGraphRepo *repositories.EventGraphRepository
	if neo4jClient != nil {
		userGraphRepo = repositories.NewUserGraphRepository(neo4jClient.Driver)
		eventGraphRepo = repositories.NewEventGraphRepository(neo4jClient.Driver)
		log.Printf("Graph Repositories Initialized: UserGraph=%v, EventGraph=%v", userGraphRepo != nil, eventGraphRepo != nil)
	}

	// Seeding
	marketplaceSeeder := seeds.NewMarketplaceSeeder(marketplaceRepo)
	if err := marketplaceSeeder.SeedCategories(context.Background()); err != nil {
		log.Printf("Warning: Failed to seed categories: %v", err)
	}

	// Initialize Kafka Producer
	kafkaProducer := kafka.NewMessageProducer(cfg.KafkaBrokers, cfg.KafkaTopic)
	defer func() {
		if err := kafkaProducer.Close(); err != nil {
			log.Printf("Error closing Kafka producer: %v", err)
		}
	}()

	// Initialize Services
	authService := services.NewAuthService(userRepo, cfg.JWTSecret, redisClient.GetClient(), cfg)
	notificationService := notifications.NewNotificationService(notificationRepo, userRepo, kafkaProducer)

	storageService, err := services.NewStorageService(cfg)
	if err != nil {
		log.Fatal("Failed to initialize storage service:", err)
	}

	// Initialize Message Archive Service for tiered storage
	var messageArchiveService *services.MessageArchiveService
	if cassandraClient != nil {
		messageArchiveService = services.NewMessageArchiveService(
			cassandraClient,
			storageService,
			redisClient,
			cfg,
		)
		// Start background archive worker (runs daily)
		go messageArchiveService.StartArchiveWorker(context.Background())
		log.Println("Message archive worker started")
	}

	feedService := services.NewFeedService(feedRepo, userRepo, friendshipRepo, communityRepo, privacyRepo, kafkaProducer, notificationService, storageService)
	// Note: UserService now needs FeedService for profile history
	userService := services.NewUserService(userRepo, reelRepo, redisClient.GetClient(), feedService)
	// Inject Graph Repo into User Service if needed later

	messageCassandraRepo := repositories.NewMessageCassandraRepository(cassandraClient)
	groupActivityRepo := repositories.NewGroupActivityRepository(cassandraClient)

	// Wire archive fetcher for tiered storage merge logic
	if messageArchiveService != nil {
		messageCassandraRepo.SetArchiveFetcher(messageArchiveService)
	}

	groupService := services.NewGroupService(groupRepo, userRepo, groupActivityRepo, cassandraClient, kafkaProducer)
	friendshipService := services.NewFriendshipService(friendshipRepo, userRepo, userGraphRepo)
	messageService := services.NewMessageService(messageRepo, groupRepo, friendshipRepo, kafkaProducer, redisClient.GetClient(), userRepo, notificationService, messageCassandraRepo)
	privacyService := services.NewPrivacyService(privacyRepo, userRepo)

	searchService := services.NewSearchService(userRepo, feedRepo, friendshipRepo)
	conversationService := services.NewConversationService(conversationRepo, messageCassandraRepo, userRepo, groupRepo)

	communityService := services.NewCommunityService(communityRepo, userRepo)
	storyService := services.NewStoryService(storyRepo, userRepo, friendshipRepo)
	reelService := services.NewReelService(reelRepo, userRepo, friendshipRepo)
	marketplaceService := services.NewMarketplaceService(marketplaceRepo, userRepo, messageCassandraRepo)

	// Inject Graph Repo into Event Service?
	// For now, let's keep EventService signature same until we refactor it.
	eventService := services.NewEventService(eventRepo, userRepo, eventGraphRepo)

	// Initialize Controllers
	authController := controllers.NewAuthController(authService, cfg)
	userController := controllers.NewUserController(userService)
	friendshipController := controllers.NewFriendshipController(friendshipService)
	groupController := controllers.NewGroupController(groupService, userService)
	messageController := controllers.NewMessageController(messageService, storageService)
	feedController := controllers.NewFeedController(feedService, userService, privacyService, storageService)
	privacyController := controllers.NewPrivacyController(privacyService, userService)
	searchController := controllers.NewSearchController(searchService)
	notificationController := controllers.NewNotificationController(notificationService)
	conversationController := controllers.NewConversationController(conversationService)
	uploadController := controllers.NewUploadController(storageService)

	communityController := controllers.NewCommunityController(communityService)
	storyController := controllers.NewStoryController(storyService)
	reelController := controllers.NewReelController(reelService)
	marketplaceController := controllers.NewMarketplaceController(marketplaceService)
	eventController := controllers.NewEventController(eventService)

	// Initialize WebSocket Hub
	hub := websocket.NewHub(redisClient, groupRepo, feedRepo, userRepo, friendshipRepo, messageRepo, messageCassandraRepo, messageService)

	// Initialize Kafka Consumers
	kafkaConsumer := kafka.NewMessageConsumer(cfg.KafkaBrokers, cfg.KafkaTopic, "message-group", hub)
	go func() {
		kafkaConsumer.ConsumeMessages(context.Background())
	}()

	notificationConsumer := kafka.NewNotificationConsumer(cfg.KafkaBrokers, "notifications_events", "notification-group", hub)
	go func() {
		notificationConsumer.Start(context.Background())
	}()
	defer func() {
		if err := notificationConsumer.Close(); err != nil {
			log.Printf("Error closing Notification Kafka consumer: %v", err)
		}
	}()

	// Initialize and Start Cleanup Service for Expired Stories
	cleanupService := services.NewCleanupService(storyRepo, storageService)
	go cleanupService.StartCleanupWorker(context.Background())

	// Initialize Gin Router with metrics middleware
	router := gin.Default()
	router.Use(config.MetricsMiddleware(metrics))

	allowedOrigins := cfg.CORSAllowedOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:5173"}
	}

	// Custom CORS configuration with explicit origins
	corsConfig := cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	router.Use(cors.New(corsConfig))
	router.Use(middleware.RateLimiter(cfg))

	// WebSocket router (without metrics middleware) - apply same CORS config
	webSocketRouter := gin.Default()
	webSocketRouter.Use(cors.New(corsConfig))

	// Start metrics server on separate port
	go func() {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", config.MetricsHandler())

		metricsServer := &http.Server{
			Addr:    ":" + cfg.PrometheusPort,
			Handler: metricsMux,
		}

		log.Printf("Metrics server starting on port %s", cfg.PrometheusPort)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Metrics server failed: %v", err)
		}
	}()

	// Health check endpoints
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		status := gin.H{"status": "ready"}
		code := http.StatusOK

		if err := mongoClient.Ping(ctx, nil); err != nil {
			status["mongo"] = "unavailable"
			code = http.StatusServiceUnavailable
		} else {
			status["mongo"] = "available"
		}

		if !redisClient.IsAvailable(ctx) {
			status["redis"] = "unavailable"
			code = http.StatusServiceUnavailable
		} else {
			status["redis"] = "available"
		}

		c.JSON(code, status)
	})

	// Auth routes (public routes)
	authRoutes := router.Group("/api/auth")
	{
		authRoutes.POST("/register", authController.Register)
		authRoutes.POST("/login", authController.Login)
		authRoutes.POST("/refresh", authController.Refresh)

	}

	// Protected routes
	authMiddleware := middleware.AuthMiddleware(cfg.JWTSecret, redisClient.GetClient())
	wsMiddleware := middleware.WSJwtAuthMiddleware(cfg.JWTSecret, redisClient.GetClient())
	api := router.Group("/api", authMiddleware)

	// Upload Route
	api.POST("/upload", uploadController.Upload)

	api.POST("/auth/logout", authController.Logout)

	// User Routes
	userRoutes := api.Group("/users")
	{
		userRoutes.GET("/me", userController.GetUser)                                  // Get current user
		userRoutes.PUT("/me", userController.UpdateUser)                               // Update current user
		userRoutes.PUT("/me/email", userController.UpdateEmail)                        // Update current user email
		userRoutes.PUT("/me/password", userController.UpdatePassword)                  // Update current user password
		userRoutes.PUT("/me/2fa", userController.ToggleTwoFactor)                      // Toggle 2FA for current user
		userRoutes.PUT("/me/deactivate", userController.DeactivateAccount)             // Deactivate current user account
		userRoutes.PUT("/me/privacy", userController.UpdatePrivacySettings)            // Update current user privacy
		userRoutes.PUT("/me/notifications", userController.UpdateNotificationSettings) // Update notification settings
		userRoutes.PUT("/me/keys", userController.UpdatePublicKey)                     // Update E2EE public key
		userRoutes.GET("/me/groups", groupController.GetUserGroups)                    // Get current user's groups

		userRoutes.GET("", userController.ListUsers)                 // List all users
		userRoutes.GET("/presence", userController.GetUsersPresence) // Get presence status for multiple users
		userRoutes.GET("/:id", userController.GetUserByID)           // Get specific user by ID
		userRoutes.GET("/:id/status", userController.GetUserStatus)  // Get user status
		userRoutes.GET("/:id/albums", feedController.GetUserAlbums)
	}

	// Feed Routes
	feedRoutes := api.Group("")
	{
		// Post routes
		feedRoutes.POST("/posts", feedController.CreatePost)
		feedRoutes.GET("/posts", feedController.ListPosts)
		feedRoutes.GET("/posts/:id", feedController.GetPostByID)
		feedRoutes.PUT("/posts/:id", feedController.UpdatePost)
		feedRoutes.PUT("/posts/:id/status", feedController.UpdatePostStatus) // Support moderation
		feedRoutes.DELETE("/posts/:id", feedController.DeletePost)
		feedRoutes.GET("/posts/:id/comments", feedController.GetCommentsByPostID)
		feedRoutes.GET("/posts/:id/reactions", feedController.GetReactionsByPostID)

		// Hashtag routes
		feedRoutes.GET("/hashtags/:hashtag/posts", feedController.GetPostsByHashtag)

		// Comment routes
		feedRoutes.POST("/comments", feedController.CreateComment)
		feedRoutes.PUT("/comments/:commentId", feedController.UpdateComment)
		feedRoutes.DELETE("/posts/:id/comments/:commentId", feedController.DeleteComment)
		feedRoutes.GET("/comments/:commentId/replies", feedController.GetRepliesByCommentID)
		feedRoutes.GET("/comments/:commentId/reactions", feedController.GetReactionsByCommentID)

		// Reply routes
		feedRoutes.POST("/comments/:commentId/replies", feedController.CreateReply)
		feedRoutes.PUT("/comments/:commentId/replies/:replyId", feedController.UpdateReply)
		feedRoutes.DELETE("/comments/:commentId/replies/:replyId", feedController.DeleteReply)
		feedRoutes.GET("/replies/:replyId/reactions", feedController.GetReactionsByReplyID)

		// Reaction routes
		feedRoutes.POST("/reactions", feedController.CreateReaction)
		feedRoutes.DELETE("/reactions/:reactionId", feedController.DeleteReaction)
	}

	// Album Routes
	albumRoutes := api.Group("/albums")
	{
		albumRoutes.POST("", feedController.CreateAlbum)
		albumRoutes.GET("/:id", feedController.GetAlbum)
		albumRoutes.PUT("/:id", feedController.UpdateAlbum)
		albumRoutes.POST("/:id/media", feedController.AddMediaToAlbum)
		albumRoutes.GET("/:id/media", feedController.GetAlbumMedia)
	}

	// Privacy Routes
	privacyRoutes := api.Group("/privacy")
	{
		privacyRoutes.GET("/settings", privacyController.GetUserPrivacySettings)
		privacyRoutes.PUT("/settings", privacyController.UpdateUserPrivacySettings)

		// Custom privacy lists
		privacyRoutes.POST("/lists", privacyController.CreateCustomPrivacyList)
		privacyRoutes.GET("/lists", privacyController.GetCustomPrivacyListsByUserID)
		privacyRoutes.GET("/lists/:id", privacyController.GetCustomPrivacyListByID)
		privacyRoutes.PUT("/lists/:id", privacyController.UpdateCustomPrivacyList)
		privacyRoutes.DELETE("/lists/:id", privacyController.DeleteCustomPrivacyList)

		// Privacy list members
		privacyRoutes.POST("/lists/:id/members", privacyController.AddMemberToCustomPrivacyList)
		privacyRoutes.DELETE("/lists/:id/members/:memberId", privacyController.RemoveMemberFromCustomPrivacyList)
	}

	// Conversation Routes
	conversationRoutes := api.Group("/conversations")
	{
		conversationRoutes.GET("", conversationController.GetConversationSummaries)
		conversationRoutes.POST("/:id/seen", messageController.MarkConversationAsSeen)
	}
	messageRoutes := api.Group("/messages")
	{
		messageRoutes.POST("", messageController.SendMessage)
		messageRoutes.GET("", messageController.GetMessages)
		messageRoutes.GET("/search", messageController.SearchMessages)
		messageRoutes.POST("/seen", messageController.MarkMessagesAsSeen)
		messageRoutes.POST("/delivered", messageController.MarkMessagesAsDelivered)
		messageRoutes.GET("/unread", messageController.GetUnreadCount)
		messageRoutes.DELETE("/:id", messageController.DeleteMessage)
		messageRoutes.POST("/:id/react", messageController.AddReactionToMessage)
		messageRoutes.DELETE("/:id/react", messageController.RemoveReactionFromMessage)
		messageRoutes.PUT("/:id", messageController.EditMessage)
	}

	// Group Routes
	groupRoutes := api.Group("/groups")
	{
		groupRoutes.POST("", groupController.CreateGroup)
		groupRoutes.GET("/:id", groupController.GetGroup)
		groupRoutes.PUT("/:id", groupController.UpdateGroup)

		// Group members
		groupRoutes.POST("/:id/members", groupController.AddMember)
		groupRoutes.POST("/:id/invite", groupController.InviteMember)
		groupRoutes.DELETE("/:id/members/:userId", groupController.RemoveMember)
		groupRoutes.POST("/:id/approve", groupController.ApproveMember)
		groupRoutes.POST("/:id/reject", groupController.RejectMember)

		// Group settings
		groupRoutes.PUT("/:id/settings", groupController.UpdateGroupSettings)

		// Group activities
		groupRoutes.GET("/:id/activities", groupController.GetActivities)

		// Group admins
		groupRoutes.POST("/:id/admins", groupController.AddAdmin)
		groupRoutes.DELETE("/:id/admins/:userId", groupController.RemoveAdmin)
	}

	// Friendship Routes
	friendshipRoutes := api.Group("/friendships")
	{
		// Friend requests
		friendshipRoutes.POST("/requests", friendshipController.SendRequest)
		friendshipRoutes.POST("/requests/:id/respond", friendshipController.RespondToRequest)

		// Friendships management
		friendshipRoutes.GET("", friendshipController.ListFriendships)
		friendshipRoutes.GET("/check", friendshipController.CheckFriendship)
		friendshipRoutes.DELETE("/:id", friendshipController.Unfriend)

		// Blocking functionality
		friendshipRoutes.POST("/block/:userId", friendshipController.BlockUser)
		friendshipRoutes.DELETE("/block/:userId", friendshipController.UnblockUser)
		friendshipRoutes.GET("/block/:userId/status", friendshipController.IsBlocked)

		friendshipRoutes.GET("/blocked", friendshipController.GetBlockedUsers)

		// Search friends
		friendshipRoutes.GET("/search", friendshipController.SearchFriends)
	}

	// Search Routes
	searchRoutes := api.Group("/search")
	{
		searchRoutes.GET("", searchController.Search)
	}

	// Notification Routes
	notificationRoutes := api.Group("/notifications")
	{
		notificationRoutes.GET("", notificationController.ListNotifications)
		notificationRoutes.PUT("/:id/read", notificationController.MarkNotificationAsRead)
		notificationRoutes.GET("/unread", notificationController.GetUnreadNotificationCount)
	}

	// Community Routes
	communityRoutes := api.Group("/communities")
	{
		communityRoutes.POST("", communityController.CreateCommunity)
		communityRoutes.GET("", communityController.ListCommunities)
		communityRoutes.GET("/user/me", communityController.GetUserCommunities) // Get current user's communities
		communityRoutes.GET("/user/:userId", communityController.GetUserCommunities)
		communityRoutes.GET("/:id", communityController.GetCommunity)
		communityRoutes.PUT("/:id/settings", communityController.UpdateSettings)
		communityRoutes.POST("/:id/join", communityController.JoinCommunity)
		communityRoutes.POST("/:id/leave", communityController.LeaveCommunity)
		communityRoutes.POST("/:id/approve", communityController.ApproveMember)
		communityRoutes.POST("/:id/reject", communityController.RejectMember)
		communityRoutes.GET("/:id/members", communityController.ListMembers)
		communityRoutes.GET("/:id/admins", communityController.GetAdmins)
		communityRoutes.GET("/:id/pending-members", communityController.GetPendingMembers)
	}

	// Story Routes
	storyRoutes := api.Group("/stories")
	{
		storyRoutes.POST("", storyController.CreateStory)
		storyRoutes.GET("", storyController.GetStoriesFeed)
		storyRoutes.GET("/user/:id", storyController.GetUserStories)
		storyRoutes.POST("/:id/view", storyController.ViewStory)
		storyRoutes.POST("/:id/react", storyController.ReactToStory)
		storyRoutes.GET("/:id/viewers", storyController.GetStoryViewers)
		storyRoutes.DELETE("/:id", storyController.DeleteStory)
	}

	// Reel Routes
	reelRoutes := api.Group("/reels")
	{
		reelRoutes.POST("", reelController.CreateReel)
		reelRoutes.GET("", reelController.GetReelsFeed)
		reelRoutes.GET("/user/:id", reelController.GetUserReels)
		reelRoutes.GET("/:id", reelController.GetReel)

		// Apply strict rate limiting to interaction endpoints
		strictLimit := middleware.StrictRateLimiter(2, 5) // 2 req/s, burst 5

		reelRoutes.POST("/:id/comments", strictLimit, reelController.AddComment)
		reelRoutes.GET("/:id/comments", reelController.GetComments)
		reelRoutes.POST("/:id/comments/:commentId/replies", strictLimit, reelController.AddReply)
		reelRoutes.POST("/:id/comments/:commentId/react", strictLimit, reelController.ReactToComment)
		reelRoutes.POST("/:id/react", strictLimit, reelController.ReactToReel)
		reelRoutes.POST("/:id/view", reelController.IncrementView)
		// reelRoutes.DELETE("/:id", reelController.DeleteReel)
	}

	// Marketplace Routes
	marketplaceRoutes := api.Group("/marketplace")
	{
		marketplaceRoutes.GET("/categories", marketplaceController.GetCategories)
		marketplaceRoutes.POST("/products", marketplaceController.CreateProduct)
		marketplaceRoutes.GET("/products", marketplaceController.ListProducts)
		marketplaceRoutes.GET("/products/:id", marketplaceController.GetProduct)
		marketplaceRoutes.DELETE("/products/:id", marketplaceController.DeleteProduct)
		marketplaceRoutes.POST("/products/:id/sold", marketplaceController.MarkSold)
		marketplaceRoutes.POST("/products/:id/save", marketplaceController.ToggleSave)
		marketplaceRoutes.GET("/conversations", marketplaceController.GetConversations)
	}

	// Event Routes
	eventGroup := api.Group("/events")
	{
		eventGroup.POST("", eventController.CreateEvent)
		eventGroup.GET("", eventController.ListEvents)
		eventGroup.GET("/my-events", eventController.GetMyEvents)
		eventGroup.GET("/birthdays", eventController.GetBirthdays)
		eventGroup.GET("/:id", eventController.GetEvent)
		eventGroup.PUT("/:id", eventController.UpdateEvent)
		eventGroup.DELETE("/:id", eventController.DeleteEvent)
		eventGroup.POST("/:id/rsvp", eventController.RSVP)
	}

	// WebSocket endpoint
	webSocketRouter.GET("/ws", wsMiddleware, func(c *gin.Context) {
		// Track WebSocket connection
		config.IncWebsocketConnections(metrics)
		defer config.DecWebsocketConnections(metrics)

		websocket.ServeWs(c, hub)
	})

	// Start HTTP server
	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: router,
	}

	// Start WebSocket server
	wsServer := &http.Server{
		Addr:    ":" + cfg.WebSocketPort,
		Handler: webSocketRouter,
	}

	// Graceful shutdown handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("HTTP server starting on port %s", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	go func() {
		log.Printf("WebSocket server listening on %s", wsServer.Addr)
		if err := wsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("WebSocket server error: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	if err := wsServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("WebSocket server shutdown error: %v", err)
	}

	log.Println("Server exited properly")
}
