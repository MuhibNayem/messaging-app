package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"messaging-app/config"
	cassdb "messaging-app/internal/db"
	"messaging-app/internal/graph"

	"gitlab.com/spydotech-group/shared-entity/redis"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func InitMongo(ctx context.Context, cfg *config.Config) (*mongo.Client, *mongo.Database, error) {
	clientOptions := options.Client().
		ApplyURI(cfg.MongoURI).
		SetAuth(options.Credential{
			Username: cfg.MongoUser,
			Password: cfg.MongoPassword,
		}).
		SetMaxPoolSize(100).
		SetSocketTimeout(10 * time.Second)

	mongoClient, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	db := mongoClient.Database(cfg.DBName)
	if err := CreateIndexes(context.Background(), db); err != nil {
		_ = mongoClient.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("failed to create MongoDB indexes: %w", err)
	}
	return mongoClient, db, nil
}

func InitRedis(cfg *config.Config) (*redis.ClusterClient, error) {
	redisConfig := redis.Config{
		RedisURLs: cfg.RedisURLs,
		RedisPass: cfg.RedisPass,
	}
	redisClient := redis.NewClusterClient(redisConfig)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("failed to connect to Redis cluster within 60s")
		case <-ticker.C:
			if redisClient.IsAvailable(ctx) {
				return redisClient, nil
			}
			log.Println("Waiting for Redis cluster to be ready...")
		}
	}
}

func InitNeo4j(cfg *config.Config) (*graph.Neo4jClient, error) {
	client, err := graph.NewNeo4jClient(cfg.Neo4jURI, cfg.Neo4jUser, cfg.Neo4jPassword)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func InitCassandra(cfg *config.Config) (*cassdb.CassandraClient, error) {
	client, err := cassdb.NewCassandraClient(cfg.CassandraHosts, cfg.CassandraKeyspace, cfg.CassandraUser, cfg.CassandraPassword)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func CreateIndexes(ctx context.Context, db *mongo.Database) error {
	log.Println("Creating MongoDB text indexes...")

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

	postIndexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "content", Value: "text"},
			{Key: "hashtags", Value: "text"},
			{Key: "community_id", Value: 1},
		},
		Options: options.Index().SetName("post_text_index_v2").SetWeights(bson.D{
			{Key: "content", Value: 10},
			{Key: "hashtags", Value: 5},
		}),
	}

	messageIndexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "content", Value: "text"}},
		Options: options.Index().SetName("message_text_index"),
	}

	if _, err := db.Collection("users").Indexes().CreateOne(ctx, userIndexModel); err != nil {
		return fmt.Errorf("failed to create user text index: %w", err)
	}
	log.Println("User text index created successfully.")

	_, _ = db.Collection("posts").Indexes().DropOne(ctx, "post_text_index")

	if _, err := db.Collection("posts").Indexes().CreateOne(ctx, postIndexModel); err != nil {
		return fmt.Errorf("failed to create post text index: %w", err)
	}
	log.Println("Post text index created successfully.")

	if _, err := db.Collection("messages").Indexes().CreateOne(ctx, messageIndexModel); err != nil {
		return fmt.Errorf("failed to create message text index: %w", err)
	}
	log.Println("Message text index created successfully.")

	return nil
}
