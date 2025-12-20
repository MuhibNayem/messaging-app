package integration

import (
	"context"
	"messaging-app/config"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"
	"messaging-app/internal/services"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AuthIntegrationTestSuite struct {
	suite.Suite
	authService *services.AuthService
	userRepo    *repositories.UserRepository
	redisClient *redis.ClusterClient
	mongoClient *mongo.Client
	testDBName  string
	testUser    *models.User
	ctx         context.Context
}

func (suite *AuthIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	suite.testDBName = "test_auth_db"

	// Initialize Redis
	suite.redisClient = redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: []string{os.Getenv("REDIS_ADDR")},
	})

	// Initialize MongoDB
	mongoURI := os.Getenv("MONGO_URI")
	opts := options.Client().ApplyURI(mongoURI)
	suite.mongoClient, _ = mongo.Connect(suite.ctx, opts)

	// Create test repositories
	suite.userRepo = repositories.NewUserRepository(suite.mongoClient.Database(suite.testDBName))

	// Create auth service
	suite.authService = services.NewAuthService(
		suite.userRepo,
		config.LoadConfig().JWTSecret,
		suite.redisClient,
		config.LoadConfig(),
		nil, // No Neo4j for integration tests
	)

	// Create test user
	suite.testUser = &models.User{
		Email:    "test@example.com",
		Password: "password123",
	}
}

func (suite *AuthIntegrationTestSuite) TearDownSuite() {
	// Cleanup test database
	suite.mongoClient.Database(suite.testDBName).Drop(suite.ctx)
	suite.mongoClient.Disconnect(suite.ctx)
	suite.redisClient.Close()
}

func (suite *AuthIntegrationTestSuite) BeforeTest(suiteName, testName string) {
	// Clear data before each test
	suite.mongoClient.Database(suite.testDBName).Drop(suite.ctx)
	suite.redisClient.FlushDB(suite.ctx)
}

func TestAuthIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests")
	}
	suite.Run(t, new(AuthIntegrationTestSuite))
}

func (suite *AuthIntegrationTestSuite) TestRegisterAndLogin() {
	// Test registration
	authResponse, err := suite.authService.Register(suite.ctx, suite.testUser)
	suite.NoError(err)
	suite.NotEmpty(authResponse.AccessToken)
	suite.NotEmpty(authResponse.RefreshToken)

	// Test login with correct credentials
	loginResponse, err := suite.authService.Login(suite.ctx, suite.testUser.Email, suite.testUser.Password)
	suite.NoError(err)
	suite.NotEmpty(loginResponse.AccessToken)
	suite.NotEmpty(loginResponse.RefreshToken)

	// Test login with wrong password
	_, err = suite.authService.Login(suite.ctx, suite.testUser.Email, "wrongpassword")
	suite.Error(err)
}

func (suite *AuthIntegrationTestSuite) TestTokenRefresh() {
	// First register a user
	authResponse, err := suite.authService.Register(suite.ctx, suite.testUser)
	suite.NoError(err)

	// Test refresh token
	refreshResponse, err := suite.authService.RefreshToken(suite.ctx, authResponse.RefreshToken)
	suite.NoError(err)
	suite.NotEmpty(refreshResponse.AccessToken)
	suite.NotEmpty(refreshResponse.RefreshToken)

	// Test with invalid refresh token
	_, err = suite.authService.RefreshToken(suite.ctx, "invalidtoken")
	suite.Error(err)
}

func (suite *AuthIntegrationTestSuite) TestLogout() {
	// First register a user
	authResponse, err := suite.authService.Register(suite.ctx, suite.testUser)
	suite.NoError(err)

	// Test logout
	err = suite.authService.Logout(suite.ctx, suite.testUser.ID.Hex(), authResponse.AccessToken)
	suite.NoError(err)

	// Verify token is blacklisted
	_, err = suite.redisClient.Get(suite.ctx, "blacklist:"+authResponse.AccessToken).Result()
	suite.NoError(err)
}
