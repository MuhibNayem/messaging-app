package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	MongoURI         string
	MongoUser        string
	MongoPassword    string
	DBName           string
	KafkaBrokers     []string
	JWTSecret        string
	ServerPort       string
	KafkaTopic       string
	WebSocketPort    string
	RedisURLs        []string
	RedisPass        string
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration
	PrometheusPort   string
	RateLimitEnabled bool
	RateLimitLimit   float64
	RateLimitBurst   int
	StorageEndpoint  string
	StorageAccessKey string
	StorageSecretKey string
	StorageBucket    string
	StorageUseSSL    bool
	StoragePublicURL string
}

func LoadConfig() *Config {
	err := godotenv.Load(".env")
	if err != nil {
		log.Println("Using environment variables directly")
	}

	accessTTL, _ := strconv.Atoi(getEnv("ACCESS_TOKEN_TTL", "15"))
	refreshTTL, _ := strconv.Atoi(getEnv("REFRESH_TOKEN_TTL", "7"))
	rateLimitEnabled, _ := strconv.ParseBool(getEnv("RATE_LIMIT_ENABLED", "true"))
	rateLimitLimit, _ := strconv.ParseFloat(getEnv("RATE_LIMIT_LIMIT", "100"), 64)
	rateLimitBurst, _ := strconv.Atoi(getEnv("RATE_LIMIT_BURST", "100"))
	storageUseSSL, _ := strconv.ParseBool(getEnv("STORAGE_USE_SSL", "false"))

	return &Config{
		MongoURI:         getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoUser:        getEnv("MONGO_USER", ""),
		MongoPassword:    getEnv("MONGO_PASSWORD", ""),
		DBName:           getEnv("DB_NAME", "messaging_app"),
		KafkaBrokers:     strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ","),
		JWTSecret:        getEnv("JWT_SECRET", "very-secret-key"),
		ServerPort:       getEnv("SERVER_PORT", "8080"),
		KafkaTopic:       getEnv("KAFKA_TOPIC", "messages"),
		WebSocketPort:    getEnv("WS_PORT", "8081"),
		RedisURLs:        strings.Split(getEnv("REDIS_URL", "localhost:6379"), ","),
		RedisPass:        getEnv("REDIS_PASS", ""),
		AccessTokenTTL:   time.Minute * time.Duration(accessTTL),
		RefreshTokenTTL:  time.Hour * 24 * time.Duration(refreshTTL),
		PrometheusPort:   getEnv("PROMETHEUS_PORT", "9091"),
		RateLimitEnabled: rateLimitEnabled,
		RateLimitLimit:   rateLimitLimit,
		RateLimitBurst:   rateLimitBurst,
		StorageEndpoint:  getEnv("STORAGE_ENDPOINT", "minio:9000"),
		StorageAccessKey: getEnv("STORAGE_ACCESS_KEY", "minioadmin"),
		StorageSecretKey: getEnv("STORAGE_SECRET_KEY", "minioadmin"),
		StorageBucket:    getEnv("STORAGE_BUCKET", "connectify-uploads"),
		StorageUseSSL:    storageUseSSL,
		StoragePublicURL: getEnv("STORAGE_PUBLIC_URL", "http://localhost:9000"),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
