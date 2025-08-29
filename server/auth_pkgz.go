package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	auth2 "github.com/go-pkgz/auth/v2"
	"github.com/go-pkgz/auth/v2/avatar"
	"github.com/go-pkgz/auth/v2/token"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func newAuthService() *auth2.Service {
	issuer := "gotak-app"
	secret := os.Getenv("AUTH_JWT_SECRET")
	if secret == "" {
		secret = "dev-secret-change-me" // fallback for dev
	}
	
	service := auth2.NewService(auth2.Opts{
		SecretReader:  token.SecretFunc(func(aud string) (string, error) { return secret, nil }),
		TokenDuration: 24 * time.Hour, // 1 day
		Issuer:        issuer,
		URL:           "https://gotak.app", // change for local/dev
		Validator:     nil, // no custom validation needed
		DisableXSRF:   true, // for API only
		AvatarStore:   avatar.NewNoOp(), // disable avatars support
	})
	
	// Add Google OAuth2 provider if credentials are available
	googleClientID := os.Getenv("GOOGLE_CLIENT_ID")
	googleClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if googleClientID != "" && googleClientSecret != "" {
		service.AddProvider("google", googleClientID, googleClientSecret)
	}
	
	return service
}

type RegisterRequest struct {
	Email    string `json:"email" example:"user@example.com"`
	Password string `json:"password" example:"secretpassword"`
	Name     string `json:"name" example:"John Doe"`
}

type LoginRequest struct {
	Email    string `json:"email" example:"user@example.com"`
	Password string `json:"password" example:"secretpassword"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

func AuthRoutes() http.Handler {
	r := chi.NewRouter()
	
	// Add rate limiting to auth endpoints
	r.Use(middleware.Throttle(5)) // 5 concurrent requests max
	
	// Mount go-pkgz auth handlers for social login
	auth := newAuthService()
	authHandler, _ := auth.Handlers() // avatarHandler not used here
	r.Mount("/", authHandler)
	
	// Add rate limiting specifically for registration and login
	r.Group(func(r chi.Router) {
		r.Use(middleware.RealIP)
		// Allow 10 requests per minute for auth operations
		r.Use(middleware.ThrottleBacklog(10, 60, 50))
		
		r.Post("/register", registerHandler)
		r.Post("/login", loginHandler)
	})
	
	// Profile endpoints with less restrictive rate limiting
	r.Get("/profile", profileHandler)
	r.Put("/profile", updateProfileHandler)
	r.Post("/logout", logoutHandler)
	
	// Password reset endpoints
	r.Post("/reset-password", resetPasswordHandler)
	r.Post("/confirm-reset", confirmResetHandler)
	
	return r
}

// @Summary Register a new user
// @Description Register with email and password
// @Tags auth
// @Accept json
// @Produce json
// @Param user body RegisterRequest true "User registration data"
// @Success 201 {object} AuthResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/register [post]
func registerHandler(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "invalid request body"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if req.Email == "" || req.Password == "" {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "email and password required"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if len(req.Password) < 8 {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "password must be at least 8 characters"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "database error"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Check if user already exists
	var existingUser User
	if err := db.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "user already exists"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Errorw("failed to hash password", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "password processing error"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Generate provider ID for local auth
	providerID := generateProviderID()

	user := User{
		Provider:     "local",
		ProviderID:   providerID,
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: string(hashedPassword),
	}

	if err := db.Create(&user).Error; err != nil {
		log.Errorw("failed to create user", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "failed to create user"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Generate JWT token
	token, err := generateJWT(user.ID)
	if err != nil {
		log.Errorw("failed to generate token", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "token generation error"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Hide password hash in response
	user.PasswordHash = ""

	if err := Renderer.JSON(w, 201, AuthResponse{Token: token, User: user}); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}

// @Summary Login user
// @Description Login with email and password
// @Tags auth
// @Accept json
// @Produce json
// @Param user body LoginRequest true "User login data"
// @Success 200 {object} AuthResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/login [post]
func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "invalid request body"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if req.Email == "" || req.Password == "" {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "email and password required"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "database error"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	var user User
	if err := db.Where("email = ? AND provider = ?", req.Email, "local").First(&user).Error; err != nil {
		if err := Renderer.JSON(w, 401, map[string]string{"error": "invalid credentials"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		if err := Renderer.JSON(w, 401, map[string]string{"error": "invalid credentials"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Generate JWT token
	token, err := generateJWT(user.ID)
	if err != nil {
		log.Errorw("failed to generate token", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "token generation error"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Hide password hash in response
	user.PasswordHash = ""

	if err := Renderer.JSON(w, 200, AuthResponse{Token: token, User: user}); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}

// @Summary Get user profile
// @Description Get current user profile information
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} User
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/profile [get]
func profileHandler(w http.ResponseWriter, r *http.Request) {
	user, err := getCurrentUser(r)
	if err != nil {
		if err := Renderer.JSON(w, 401, map[string]string{"error": "unauthorized"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Hide password hash
	user.PasswordHash = ""

	if err := Renderer.JSON(w, 200, user); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}

type UpdateProfileRequest struct {
	Name        string `json:"name,omitempty"`
	Preferences string `json:"preferences,omitempty"`
}

type ResetPasswordRequest struct {
	Email string `json:"email" example:"user@example.com"`
}

type ConfirmResetRequest struct {
	Token       string `json:"token" example:"reset-token-here"`
	NewPassword string `json:"new_password" example:"newsecretpassword"`
}

// @Summary Update user profile
// @Description Update current user profile information
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param profile body UpdateProfileRequest true "Profile update data"
// @Success 200 {object} User
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/profile [put]
func updateProfileHandler(w http.ResponseWriter, r *http.Request) {
	user, err := getCurrentUser(r)
	if err != nil {
		if err := Renderer.JSON(w, 401, map[string]string{"error": "unauthorized"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "invalid request body"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "database error"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Update fields if provided
	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Preferences != "" {
		updates["preferences"] = req.Preferences
	}

	if len(updates) > 0 {
		if err := db.Model(&user).Updates(updates).Error; err != nil {
			log.Errorw("failed to update user", zap.Error(err))
			if err := Renderer.JSON(w, 500, map[string]string{"error": "failed to update profile"}); err != nil {
				log.Errorw("failed to render JSON", zap.Error(err))
			}
			return
		}
	}

	// Reload user to get updated data
	if err := db.First(&user, user.ID).Error; err != nil {
		log.Errorw("failed to reload user", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "failed to reload profile"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Hide password hash
	user.PasswordHash = ""

	if err := Renderer.JSON(w, 200, user); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}

// @Summary Logout user
// @Description Logout current user (client should discard token)
// @Tags auth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Router /auth/logout [post]
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	// For JWT-based auth, logout is handled client-side by discarding the token
	// We could implement a token blacklist here if needed
	if err := Renderer.JSON(w, 200, map[string]string{"message": "logged out successfully"}); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}

func generateProviderID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", bytes)
}

func generateJWT(userID int64) (string, error) {
	auth := newAuthService()
	tokenService := auth.TokenService()
	claims := token.Claims{
		User: &token.User{
			ID:   fmt.Sprintf("%d", userID),
			Name: fmt.Sprintf("user-%d", userID),
		},
	}
	tokenString, err := tokenService.Token(claims)
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

func getCurrentUser(r *http.Request) (*User, error) {
	// Get token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, fmt.Errorf("missing or invalid authorization header")
	}
	
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	
	auth := newAuthService()
	tokenService := auth.TokenService()
	claims, err := tokenService.Parse(tokenString)
	if err != nil {
		return nil, err
	}
	
	userID := claims.User.ID
	
	db, err := getDB()
	if err != nil {
		return nil, err
	}
	
	var user User
	if err := db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	
	return &user, nil
}

// Auth middleware to protect routes
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := getCurrentUser(r)
		if err != nil {
			if err := Renderer.JSON(w, 401, map[string]string{"error": "authentication required"}); err != nil {
				log.Errorw("failed to render JSON", zap.Error(err))
			}
			return
		}

		// Add user to request context
		ctx := context.WithValue(r.Context(), "user", user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Helper to get user from request context
func getUserFromContext(r *http.Request) *User {
	if user, ok := r.Context().Value("user").(*User); ok {
		return user
	}
	return nil
}

// @Summary Request password reset
// @Description Send password reset token for email
// @Tags auth
// @Accept json
// @Produce json
// @Param request body ResetPasswordRequest true "Reset password request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/reset-password [post]
func resetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "invalid request body"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if req.Email == "" {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "email required"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "database error"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Check if user exists
	var user User
	if err := db.Where("email = ? AND provider = ?", req.Email, "local").First(&user).Error; err != nil {
		// Return success even if user doesn't exist (security best practice)
		if err := Renderer.JSON(w, 200, map[string]string{"message": "if email exists, reset instructions sent"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Generate reset token (in production, store this in database with expiration)
	resetToken := generateProviderID()
	
	// TODO: In production, implement email sending and store token with expiration
	// For now, just log the token for development
	log.Infow("Password reset token generated", "email", req.Email, "token", resetToken)

	if err := Renderer.JSON(w, 200, map[string]string{
		"message": "if email exists, reset instructions sent",
		"dev_token": resetToken, // Remove in production
	}); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}

// @Summary Confirm password reset
// @Description Reset password with token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body ConfirmResetRequest true "Confirm reset request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/confirm-reset [post]
func confirmResetHandler(w http.ResponseWriter, r *http.Request) {
	var req ConfirmResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "invalid request body"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if req.Token == "" || req.NewPassword == "" {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "token and new password required"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if len(req.NewPassword) < 8 {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "password must be at least 8 characters"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// TODO: In production, validate token from database and check expiration
	// For now, just return error since we don't have token storage
	if err := Renderer.JSON(w, 400, map[string]string{"error": "password reset not fully implemented - token storage needed"}); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}
