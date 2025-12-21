package controllers

import (
	"messaging-app/config"
	"gitlab.com/spydotech-group/shared-entity/models"
	"messaging-app/internal/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type AuthController struct {
	authService *services.AuthService
	cfg         *config.Config
}

func NewAuthController(authService *services.AuthService, cfg *config.Config) *AuthController {
	return &AuthController{
		authService: authService,
		cfg:         cfg,
	}
}

func (c *AuthController) setRefreshCookie(ctx *gin.Context, token string) {
	if c.cfg == nil || c.cfg.RefreshCookieName == "" {
		return
	}

	maxAge := int(c.cfg.RefreshTokenTTL.Seconds())
	if token == "" {
		maxAge = -1
	}

	cookie := &http.Cookie{
		Name:     c.cfg.RefreshCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   c.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	if c.cfg.CookieDomain != "" {
		cookie.Domain = c.cfg.CookieDomain
	}

	http.SetCookie(ctx.Writer, cookie)
}

func (c *AuthController) Register(ctx *gin.Context) {
	var user models.User
	if err := ctx.ShouldBindJSON(&user); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response, err := c.authService.Register(ctx.Request.Context(), &user)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.setRefreshCookie(ctx, response.RefreshToken)
	response.RefreshToken = ""

	ctx.JSON(http.StatusCreated, response)
}

func (c *AuthController) Login(ctx *gin.Context) {
	var loginReq struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&loginReq); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response, err := c.authService.Login(ctx.Request.Context(), loginReq.Email, loginReq.Password)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.setRefreshCookie(ctx, response.RefreshToken)
	response.RefreshToken = ""

	ctx.JSON(http.StatusOK, response)
}

func (c *AuthController) Refresh(ctx *gin.Context) {
	refreshToken := ""
	if c.cfg != nil && c.cfg.RefreshCookieName != "" {
		if cookieToken, err := ctx.Cookie(c.cfg.RefreshCookieName); err == nil && cookieToken != "" {
			refreshToken = cookieToken
		}
	}

	if refreshToken == "" {
		var refreshReq models.RefreshRequest
		if err := ctx.ShouldBindJSON(&refreshReq); err != nil || refreshReq.RefreshToken == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "refresh token required"})
			return
		}
		refreshToken = refreshReq.RefreshToken
	}

	response, err := c.authService.RefreshToken(ctx.Request.Context(), refreshToken)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.setRefreshCookie(ctx, response.RefreshToken)
	response.RefreshToken = ""

	ctx.JSON(http.StatusOK, response)
}

func (c *AuthController) Logout(ctx *gin.Context) {
	userID := ctx.MustGet("userID").(string)
	authHeader := strings.TrimSpace(ctx.GetHeader("Authorization"))
	if authHeader == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "authorization header required"})
		return
	}

	tokenString := authHeader
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		tokenString = strings.TrimSpace(authHeader[7:])
	}

	if tokenString == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "authorization token required"})
		return
	}

	if err := c.authService.Logout(ctx.Request.Context(), userID, tokenString); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.setRefreshCookie(ctx, "")

	ctx.JSON(http.StatusOK, gin.H{"message": "Successfully logged out"})
}
