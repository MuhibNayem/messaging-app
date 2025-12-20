package utils

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

// String helpers
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func RemoveString(slice []string, s string) []string {
	result := []string{}
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

// Crypto helpers
func GenerateRandomString(length int) (string, error) {
	bytes := make([]byte, length/2+1)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes)[:length], nil
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// UUID helpers
func NewUUID() string {
	return uuid.New().String()
}

func IsValidUUID(u string) bool {
	_, err := uuid.Parse(u)
	return err == nil
}

// Time helpers
func ParseDuration(durationStr string) (time.Duration, error) {
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		if strings.HasSuffix(durationStr, "d") {
			days := strings.TrimSuffix(durationStr, "d")
			daysInt, err := time.ParseDuration(days + "h")
			if err != nil {
				return 0, err
			}
			return daysInt * 24, nil
		}
		return 0, err
	}
	return duration, nil
}

// JSON helpers
func StructToMap(obj interface{}) (map[string]interface{}, error) {
	out := make(map[string]interface{})
	j, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(j, &out)
	return out, err
}

func MapToStruct(m map[string]interface{}, out interface{}) error {
	j, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(j, out)
}

// HTTP helpers
func WriteJSONResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func ReadJSONBody(r *http.Request, dst interface{}) error {
	if r.Body == nil {
		return errors.New("empty request body")
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

// Validation helpers
func IsEmpty(value interface{}) bool {
	if value == nil {
		return true
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.String, reflect.Array, reflect.Map, reflect.Slice:
		return v.Len() == 0
	case reflect.Ptr:
		return v.IsNil()
	default:
		return false
	}
}

func ValidationError(fields map[string]string) map[string]interface{} {
	return map[string]interface{}{
		"error":   true,
		"message": "validation failed",
		"fields":  fields,
	}
}

// WebSocket helpers
func NormalizeWSMessage(message []byte) ([]byte, error) {
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		return nil, fmt.Errorf("invalid JSON format")
	}
	return json.Marshal(msg)
}

// Logging helpers
func LogError(err error) string {
	return fmt.Sprintf("[ERROR] %v", err)
}

func LogRequest(r *http.Request) string {
	return fmt.Sprintf("%s %s %s", r.Method, r.URL.Path, r.RemoteAddr)
}

// RespondWithError sends a JSON error response with the given status code and message
func RespondWithError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"status":  statusCode,
			"message": message,
		},
	})
	c.Abort()
}

// RespondWithSuccess sends a JSON success response with the given data
func RespondWithSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, data)
}

// GetStatusCode determines the appropriate HTTP status code for different error types
func GetStatusCode(err error) int {
	if err == nil {
		return http.StatusOK
	}

	switch err.Error() {
	case "not found", "user not found", "group not found":
		return http.StatusNotFound
	case "already exists", "user is already a group member", "user is already an admin":
		return http.StatusConflict
	case "unauthorized", "authentication required":
		return http.StatusUnauthorized
	case "forbidden", "only admins can add members", "only admins can add other admins":
		return http.StatusForbidden
	case "invalid input", "no valid fields to update":
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// GetUserIDFromContext extracts user ID from Gin context (set by auth middleware)
func GetUserIDFromContext(c *gin.Context) (primitive.ObjectID, error) {
	userID, exists := c.Get("userID")
	if !exists {
		return primitive.NilObjectID, fmt.Errorf("authentication required")
	}

	switch v := userID.(type) {
	case string:
		return primitive.ObjectIDFromHex(v)
	case primitive.ObjectID:
		return v, nil
	default:
		return primitive.NilObjectID, fmt.Errorf("invalid user ID type")
	}
}

func GetUserIDFromClaims(claims jwt.Claims) (primitive.ObjectID, error) {
	mapClaims, ok := claims.(jwt.MapClaims)
	if !ok {
		return primitive.NilObjectID, errors.New("invalid claims type")
	}

	userIDKey := "id"
	userIDValue, ok := mapClaims[userIDKey].(string)
	if !ok {
		return primitive.NilObjectID, fmt.Errorf("claim '%s' not found or invalid type", userIDKey)
	}

	objectID, err := primitive.ObjectIDFromHex(userIDValue)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("invalid user ID format: %v", err)
	}

	return objectID, nil
}

func getClaims(token *jwt.Token) (jwt.MapClaims, error) {
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token")
	}

	if claims["type"] != "access" {
		return nil, fmt.Errorf("invalid token type")
	}

	return claims, nil
}

// GetConversationID generates a consistent conversation ID for two users (direct message)
func GetConversationID(user1, user2 primitive.ObjectID) string {
	if user1.Hex() < user2.Hex() {
		return fmt.Sprintf("dm_%s_%s", user1.Hex(), user2.Hex())
	}
	return fmt.Sprintf("dm_%s_%s", user2.Hex(), user1.Hex())
}
