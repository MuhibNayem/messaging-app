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
	MongoURI       string
	MongoUser      string
	MongoPassword  string
	DBName         string
	KafkaBrokers   []string
	JWTSecret      string
	ServerPort     string
	KafkaTopic     string
	WebSocketPort  string
	RedisURLs      []string
	RedisPass      string
	AccessTokenTTL time.Duration
	RefreshTokenTTL time.Duration
	PrometheusPort string
}

func LoadConfig() *Config {
	err := godotenv.Load(".env")
	if err != nil {
		log.Println("Using environment variables directly")
	}

	accessTTL, _ := strconv.Atoi(getEnv("ACCESS_TOKEN_TTL", "15"))
	refreshTTL, _ := strconv.Atoi(getEnv("REFRESH_TOKEN_TTL", "7"))

	return &Config{
		MongoURI:       getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoUser:      getEnv("MONGO_USER", ""),
		MongoPassword:  getEnv("MONGO_PASSWORD", ""),
		DBName:         getEnv("DB_NAME", "messaging_app"),
		KafkaBrokers:   strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ","),
		JWTSecret:      getEnv("JWT_SECRET", "very-secret-key"),
		ServerPort:     getEnv("SERVER_PORT", "8080"),
		KafkaTopic:     getEnv("KAFKA_TOPIC", "messages"),
		WebSocketPort:  getEnv("WS_PORT", "8081"),
		RedisURLs:      strings.Split(getEnv("REDIS_URL", "localhost:6379"), ","),
		RedisPass:      getEnv("REDIS_PASS", ""),
		AccessTokenTTL: time.Minute * time.Duration(accessTTL),
		RefreshTokenTTL: time.Hour * 24 * time.Duration(refreshTTL),
		PrometheusPort: getEnv("PROMETHEUS_PORT", "9090"),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}