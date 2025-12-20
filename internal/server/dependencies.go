package server

import (
	"context"
	"log"

	"messaging-app/config"
	"messaging-app/internal/cache"
	"messaging-app/internal/controllers"
	cassdb "messaging-app/internal/db"
	"messaging-app/internal/graph"
	notifications "messaging-app/internal/notifications"
	"messaging-app/internal/repositories"
	"messaging-app/internal/seeds"
	"messaging-app/internal/services"

	"go.mongodb.org/mongo-driver/mongo"
)

type repositoryBundle struct {
	User             *repositories.UserRepository
	Message          *repositories.MessageRepository
	Group            *repositories.GroupRepository
	Friendship       *repositories.FriendshipRepository
	Feed             *repositories.FeedRepository
	Privacy          repositories.PrivacyRepository
	Notification     *repositories.NotificationRepository
	Conversation     *repositories.ConversationRepository
	Community        *repositories.CommunityRepository
	Story            *repositories.StoryRepository
	Reel             *repositories.ReelRepository
	Marketplace      *repositories.MarketplaceRepository
	Event            *repositories.EventRepository
	EventInvitation  *repositories.EventInvitationRepository
	EventPost        *repositories.EventPostRepository
	MessageCassandra *repositories.MessageCassandraRepository
	GroupActivity    *repositories.GroupActivityRepository
}

func buildRepositories(db *mongo.Database, cassandra *cassdb.CassandraClient) repositoryBundle {
	userRepo := repositories.NewUserRepository(db)
	groupRepo := repositories.NewGroupRepository(db)

	return repositoryBundle{
		User:             userRepo,
		Message:          repositories.NewMessageRepository(db),
		Group:            groupRepo,
		Friendship:       repositories.NewFriendshipRepository(db),
		Feed:             repositories.NewFeedRepository(db),
		Privacy:          repositories.NewPrivacyRepository(db),
		Notification:     repositories.NewNotificationRepository(db),
		Conversation:     repositories.NewConversationRepository(db, userRepo, groupRepo),
		Community:        repositories.NewCommunityRepository(db),
		Story:            repositories.NewStoryRepository(db),
		Reel:             repositories.NewReelRepository(db),
		Marketplace:      repositories.NewMarketplaceRepository(db),
		Event:            repositories.NewEventRepository(db),
		EventInvitation:  repositories.NewEventInvitationRepository(db),
		EventPost:        repositories.NewEventPostRepository(db),
		MessageCassandra: repositories.NewMessageCassandraRepository(cassandra),
		GroupActivity:    repositories.NewGroupActivityRepository(cassandra),
	}
}

type graphBundle struct {
	UserGraph  *repositories.UserGraphRepository
	EventGraph *repositories.EventGraphRepository
	GroupGraph *repositories.GroupGraphRepository
}

func buildGraphRepositories(neo4jClient *graph.Neo4jClient) graphBundle {
	if neo4jClient == nil {
		return graphBundle{}
	}
	return graphBundle{
		UserGraph:  repositories.NewUserGraphRepository(neo4jClient.Driver),
		EventGraph: repositories.NewEventGraphRepository(neo4jClient.Driver),
		GroupGraph: repositories.NewGroupGraphRepository(neo4jClient.Driver),
	}
}

func seedMarketplace(ctx context.Context, repo *repositories.MarketplaceRepository) {
	if repo == nil {
		return
	}
	marketplaceSeeder := seeds.NewMarketplaceSeeder(repo)
	if err := marketplaceSeeder.SeedCategories(ctx); err != nil {
		log.Printf("Warning: Failed to seed categories: %v", err)
	}
}

type serviceBundle struct {
	Auth                *services.AuthService
	Notification        *notifications.NotificationService
	Storage             *services.StorageService
	Feed                *services.FeedService
	User                *services.UserService
	Group               *services.GroupService
	Friendship          *services.FriendshipService
	Message             *services.MessageService
	Privacy             *services.PrivacyService
	Search              *services.SearchService
	Conversation        *services.ConversationService
	Community           *services.CommunityService
	Story               *services.StoryService
	Reel                *services.ReelService
	Marketplace         *services.MarketplaceService
	Event               *services.EventService
	EventRecommendation *services.EventRecommendationService
	EventCache          *cache.EventCache
	Cleanup             *services.CleanupService
}

func (a *Application) buildBaseServices(repos repositoryBundle, graphs graphBundle) (serviceBundle, error) {
	authService := services.NewAuthService(repos.User, a.cfg.JWTSecret, a.redisClient.GetClient(), a.cfg, graphs.UserGraph)
	notificationService := notifications.NewNotificationService(repos.Notification, repos.User, a.kafkaProducer)

	storageService, err := services.NewStorageService(a.cfg)
	if err != nil {
		return serviceBundle{}, err
	}

	if a.cassandra != nil {
		a.messageArchiveService = services.NewMessageArchiveService(
			a.cassandra,
			storageService,
			a.redisClient,
			a.cfg,
		)
		repos.MessageCassandra.SetArchiveFetcher(a.messageArchiveService)
	}

	feedService := services.NewFeedService(repos.Feed, repos.User, repos.Friendship, repos.Community, repos.Privacy, a.kafkaProducer, notificationService, storageService)
	userService := services.NewUserService(repos.User, repos.Reel, a.redisClient.GetClient(), feedService)
	groupService := services.NewGroupService(repos.Group, repos.User, repos.GroupActivity, a.cassandra, a.kafkaProducer, a.redisClient.GetClient(), graphs.GroupGraph)
	friendshipService := services.NewFriendshipService(repos.Friendship, repos.User, graphs.UserGraph)
	messageService := services.NewMessageService(repos.Message, repos.Group, repos.Friendship, a.kafkaProducer, a.redisClient.GetClient(), repos.User, notificationService, repos.MessageCassandra, repos.GroupActivity)
	privacyService := services.NewPrivacyService(repos.Privacy, repos.User)
	searchService := services.NewSearchService(repos.User, repos.Feed, repos.Friendship)
	conversationService := services.NewConversationService(repos.Conversation, repos.MessageCassandra, repos.User, repos.Group)
	communityService := services.NewCommunityService(repos.Community, repos.User)
	storyService := services.NewStoryService(repos.Story, repos.User, repos.Friendship)
	reelService := services.NewReelService(repos.Reel, repos.User, repos.Friendship)
	marketplaceService := services.NewMarketplaceService(repos.Marketplace, repos.User, repos.MessageCassandra)

	eventCache := cache.NewEventCache(a.redisClient)
	cleanupService := services.NewCleanupService(repos.Story, storageService)

	return serviceBundle{
		Auth:                authService,
		Notification:        notificationService,
		Storage:             storageService,
		Feed:                feedService,
		User:                userService,
		Group:               groupService,
		Friendship:          friendshipService,
		Message:             messageService,
		Privacy:             privacyService,
		Search:              searchService,
		Conversation:        conversationService,
		Community:           communityService,
		Story:               storyService,
		Reel:                reelService,
		Marketplace:         marketplaceService,
		EventCache:          eventCache,
		Cleanup:             cleanupService,
		Event:               nil, // initialized after hub/broadcaster are ready
		EventRecommendation: nil,
	}, nil
}

func buildControllers(cfg *config.Config, services serviceBundle) routerConfig {
	return routerConfig{
		authController:         controllers.NewAuthController(services.Auth, cfg),
		userController:         controllers.NewUserController(services.User),
		friendshipController:   controllers.NewFriendshipController(services.Friendship),
		groupController:        controllers.NewGroupController(services.Group, services.User),
		messageController:      controllers.NewMessageController(services.Message, services.Storage, services.Group),
		feedController:         controllers.NewFeedController(services.Feed, services.User, services.Privacy, services.Storage),
		privacyController:      controllers.NewPrivacyController(services.Privacy, services.User),
		searchController:       controllers.NewSearchController(services.Search),
		notificationController: controllers.NewNotificationController(services.Notification),
		conversationController: controllers.NewConversationController(services.Conversation),
		uploadController:       controllers.NewUploadController(services.Storage),
		communityController:    controllers.NewCommunityController(services.Community),
		storyController:        controllers.NewStoryController(services.Story),
		reelController:         controllers.NewReelController(services.Reel),
		marketplaceController:  controllers.NewMarketplaceController(services.Marketplace),
		eventController:        controllers.NewEventController(services.Event, services.EventRecommendation),
	}
}
