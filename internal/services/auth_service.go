package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"messaging-app/config"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userRepo      *repositories.UserRepository
	jwtSecret     string
	redisClient   *redis.ClusterClient
	cfg           *config.Config
	userGraphRepo *repositories.UserGraphRepository
}

func NewAuthService(
	userRepo *repositories.UserRepository,
	jwtSecret string,
	redisClient *redis.ClusterClient,
	cfg *config.Config,
	userGraphRepo *repositories.UserGraphRepository,
) *AuthService {
	return &AuthService{
		userRepo:      userRepo,
		jwtSecret:     jwtSecret,
		redisClient:   redisClient,
		cfg:           cfg,
		userGraphRepo: userGraphRepo,
	}
}

func (s *AuthService) Register(ctx context.Context, user *models.User) (*models.AuthResponse, error) {
	existingUserEmail, _ := s.userRepo.FindUserByEmail(ctx, user.Email)
	if existingUserEmail != nil {
		return nil, errors.New("user email already exists")
	}

	existingUserUsername, _ := s.userRepo.FindUserByUserName(ctx, user.Username)
	if existingUserUsername != nil {
		return nil, errors.New("username already exists")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user.Password = string(hashedPassword)

	user.Friends = []primitive.ObjectID{}
	user.Blocked = []primitive.ObjectID{}

	user.SetDefaultPrivacySettings()

	createdUser, err := s.userRepo.CreateUser(ctx, user)
	if err != nil {
		return nil, err
	}

	// Sync new user to graph database immediately (async)
	if s.userGraphRepo != nil {
		go s.userGraphRepo.SyncUser(context.Background(), createdUser.ID)
	}

	accessToken, refreshToken, err := s.generateTokens(ctx, createdUser)
	if err != nil {
		return nil, err
	}

	return &models.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         createdUser.ToSafeResponse(),
	}, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*models.AuthResponse, error) {
	user, err := s.userRepo.FindUserByEmail(ctx, email)

	if err != nil {
		log.Printf("err in email: %s", err)
		return nil, errors.New("invalid credentials: please check email")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		log.Printf("err in pass matching: %s", err)
		return nil, errors.New("invalid credentials: please check password")
	}

	accessToken, refreshToken, err := s.generateTokens(ctx, user)
	if err != nil {
		return nil, err
	}

	return &models.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user.ToSafeResponse(),
	}, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*models.AuthResponse, error) {
	token, err := jwt.Parse(refreshToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid refresh token")
	}

	if claims["type"] != "refresh" {
		return nil, errors.New("invalid token type")
	}

	userID := claims["id"].(string)
	storedToken, err := s.redisClient.Get(ctx, "refresh:"+userID).Result()
	if err != nil || storedToken != refreshToken {
		return nil, errors.New("invalid refresh token")
	}

	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}

	user, err := s.userRepo.FindUserByID(ctx, objID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	newAccessToken, newRefreshToken, err := s.generateTokens(ctx, user)
	if err != nil {
		return nil, err
	}

	return &models.AuthResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
		User:         user.ToSafeResponse(),
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, userID, accessToken string) error {
	remainingTTL := s.getRemainingTTL(accessToken)
	if remainingTTL > 0 {
		err := s.redisClient.Set(ctx, "blacklist:"+accessToken, "1", time.Duration(remainingTTL)*time.Second).Err()
		if err != nil {
			return err
		}
	}

	err := s.redisClient.Del(ctx, "refresh:"+userID).Err()
	if err != nil {
		return err
	}

	return nil
}

func (s *AuthService) generateTokens(ctx context.Context, user *models.User) (string, string, error) {
	accessClaims := jwt.MapClaims{
		"id":    user.ID.Hex(),
		"email": user.Email,
		"type":  "access",
		"exp":   time.Now().Add(s.cfg.AccessTokenTTL).Unix(),
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", "", err
	}

	refreshClaims := jwt.MapClaims{
		"id":   user.ID.Hex(),
		"type": "refresh",
		"exp":  time.Now().Add(s.cfg.RefreshTokenTTL).Unix(),
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", "", err
	}

	err = s.redisClient.Set(ctx, "refresh:"+user.ID.Hex(), refreshTokenString, s.cfg.RefreshTokenTTL).Err()
	if err != nil {
		return "", "", err
	}

	return accessTokenString, refreshTokenString, nil
}

func (s *AuthService) getRemainingTTL(tokenString string) int64 {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return 0
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0
	}

	exp, ok := claims["exp"].(float64)
	if !ok {
		return 0
	}

	expTime := time.Unix(int64(exp), 0)
	return int64(time.Until(expTime).Seconds())
}
