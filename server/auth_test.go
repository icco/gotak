package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"golang.org/x/crypto/bcrypt"
)

func TestUserModel(t *testing.T) {
	db := setupTestDB(t)

	// Test creating a user
	user := User{
		Provider:   "local",
		ProviderID: "test-123",
		Email:      "test@example.com",
		Name:       "Test User",
	}

	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Verify user was created
	var retrieved User
	if err := db.Where("email = ?", "test@example.com").First(&retrieved).Error; err != nil {
		t.Fatalf("Failed to retrieve user: %v", err)
	}

	if retrieved.Email != user.Email {
		t.Errorf("Expected email %s, got %s", user.Email, retrieved.Email)
	}

	if retrieved.Provider != user.Provider {
		t.Errorf("Expected provider %s, got %s", user.Provider, retrieved.Provider)
	}
}

func TestPasswordHashing(t *testing.T) {
	password := "testpassword123"
	
	// Test password hashing
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	// Test password verification
	if err := bcrypt.CompareHashAndPassword(hashedPassword, []byte(password)); err != nil {
		t.Error("Password verification failed")
	}

	// Test wrong password
	if err := bcrypt.CompareHashAndPassword(hashedPassword, []byte("wrongpassword")); err == nil {
		t.Error("Wrong password should not verify")
	}
}

func TestUserGameAssociation(t *testing.T) {
	db := setupTestDB(t)

	// Create a test user
	user := createTestUser(t, db)

	// Create a game linked to the user
	slug, err := createGame(db, 6, &user.ID)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Verify the game is linked to the user
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		t.Fatalf("Failed to retrieve game: %v", err)
	}

	if game.UserID == nil {
		t.Error("Game should be linked to a user")
	}

	if *game.UserID != user.ID {
		t.Errorf("Game should be linked to user %d, got %d", user.ID, *game.UserID)
	}

	// Test loading game with user association
	if err := db.Preload("User").First(&game, game.ID).Error; err != nil {
		t.Fatalf("Failed to load game with user: %v", err)
	}

	if game.User == nil {
		t.Error("User association not loaded")
	}

	if game.User.Email != user.Email {
		t.Errorf("Expected user email %s, got %s", user.Email, game.User.Email)
	}
}

func TestAuthenticationRequirement(t *testing.T) {
	db := setupTestDB(t)

	// Test that createGame requires a user ID
	_, err := createGame(db, 6, nil)
	if err == nil {
		t.Error("Expected error when creating game without user")
	}

	if err.Error() != "user authentication required" {
		t.Errorf("Expected specific error message, got: %s", err.Error())
	}
}

func TestGenerateProviderID(t *testing.T) {
	id1 := generateProviderID()
	id2 := generateProviderID()

	if id1 == "" {
		t.Error("Provider ID should not be empty")
	}

	if id2 == "" {
		t.Error("Provider ID should not be empty")
	}

	if id1 == id2 {
		t.Error("Provider IDs should be unique")
	}

	if len(id1) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("Expected 32 character hex string, got %d chars", len(id1))
	}
}

func TestAuthMiddlewareWithoutUser(t *testing.T) {
	// Test middleware behavior when no user in context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := getUserFromContext(r)
		if user != nil {
			t.Error("Expected no user in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}