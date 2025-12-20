package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"messaging-app/config"
	cassdb "messaging-app/internal/db"
	"messaging-app/internal/graph"
	"messaging-app/internal/kafka"
	"messaging-app/internal/models"
	"messaging-app/internal/redis"
	"messaging-app/internal/services"
	"messaging-app/internal/websocket"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

type Application struct {
	ctx    context.Context
	cancel context.CancelFunc

	cfg     *config.Config
	metrics *config.Metrics

	mongoClient *mongo.Client
	db          *mongo.Database
	redisClient *redis.ClusterClient
	neo4jClient *graph.Neo4jClient
	cassandra   *cassdb.CassandraClient

	kafkaProducer          *kafka.MessageProducer
	kafkaConsumer          *kafka.MessageConsumer
	notificationConsumer   *kafka.NotificationConsumer
	messageArchiveService  *services.MessageArchiveService
	cleanupService         *services.CleanupService
	hub                    *websocket.Hub
	eventBroadcaster       *HubEventBroadcaster
	mainRouter             *gin.Engine
	websocketRouter        *gin.Engine
	httpServer             *http.Server
	wsServer               *http.Server
	metricsServer          *http.Server
	backgroundWorkers      []func()
	backgroundWorkerCancel context.CancelFunc

	shutdownOnce sync.Once
}

func NewApplication(parentCtx context.Context, cfg *config.Config, metrics *config.Metrics) (*Application, error) {
	ctx, cancel := context.WithCancel(parentCtx)
	app := &Application{
		ctx:     ctx,
		cancel:  cancel,
		cfg:     cfg,
		metrics: metrics,
	}

	if err := app.bootstrap(); err != nil {
		app.Close()
		return nil, err
	}
	return app, nil
}

func (a *Application) Run() error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	a.startBackgroundWorkers()

	errCh := make(chan error, 3)
	startServer := func(srv *http.Server, name string) {
		go func() {
			log.Printf("%s starting on %s", name, srv.Addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("%s failed: %w", name, err)
			}
		}()
	}

	startServer(a.httpServer, "HTTP server")
	startServer(a.wsServer, "WebSocket server")
	startServer(a.metricsServer, "Metrics server")

	select {
	case <-quit:
		log.Println("Received shutdown signal")
		return a.Shutdown()
	case err := <-errCh:
		log.Printf("Server error: %v", err)
		return a.Shutdown()
	}
}

func (a *Application) Shutdown() error {
	var shutdownErr error
	a.shutdownOnce.Do(func() {
		log.Println("Shutting down application...")
		a.cancel()
		if a.backgroundWorkerCancel != nil {
			a.backgroundWorkerCancel()
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := a.httpServer.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
			shutdownErr = err
		}
		if err := a.wsServer.Shutdown(ctx); err != nil {
			log.Printf("WebSocket server shutdown error: %v", err)
			shutdownErr = err
		}
		if err := a.metricsServer.Shutdown(ctx); err != nil {
			log.Printf("Metrics server shutdown error: %v", err)
			shutdownErr = err
		}

		a.Close()
	})
	return shutdownErr
}

func (a *Application) Close() {
	if a.kafkaConsumer != nil {
		a.kafkaConsumer.Close()
	}
	if a.notificationConsumer != nil {
		_ = a.notificationConsumer.Close()
	}
	if a.kafkaProducer != nil {
		_ = a.kafkaProducer.Close()
	}
	if a.neo4jClient != nil {
		_ = a.neo4jClient.Close(context.Background())
	}
	if a.cassandra != nil {
		a.cassandra.Close()
	}
	if a.redisClient != nil {
		_ = a.redisClient.Close()
	}
	if a.mongoClient != nil {
		_ = a.mongoClient.Disconnect(context.Background())
	}
}

func (a *Application) bootstrap() error {
	var err error
	a.mongoClient, a.db, err = initMongo(a.ctx, a.cfg)
	if err != nil {
		return err
	}

	a.redisClient, err = initRedis(a.cfg)
	if err != nil {
		return err
	}

	a.neo4jClient, err = initNeo4j(a.cfg)
	if err != nil {
		log.Printf("Warning: Failed to connect to Neo4j: %v", err)
	}

	a.cassandra, err = initCassandra(a.cfg)
	if err != nil {
		log.Printf("Warning: Failed to connect to Cassandra: %v", err)
	}

	a.kafkaProducer = kafka.NewMessageProducer(a.cfg.KafkaBrokers, a.cfg.KafkaTopic)

	if err := a.initDomain(); err != nil {
		return err
	}

	return nil
}

func (a *Application) initDomain() error {
	repos := buildRepositories(a.db, a.cassandra)
	graphs := buildGraphRepositories(a.neo4jClient)
	seedMarketplace(a.ctx, repos.Marketplace)

	servicesBundle, err := a.buildBaseServices(repos, graphs)
	if err != nil {
		return fmt.Errorf("failed to initialize services: %w", err)
	}
	a.cleanupService = servicesBundle.Cleanup

	a.hub = websocket.NewHub(a.redisClient, repos.Group, repos.Feed, repos.User, repos.Friendship, repos.Message, repos.MessageCassandra, servicesBundle.Message)
	a.eventBroadcaster = &HubEventBroadcaster{hub: a.hub}

	servicesBundle.Event = services.NewEventService(
		repos.Event,
		repos.User,
		graphs.EventGraph,
		repos.EventInvitation,
		repos.EventPost,
		repos.Notification,
		servicesBundle.EventCache,
		a.eventBroadcaster,
	)
	servicesBundle.EventRecommendation = services.NewEventRecommendationService(
		repos.Event,
		graphs.EventGraph,
		repos.User,
		repos.Friendship,
		servicesBundle.EventCache,
	)

	controllerConfig := buildControllers(a.cfg, servicesBundle)

	a.kafkaConsumer = kafka.NewMessageConsumer(a.cfg.KafkaBrokers, a.cfg.KafkaTopic, "message-group", a.hub)
	a.notificationConsumer = kafka.NewNotificationConsumer(a.cfg.KafkaBrokers, "notifications_events", "notification-group", a.hub)

	a.mainRouter, a.websocketRouter = a.buildRouters(controllerConfig)

	a.httpServer = &http.Server{
		Addr:    net.JoinHostPort("", a.cfg.ServerPort),
		Handler: a.mainRouter,
	}
	a.wsServer = &http.Server{
		Addr:    net.JoinHostPort("", a.cfg.WebSocketPort),
		Handler: a.websocketRouter,
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", config.MetricsHandler())
	a.metricsServer = &http.Server{
		Addr:    net.JoinHostPort("", a.cfg.PrometheusPort),
		Handler: metricsMux,
	}
	return nil
}

func (a *Application) startBackgroundWorkers() {
	ctx, cancel := context.WithCancel(a.ctx)
	a.backgroundWorkerCancel = cancel

	if a.messageArchiveService != nil {
		go a.messageArchiveService.StartArchiveWorker(ctx)
	}
	go a.kafkaConsumer.ConsumeMessages(ctx)
	go a.notificationConsumer.Start(ctx)
	go a.cleanupService.StartCleanupWorker(ctx)
}

type HubEventBroadcaster struct {
	hub *websocket.Hub
}

func (h *HubEventBroadcaster) BroadcastRSVP(event models.EventRSVPEvent) {
	select {
	case h.hub.EventRSVPEvents <- event:
	default:
	}
}
