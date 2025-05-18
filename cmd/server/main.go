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

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()
	config.InitMetrics()

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !redisClient.IsAvailable(ctx) {
		log.Fatal("Failed to connect to Redis cluster")
	}

	// Initialize Repositories
	userRepo := repositories.NewUserRepository(db)
	messageRepo := repositories.NewMessageRepository(db)
	groupRepo := repositories.NewGroupRepository(db)
	friendshipRepo := repositories.NewFriendshipRepository(db)


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

	// Initialize Controllers
	authController := controllers.NewAuthController(authService)
	userController := controllers.NewUserController(userService)
	messageController := controllers.NewMessageController(messageService)
	groupController := controllers.NewGroupController(groupService, userService)
	friendshipController := controllers.NewFriendshipController(friendshipService)

	// Initialize Gin Router
	router := gin.Default()
	webSocketRouter := gin.Default()

	// Prometheus metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Health check endpoints
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	
	router.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
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
	// wsAuthMiddleware := middleware.WSJwtAuthMiddleware(cfg.JWTSecret, redisClient.GetClient())
	api := router.Group("/api", authMiddleware)
	{

		// ========================= User ==========================

		// User Profile Endpoints
		api.GET("/user", userController.GetUser)          
		api.PUT("/user", userController.UpdateUser)      
		
		// User Discovery Endpoints
		api.GET("/users", userController.ListUsers)      
		
		// Suggested Additional Endpoints:
		api.GET("/users/:id", userController.GetUserByID) 

		//TODO: will add this features later 
		// Delete current account
		// api.DELETE("/user", userController.DeleteUser)  
		
		// Upload profile picture  
		// api.POST("/user/avatar", userController.UploadAvatar) 

		// ========================= Messages ==========================

		api.POST("/messages", messageController.SendMessage)
		api.GET("/messages/:id", messageController.GetMessages)

		// ========================= Groups ==========================

		// Group CRUD Operations
		api.POST("/groups", groupController.CreateGroup)        
		api.GET("/groups/:id", groupController.GetGroup)         
		api.PATCH("/groups/:id", groupController.UpdateGroup)    
		
		// Group Membership Management
		api.POST("/groups/:id/members", groupController.AddMember)
		api.DELETE("/groups/:id/members/:user_id", groupController.RemoveMember) 
		
		// Group Admin Management
		api.POST("/groups/:id/admins", groupController.AddAdmin)
		
		// User-Centric Group Operations
		api.GET("/users/me/groups", groupController.GetUserGroups) 
		
		// ========================= Friendships ==========================

		// Friend requests
		api.POST("/friendships/requests", friendshipController.SendRequest)
		api.POST("/friendships/requests/:id/respond", friendshipController.RespondToRequest)
		
		// Friendship management
		api.GET("/friendships", friendshipController.ListFriendships)
		api.GET("/friendships/check", friendshipController.CheckFriendship)
		api.DELETE("/friendships/:id", friendshipController.Unfriend)
		
		// Blocking functionality
		api.POST("/friendships/block/:user_id", friendshipController.BlockUser)
		api.DELETE("/friendships/block/:user_id", friendshipController.UnblockUser)
		api.GET("/friendships/block/:user_id/status", friendshipController.IsBlocked)
		api.GET("/friendships/blocked", friendshipController.GetBlockedUsers)
	}

	

	// WebSocket route
	webSocketRouter.GET("/ws",authMiddleware, func(c *gin.Context) {
		websocket.ServeWs(c, hub)
	})

	// Start HTTP server
	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: router,
	}

	// start websocket hub
	wsServer := &http.Server{
		Addr:    ":" + cfg.WebSocketPort,
		Handler: webSocketRouter,
	}

	// Graceful shutdown handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on port %s", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
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
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server exited properly")
}