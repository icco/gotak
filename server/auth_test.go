package main

import (
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/http/httptest"
	"testing"
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
	slug, err := createGame(db, 6, user.ID)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Verify the game is linked to the user
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		t.Fatalf("Failed to retrieve game: %v", err)
	}

	if game.WhitePlayerID == 0 {
		t.Error("Game should have a white player")
	}

	if game.WhitePlayerID != user.ID {
		t.Errorf("Game white player should be user %d, got %d", user.ID, game.WhitePlayerID)
	}

	// Test loading game with user associations
	if err := db.Preload("WhitePlayer").Preload("BlackPlayer").First(&game, game.ID).Error; err != nil {
		t.Fatalf("Failed to load game with user: %v", err)
	}

	if game.WhitePlayer == nil {
		t.Error("White player association not loaded")
	}

	if game.WhitePlayer.Email != user.Email {
		t.Errorf("Expected white player email %s, got %s", user.Email, game.WhitePlayer.Email)
	}

	// Black player should be nil for a new game
	if game.BlackPlayer != nil {
		t.Error("Black player should be nil for a new game")
	}
}

func TestAuthenticationRequirement(t *testing.T) {
	db := setupTestDB(t)

	// Test that createGame requires a user ID
	_, err := createGame(db, 6, 0)
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

func TestGameParticipationVerification(t *testing.T) {
	db := setupTestDB(t)

	// Create two users
	user1 := createTestUser(t, db)

	user2 := &User{
		Provider:   "local",
		ProviderID: "test-user-456",
		Email:      "user2@example.com",
		Name:       "Test User 2",
	}
	if err := db.Create(user2).Error; err != nil {
		t.Fatalf("Failed to create second test user: %v", err)
	}

	// Create a game owned by user1
	slug, err := createGame(db, 6, user1.ID)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Test that user1 can participate in their game (as white player)
	err = verifyGameParticipation(db, slug, user1.ID)
	if err != nil {
		t.Errorf("User1 should be able to participate in their game, got error: %v", err)
	}

	// Test that user2 cannot participate in user1's game (game is waiting for black player)
	err = verifyGameParticipation(db, slug, user2.ID)
	if err == nil {
		t.Error("User2 should not be able to participate in user1's game without joining")
	}

	// Test user2 joining the game as black player
	err = joinGame(db, slug, user2.ID)
	if err != nil {
		t.Errorf("User2 should be able to join the game, got error: %v", err)
	}

	// Now user2 should be able to participate
	err = verifyGameParticipation(db, slug, user2.ID)
	if err != nil {
		t.Errorf("User2 should be able to participate after joining, got error: %v", err)
	}

	// Test non-existent game
	err = verifyGameParticipation(db, "nonexistent", user1.ID)
	if err == nil {
		t.Error("Should get error for non-existent game")
	}
}

func TestMustUserFromContext(t *testing.T) {
	// Test getMustUserFromContext with nil user (should panic)
	req := httptest.NewRequest("GET", "/test", nil)

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when user is nil in protected route")
		}
	}()

	getMustUserFromContext(req)
}
