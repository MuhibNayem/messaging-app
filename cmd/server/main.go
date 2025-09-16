package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"messaging-app/config"
	"messaging-app/internal/controllers"
	"messaging-app/internal/kafka"
	"messaging-app/internal/redis"
	"messaging-app/internal/repositories"
	"messaging-app/internal/services"
	"messaging-app/internal/websocket"
	"messaging-app/pkg/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

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

	// Initialize Repositories
	userRepo := repositories.NewUserRepository(db)
	messageRepo := repositories.NewMessageRepository(db)
	groupRepo := repositories.NewGroupRepository(db)
	friendshipRepo := repositories.NewFriendshipRepository(db)
	feedRepo := repositories.NewFeedRepository(db)
	privacyRepo := repositories.NewPrivacyRepository(db)

	// Initialize Kafka Producer
	kafkaProducer := kafka.NewMessageProducer(cfg.KafkaBrokers, cfg.KafkaTopic)
	defer func() {
		if err := kafkaProducer.Close(); err != nil {
			log.Printf("Error closing Kafka producer: %v", err)
		}
	}()

	// Initialize WebSocket Hub
	hub := websocket.NewHub(redisClient, groupRepo)

	// Initialize Kafka Consumer
	kafkaConsumer := kafka.NewMessageConsumer(cfg.KafkaBrokers, cfg.KafkaTopic, "message-group", hub)
	go func() {
		kafkaConsumer.ConsumeMessages(context.Background())
	}()

	// Initialize Services
	authService := services.NewAuthService(userRepo, cfg.JWTSecret, redisClient.GetClient(), cfg)
	userService := services.NewUserService(userRepo)
	messageService := services.NewMessageService(messageRepo, groupRepo, friendshipRepo, kafkaProducer, redisClient.GetClient())
	groupService := services.NewGroupService(groupRepo, userRepo)
	friendshipService := services.NewFriendshipService(friendshipRepo, userRepo)
	feedService := services.NewFeedService(feedRepo, userRepo, friendshipRepo, privacyRepo, kafkaProducer)
	privacyService := services.NewPrivacyService(privacyRepo, userRepo)

	// Initialize Controllers
	authController := controllers.NewAuthController(authService)
	userController := controllers.NewUserController(userService)
	friendshipController := controllers.NewFriendshipController(friendshipService)
	groupController := controllers.NewGroupController(groupService, userService)
	messageController := controllers.NewMessageController(messageService)
	feedController := controllers.NewFeedController(feedService, userService, privacyService)
	privacyController := controllers.NewPrivacyController(privacyService, userService)

	// Initialize Gin Router with metrics middleware
	router := gin.Default()
	router.Use(config.MetricsMiddleware(metrics))
	router.Use(cors.Default())

	// WebSocket router (without metrics middleware)
	webSocketRouter := gin.Default()
	webSocketRouter.Use(cors.Default())

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

	// Auth routes
	router.POST("/api/auth/register", authController.Register)
	router.POST("/api/auth/login", authController.Login)
	router.POST("/api/auth/refresh", authController.Refresh)
	router.POST("/api/auth/logout", authController.Logout)

	// Protected routes
	authMiddleware := middleware.AuthMiddleware(cfg.JWTSecret, redisClient.GetClient())
	api := router.Group("/api", authMiddleware)
	{
	api.GET("/user", authMiddleware, userController.GetUser)
	api.PUT("/user", authMiddleware, userController.UpdateUser)
	api.PUT("/user/email", authMiddleware, userController.UpdateEmail)
	api.PUT("/user/password", authMiddleware, userController.UpdatePassword)
	api.PUT("/user/2fa", authMiddleware, userController.ToggleTwoFactor)
	api.PUT("/user/deactivate", authMiddleware, userController.DeactivateAccount)
	api.GET("/users", authMiddleware, userController.ListUsers)
	api.GET("/users/:id", authMiddleware, userController.GetUserByID)
	api.PUT("/user/privacy", authMiddleware, userController.UpdatePrivacySettings)

	// Feed Routes
	api.POST("/posts", authMiddleware, feedController.CreatePost)
	api.GET("/posts/:id", authMiddleware, feedController.GetPostByID)
	api.GET("/posts/:postId/comments", authMiddleware, feedController.GetCommentsByPostID)
	api.GET("/posts/:postId/reactions", authMiddleware, feedController.GetReactionsByPostID)
	api.PUT("/posts/:id", authMiddleware, feedController.UpdatePost)
	api.DELETE("/posts/:id", authMiddleware, feedController.DeletePost)

	api.POST("/comments", authMiddleware, feedController.CreateComment)
	api.GET("/comments/:commentId/replies", authMiddleware, feedController.GetRepliesByCommentID)
	api.GET("/comments/:commentId/reactions", authMiddleware, feedController.GetReactionsByCommentID)
	api.PUT("/comments/:id", authMiddleware, feedController.UpdateComment)
	api.DELETE("/posts/:postId/comments/:commentId", authMiddleware, feedController.DeleteComment)

	api.POST("/comments/:commentId/replies", authMiddleware, feedController.CreateReply)
	api.PUT("/comments/:commentId/replies/:replyId", authMiddleware, feedController.UpdateReply)
	api.DELETE("/comments/:commentId/replies/:replyId", authMiddleware, feedController.DeleteReply)
	api.GET("/replies/:replyId/reactions", authMiddleware, feedController.GetReactionsByReplyID)

	api.POST("/reactions", authMiddleware, feedController.CreateReaction)
	api.DELETE("/reactions/:reactionId", authMiddleware, feedController.DeleteReaction)

	// Privacy Routes
	api.GET("/privacy/settings", authMiddleware, privacyController.GetUserPrivacySettings)
	api.PUT("/privacy/settings", authMiddleware, privacyController.UpdateUserPrivacySettings)

	api.POST("/privacy/lists", authMiddleware, privacyController.CreateCustomPrivacyList)
	api.GET("/privacy/lists", authMiddleware, privacyController.GetCustomPrivacyListsByUserID)
	api.GET("/privacy/lists/:id", authMiddleware, privacyController.GetCustomPrivacyListByID)
	api.PUT("/privacy/lists/:id", authMiddleware, privacyController.UpdateCustomPrivacyList)
	api.DELETE("/privacy/lists/:id", authMiddleware, privacyController.DeleteCustomPrivacyList)

	api.POST("/privacy/lists/:id/members", authMiddleware, privacyController.AddMemberToCustomPrivacyList)
	api.DELETE("/privacy/lists/:id/members/:member_id", authMiddleware, privacyController.RemoveMemberFromCustomPrivacyList)

	// Message Routes
	api.POST("/messages", authMiddleware, messageController.SendMessage)
	api.GET("/messages", authMiddleware, messageController.GetMessages)
	api.POST("/messages/seen", authMiddleware, messageController.MarkMessagesAsSeen)
	api.GET("/messages/unread", authMiddleware, messageController.GetUnreadCount)
	api.DELETE("/messages/:id", authMiddleware, messageController.DeleteMessage)

		// Group endpoints
		api.POST("/groups", groupController.CreateGroup)        
		api.GET("/groups/:id", groupController.GetGroup)         
		api.PATCH("/groups/:id", groupController.UpdateGroup)    
		api.POST("/groups/:id/members", groupController.AddMember)
		api.DELETE("/groups/:id/members/:user_id", groupController.RemoveMember) 
		api.POST("/groups/:id/admins", groupController.AddAdmin)
		api.GET("/users/me/groups", groupController.GetUserGroups)

		// Friendship endpoints
		api.POST("/friendships/requests", friendshipController.SendRequest)
		api.POST("/friendships/requests/:id/respond", friendshipController.RespondToRequest)
		api.GET("/friendships", friendshipController.ListFriendships)
		api.GET("/friendships/check", friendshipController.CheckFriendship)
		api.DELETE("/friendships/:id", friendshipController.Unfriend)
		api.POST("/friendships/block/:user_id", friendshipController.BlockUser)
		api.DELETE("/friendships/block/:user_id", friendshipController.UnblockUser)
		api.GET("/friendships/block/:user_id/status", friendshipController.IsBlocked)
		api.GET("/friendships/blocked", friendshipController.GetBlockedUsers)
	}

	webSocketRouter.GET("/ws", authMiddleware, func(c *gin.Context) {
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

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	if err := wsServer.Shutdown(ctx); err != nil {
		log.Printf("WebSocket server shutdown error: %v", err)
	}

	log.Println("Server exited properly")
}
