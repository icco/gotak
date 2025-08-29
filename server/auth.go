package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret = []byte("replace-with-strong-secret")

// RegisterRequest is the request body for user registration
// @Description User registration request
// @Param username body string true "Username"
// @Param email body string true "Email"
// @Param password body string true "Password"
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest is the request body for user login
// @Description User login request
// @Param email body string true "Email"
// @Param password body string true "Password"
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// JWTClaims represents the JWT claims
// @Description JWT claims for authentication
type JWTClaims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	jwt.RegisteredClaims
}

// registerHandler handles user registration
func registerHandler(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Renderer.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.Username == "" || req.Email == "" || req.Password == "" {
		Renderer.JSON(w, http.StatusBadRequest, map[string]string{"error": "all fields required"})
		return
	}
	// Basic email normalization
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		Renderer.JSON(w, http.StatusInternalServerError, map[string]string{"error": "could not hash password"})
		return
	}
	user := User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db, err := getDB()
	if err != nil {
		Renderer.JSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	if err := db.Create(&user).Error; err != nil {
		Renderer.JSON(w, http.StatusBadRequest, map[string]string{"error": "user already exists or db error"})
		return
	}
	Renderer.JSON(w, http.StatusCreated, map[string]string{"message": "user registered"})
}

// loginHandler handles user login
func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Renderer.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	db, err := getDB()
	if err != nil {
		Renderer.JSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	var user User
	if err := db.Where("email = ?", strings.ToLower(strings.TrimSpace(req.Email))).First(&user).Error; err != nil {
		Renderer.JSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		Renderer.JSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	claims := JWTClaims{
		UserID:   user.ID,
		Username: user.Username,
		Email:    user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtSecret)
	if err != nil {
		Renderer.JSON(w, http.StatusInternalServerError, map[string]string{"error": "could not sign token"})
		return
	}
	Renderer.JSON(w, http.StatusOK, map[string]string{"token": signed})
}
