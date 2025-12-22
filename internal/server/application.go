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
	"messaging-app/internal/eventsclient"
	"messaging-app/internal/graph"
	"messaging-app/internal/kafka"
	"messaging-app/internal/marketplaceclient"
	"messaging-app/internal/realtime"
	"messaging-app/internal/services"
	"messaging-app/internal/websocket"

	pkgkafka "gitlab.com/spydotech-group/shared-entity/kafka"
	realtimepb "gitlab.com/spydotech-group/shared-entity/proto/realtime/v1"
	"gitlab.com/spydotech-group/shared-entity/redis"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/grpc"
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

	kafkaProducer           *kafka.MessageProducer
	userKafkaProducer       *kafka.MessageProducer
	friendshipKafkaProducer *kafka.MessageProducer
	dlqProducer             *pkgkafka.DLQProducer
	kafkaConsumer           *kafka.MessageConsumer
	notificationConsumer    *kafka.NotificationConsumer
	eventsClient            *eventsclient.Client
	marketplaceClient       *marketplaceclient.Client
	messageArchiveService   *services.MessageArchiveService
	cleanupService          *services.CleanupService
	hub                     *websocket.Hub
	mainRouter              *gin.Engine
	websocketRouter         *gin.Engine
	httpServer              *http.Server
	wsServer                *http.Server
	metricsServer           *http.Server
	realtimeServer          *grpc.Server
	realtimeListener        net.Listener
	backgroundWorkers       []func()
	backgroundWorkerCancel  context.CancelFunc

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

	errCh := make(chan error, 4)
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
	if a.realtimeServer != nil && a.realtimeListener != nil {
		go func() {
			log.Printf("Realtime gRPC server starting on %s", a.realtimeListener.Addr())
			if err := a.realtimeServer.Serve(a.realtimeListener); err != nil {
				errCh <- fmt.Errorf("Realtime gRPC server failed: %w", err)
			}
		}()
	}

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
		if a.realtimeServer != nil {
			a.realtimeServer.GracefulStop()
		}
		if a.realtimeListener != nil {
			_ = a.realtimeListener.Close()
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
	if a.userKafkaProducer != nil {
		_ = a.userKafkaProducer.Close()
	}
	if a.friendshipKafkaProducer != nil {
		_ = a.friendshipKafkaProducer.Close()
	}
	if a.dlqProducer != nil {
		a.dlqProducer.Close()
	}
	if a.eventsClient != nil {
		_ = a.eventsClient.Close()
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
	a.mongoClient, a.db, err = InitMongo(a.ctx, a.cfg)
	if err != nil {
		return err
	}

	a.redisClient, err = InitRedis(a.cfg)
	if err != nil {
		return err
	}

	a.neo4jClient, err = InitNeo4j(a.cfg)
	if err != nil {
		log.Printf("Warning: Failed to connect to Neo4j: %v", err)
	}

	a.cassandra, err = InitCassandra(a.cfg)
	if err != nil {
		log.Printf("Warning: Failed to connect to Cassandra: %v", err)
	}

	a.kafkaProducer = kafka.NewMessageProducer(a.cfg.KafkaBrokers, a.cfg.KafkaTopic)
	a.userKafkaProducer = kafka.NewMessageProducer(a.cfg.KafkaBrokers, "user-events")
	a.friendshipKafkaProducer = kafka.NewMessageProducer(a.cfg.KafkaBrokers, "friendship-events")
	a.dlqProducer = pkgkafka.NewDLQProducer(a.cfg.KafkaBrokers)

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

	client, err := eventsclient.New(a.ctx, a.cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to events service: %w", err)
	}
	a.eventsClient = client
	servicesBundle.Event = client
	servicesBundle.EventRecommendation = client

	// Initialize marketplace gRPC client
	marketplaceClient, err := marketplaceclient.New(a.ctx, a.cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to marketplace service: %w", err)
	}
	a.marketplaceClient = marketplaceClient
	// TODO: Update servicesBundle.Marketplace to use marketplaceClient

	controllerConfig := buildControllers(a.cfg, servicesBundle, a.marketplaceClient)

	a.kafkaConsumer = kafka.NewMessageConsumer(a.cfg.KafkaBrokers, a.cfg.KafkaTopic, "message-group", a.hub)
	a.notificationConsumer = kafka.NewNotificationConsumer(a.cfg.KafkaBrokers, "notifications_events", "notification-group", a.hub, repos.Notification, a.dlqProducer)

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
	if err := a.initRealtimeServer(); err != nil {
		return err
	}
	return nil
}

func (a *Application) initRealtimeServer() error {
	if a.hub == nil {
		return fmt.Errorf("hub is not initialized")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("", a.cfg.RealtimeGRPCPort))
	if err != nil {
		return fmt.Errorf("failed to bind realtime gRPC listener: %w", err)
	}
	a.realtimeListener = listener
	a.realtimeServer = grpc.NewServer()
	realtimepb.RegisterRealtimeServiceServer(a.realtimeServer, realtime.NewServer(a.hub))
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
