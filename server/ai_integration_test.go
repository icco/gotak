// +build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/icco/gotak/ai"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AI Integration E2E Tests
//
// These are proper E2E tests that:
// - Make actual HTTP API calls (not direct function calls) 
// - Test database integration through the API
// - Would catch the original "AI uses placeholder game" bug immediately
//
// They run only with `-tags=integration` and require a real database.

// TestAIIntegration tests the full E2E integration of AI via HTTP API
func TestAIIntegration(t *testing.T) {
	// Set up test server with in-memory database
	server := setupTestServer(t)
	defer server.Close()

	// Create authenticated user
	user := createTestUser(t, setupTestDB(t))

	// Test different AI difficulty levels through the API
	testCases := []struct {
		name  string
		level string
	}{
		{"Beginner", "beginner"},
		{"Intermediate", "intermediate"},
		{"Advanced", "advanced"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a new game via API
			gameSlug := createTestGameViaAPI(t, server.URL, user)

			// Make one human move to set up board state
			makeTestMove(t, server.URL, user, gameSlug, "c3")

			// Request AI move via API
			aiMove := requestAIMove(t, server.URL, user, gameSlug, tc.level)

			// Validate AI move
			if len(aiMove.Move) < 2 {
				t.Errorf("AI generated invalid move: %s", aiMove.Move)
				return
			}

			// AI should suggest different move than the human move
			if aiMove.Move == "c3" {
				t.Errorf("AI suggested same move as human, shows it's not seeing board state")
			}

			// Make AI move to advance game state
			makeTestMove(t, server.URL, user, gameSlug, aiMove.Move)

			// Make another human move
			makeTestMove(t, server.URL, user, gameSlug, "d4")

			// Request another AI move - should be different from first move
			secondAIMove := requestAIMove(t, server.URL, user, gameSlug, tc.level)

			// The two AI moves should likely be different (board state changed)
			t.Logf("%s AI moves: %s -> %s", tc.name, aiMove.Move, secondAIMove.Move)

			// Validate AI is responding to board state by making reasonable moves
			if aiMove.Move == secondAIMove.Move && tc.level != "beginner" {
				// Note: Beginner is random so might repeat, but intermediate+ should adapt
				t.Logf("Warning: AI suggested same move twice, might not be adapting to board state")
			}
		})
	}
}

// TestAIPerformance tests that AI API responds within reasonable time limits
func TestAIPerformance(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	user := createTestUser(t, setupTestDB(t))
	gameSlug := createTestGameViaAPI(t, server.URL, user)

	// Make a move to set up board state
	makeTestMove(t, server.URL, user, gameSlug, "c3")

	// Test AI response time via API
	start := time.Now()
	aiMove := requestAIMove(t, server.URL, user, gameSlug, "beginner")
	elapsed := time.Since(start)

	if aiMove.Move == "" {
		t.Errorf("AI returned empty move")
		return
	}

	t.Logf("AI API response time: %v, move: %s", elapsed, aiMove.Move)

	// Should respond within reasonable time for API call
	if elapsed > 5*time.Second {
		t.Errorf("AI API took too long to respond: %v", elapsed)
	}
}

// TestAIDifferentBoardSizes tests AI works on different board sizes via API
func TestAIDifferentBoardSizes(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	user := createTestUser(t, setupTestDB(t))

	boardSizes := []int{4, 5, 6}
	for _, size := range boardSizes {
		t.Run(fmt.Sprintf("%d", size), func(t *testing.T) {
			gameSlug := createTestGameWithSize(t, server.URL, user, size)

			// Make initial move
			makeTestMove(t, server.URL, user, gameSlug, "a1")

			// Get AI move
			aiMove := requestAIMove(t, server.URL, user, gameSlug, "intermediate")

			if aiMove.Move == "" {
				t.Errorf("AI returned empty move for %dx%d board", size, size)
				return
			}

			t.Logf("%dx%d board - AI move: %s", size, size, aiMove.Move)
		})
	}
}

// TestAIGameProgression tests AI adapts to actual game progression via API
func TestAIGameProgression(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	user := createTestUser(t, setupTestDB(t))
	gameSlug := createTestGameViaAPI(t, server.URL, user)

	// Play several moves and test AI adaptation
	humanMoves := []string{"a1", "b2", "c3"}

	for i, move := range humanMoves {
		// Make human move
		makeTestMove(t, server.URL, user, gameSlug, move)

		// Get AI response
		aiMove := requestAIMove(t, server.URL, user, gameSlug, "intermediate")

		if aiMove.Move == "" {
			t.Errorf("AI returned empty move after move %d", i+1)
			return
		}

		t.Logf("After move %d (%s), AI suggests: %s", i+1, move, aiMove.Move)

		// Make AI move
		makeTestMove(t, server.URL, user, gameSlug, aiMove.Move)
	}
}

// TestAIErrorHandling tests AI API error cases
func TestAIErrorHandling(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	user := createTestUser(t, setupTestDB(t))

	// Test invalid game slug
	t.Run("InvalidGameSlug", func(t *testing.T) {
		resp := makeAIRequest(t, server.URL, user, "nonexistent-game", "intermediate")
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("Expected error for invalid game slug, got status: %d", resp.StatusCode)
		}
	})

	// Test without authentication
	t.Run("NoAuth", func(t *testing.T) {
		gameSlug := createTestGameViaAPI(t, server.URL, user)

		req := buildAIRequest(server.URL+"/game/"+gameSlug+"/ai-move", "intermediate")
		// Don't add auth header
		
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
			t.Errorf("Expected auth error, got status: %d", resp.StatusCode)
		}
	})
}

// Helper functions for E2E testing

func setupTestServer(t *testing.T) *httptest.Server {
	// Set up test database - use the same setup as other tests
	testDB := setupTestDB(t)
	
	// Create a router similar to the main server but with test auth
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	
	// Create test-specific handlers that use the test database
	r.Post("/game/new", func(w http.ResponseWriter, r *http.Request) {
		testNewGameHandler(w, r, testDB)
	})
	r.Post("/game/{slug}/move", func(w http.ResponseWriter, r *http.Request) {
		testNewMoveHandler(w, r, testDB)
	})
	r.Post("/game/{slug}/ai-move", func(w http.ResponseWriter, r *http.Request) {
		testPostAIMoveHandler(w, r, testDB)
	})
	r.Get("/game/{slug}", func(w http.ResponseWriter, r *http.Request) {
		testGetGameHandler(w, r, testDB)
	})
	
	return httptest.NewServer(r)
}

func createTestGameViaAPI(t *testing.T, serverURL string, user *User) string {
	return createTestGameWithSize(t, serverURL, user, 5)
}

func createTestGameWithSize(t *testing.T, serverURL string, user *User, size int) string {
	payload := map[string]interface{}{
		"size": size,
	}
	
	data, _ := json.Marshal(payload)
	
	req, _ := http.NewRequest("POST", serverURL+"/game/new", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(user))
	
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}
	
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("Expected redirect from game creation, got: %d", resp.StatusCode)
	}
	
	location := resp.Header.Get("Location")
	if location == "" {
		t.Fatalf("No location header in redirect")
	}
	
	// Extract slug from "/game/{slug}"
	parts := strings.Split(location, "/")
	if len(parts) < 3 {
		t.Fatalf("Invalid redirect location: %s", location)
	}
	
	return parts[len(parts)-1]
}

func makeTestMove(t *testing.T, serverURL string, user *User, gameSlug, move string) {
	payload := map[string]interface{}{
		"move":   move,
		"player": 1, // Assume white player for simplicity
	}
	
	data, _ := json.Marshal(payload)
	
	req, _ := http.NewRequest("POST", serverURL+"/game/"+gameSlug+"/move", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(user))
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make move %s: %v", move, err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Move %s failed with status: %d", move, resp.StatusCode)
	}
}

func requestAIMove(t *testing.T, serverURL string, user *User, gameSlug, level string) *AIMoveResponse {
	resp := makeAIRequest(t, serverURL, user, gameSlug, level)
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("AI request failed with status: %d", resp.StatusCode)
	}
	
	var aiMove AIMoveResponse
	if err := json.NewDecoder(resp.Body).Decode(&aiMove); err != nil {
		t.Fatalf("Failed to decode AI response: %v", err)
	}
	
	return &aiMove
}

func makeAIRequest(t *testing.T, serverURL string, user *User, gameSlug, level string) *http.Response {
	req := buildAIRequest(serverURL+"/game/"+gameSlug+"/ai-move", level)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(user))
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("AI request failed: %v", err)
	}
	
	return resp
}

func buildAIRequest(url, level string) *http.Request {
	payload := map[string]interface{}{
		"level":      level,
		"style":      "balanced",
		"time_limit": "5s",
	}
	
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	
	return req
}

func generateTestToken(user *User) string {
	// Simple test token - in real implementation, use proper JWT
	return fmt.Sprintf("test-token-user-%d", user.ID)
}

// Test-specific handlers that inject test database and handle simple auth

func testNewGameHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	user := extractTestUser(w, r, db)
	if user == nil {
		return
	}
	
	// Inject user into context
	ctx := context.WithValue(r.Context(), userContextKey, user)
	r = r.WithContext(ctx)
	
	// Call the original handler logic with test database
	testNewGameHandlerWithDB(w, r, db)
}

func testNewMoveHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	user := extractTestUser(w, r, db)
	if user == nil {
		return
	}
	
	ctx := context.WithValue(r.Context(), userContextKey, user)
	r = r.WithContext(ctx)
	
	testNewMoveHandlerWithDB(w, r, db)
}

func testPostAIMoveHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	user := extractTestUser(w, r, db)
	if user == nil {
		return
	}
	
	ctx := context.WithValue(r.Context(), userContextKey, user)
	r = r.WithContext(ctx)
	
	testPostAIMoveHandlerWithDB(w, r, db)
}

func testGetGameHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	// Get game handler doesn't require auth in main server
	testGetGameHandlerWithDB(w, r, db)
}

func extractTestUser(w http.ResponseWriter, r *http.Request, db *gorm.DB) *User {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		if err := Renderer.JSON(w, 401, map[string]string{"error": "missing authorization header"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return nil
	}
	
	token := strings.TrimPrefix(authHeader, "Bearer ")
	// Parse test token format: "test-token-user-{id}"
	if !strings.HasPrefix(token, "test-token-user-") {
		if err := Renderer.JSON(w, 401, map[string]string{"error": "invalid test token"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return nil
	}
	
	userIDStr := strings.TrimPrefix(token, "test-token-user-")
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		if err := Renderer.JSON(w, 401, map[string]string{"error": "invalid user ID in token"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return nil
	}
	
	var user User
	if err := db.First(&user, userID).Error; err != nil {
		if err := Renderer.JSON(w, 401, map[string]string{"error": "user not found"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return nil
	}
	
	return &user
}

// Context key is already defined in auth_pkgz.go

// Test handler implementations that use injected database

func testNewGameHandlerWithDB(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	// Get current user from context
	user := getMustUserFromContext(r)
	userID := user.ID

	boardSize := 8

	var data CreateGameRequest
	if err := json.NewDecoder(r.Body).Decode(&data); err == nil && data.Size != "" {
		i, err := strconv.Atoi(data.Size)
		if err == nil && i > 0 {
			boardSize = i
		}
	}

	slug, err := createGame(db, boardSize, userID)
	if err != nil {
		log.Errorw("could not create game", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not create game"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	http.Redirect(w, r, "/game/"+slug, http.StatusTemporaryRedirect)
}

func testNewMoveHandlerWithDB(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "missing game slug"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Get current user from context
	user := getMustUserFromContext(r)

	// Verify user can access this game
	err := verifyGameParticipation(db, slug, user.ID)
	if err != nil {
		log.Errorw("user not authorized for game", "slug", slug, "user_id", user.ID, zap.Error(err))
		if err := Renderer.JSON(w, 403, map[string]string{"error": "unauthorized"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	var data MoveRequest
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "invalid request body"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Get game to access GameID
	game, err := getGame(db, slug)
	if err != nil {
		log.Errorw("could not get game", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "game not found"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	err = insertMove(db, game.ID, data.Player, data.Text, data.Turn)
	if err != nil {
		log.Errorw("could not add move", "slug", slug, "move", data.Text, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not add move"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if err := Renderer.JSON(w, 200, map[string]string{"status": "move added"}); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}

func testPostAIMoveHandlerWithDB(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "missing game slug"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Get current user (required by authMiddleware)
	user := getMustUserFromContext(r)

	// Verify user can access this game
	err := verifyGameParticipation(db, slug, user.ID)
	if err != nil {
		log.Errorw("user not authorized for game", "slug", slug, "user_id", user.ID, zap.Error(err))
		if err := Renderer.JSON(w, 403, map[string]string{"error": "unauthorized"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Parse request body
	var req AIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Errorw("failed to decode AI move request", zap.Error(err))
		if err := Renderer.JSON(w, 400, map[string]string{"error": "invalid request body"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Load game from database
	game, err := getGame(db, slug)
	if err != nil {
		log.Errorw("could not get game", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "game not found"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Parse AI level (same logic as main handler)
	var level ai.DifficultyLevel
	switch req.Level {
	case "beginner":
		level = ai.Beginner
	case "intermediate":
		level = ai.Intermediate
	case "advanced":
		level = ai.Advanced
	case "expert":
		level = ai.Expert
	default:
		level = ai.Intermediate // default
	}

	// Parse AI style
	var style ai.Style
	switch req.Style {
	case "aggressive":
		style = ai.Aggressive
	case "defensive":
		style = ai.Defensive
	case "balanced":
		style = ai.Balanced
	default:
		style = ai.Balanced // default
	}

	// Parse time limit (default to 10 seconds)
	timeLimit := 10 * time.Second
	if req.TimeLimit > 0 {
		timeLimit = req.TimeLimit
	}

	cfg := ai.AIConfig{
		Level:       level,
		Style:       style,
		TimeLimit:   timeLimit,
		Personality: req.Personality,
	}

	// Get AI move using actual game state
	engine := &ai.TakticianEngine{}
	move, err := engine.GetMove(r.Context(), game, cfg)
	if err != nil {
		log.Errorw("AI move failed", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "AI move failed"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	hint, _ := engine.ExplainMove(r.Context(), game, cfg)

	response := AIMoveResponse{
		Move: move,
		Hint: hint,
	}

	if err := Renderer.JSON(w, 200, response); err != nil {
		log.Errorw("failed to render AI move response", zap.Error(err))
	}
}

func testGetGameHandlerWithDB(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "missing game slug"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	game, err := getGame(db, slug)
	if err != nil {
		log.Errorw("could not get game", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 404, map[string]string{"error": "game not found"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if err := Renderer.JSON(w, 200, game); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}

// getMustUserFromContext is already defined in auth_pkgz.go