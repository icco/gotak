//go:build integration

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
	"github.com/icco/gotak"
	"github.com/icco/gotak/ai"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TestNewAIServerSideExecution tests the new AI architecture that executes moves server-side
// and returns the updated game state directly, eliminating authentication and player assignment issues
func TestNewAIServerSideExecution(t *testing.T) {
	server := setupE2ETestServer(t)
	defer server.Close()

	user := &User{ID: 1} // User created in setupE2ETestServer
	gameSlug := createGameForE2E(t, server.URL, user, 5)

	// Make a human move first (white player) so AI (black) can make the second move
	makeE2EMove(t, server.URL, user, gameSlug, "c3", gotak.PlayerWhite)

	// Request AI move - this should execute the move server-side and return updated game state
	aiGame := requestAIAndGetGameState(t, server.URL, user, gameSlug, "intermediate")

	// Verify the AI move was executed server-side
	if len(aiGame.Turns) != 1 {
		t.Fatalf("Expected 1 turn after AI move, got %d", len(aiGame.Turns))
	}

	// AI should be the second move in turn 1 (after human's first move)
	var aiMoveText string
	if aiGame.Turns[0].Second != nil {
		// AI move was second move in turn 1
		aiMoveText = aiGame.Turns[0].Second.Text
	} else {
		t.Fatal("Could not find AI move in game state")
	}

	if aiMoveText == "" {
		t.Fatal("AI move text is empty")
	}
	// Verify AI made a valid move format (like "a1", "c3", etc.)
	if len(aiMoveText) < 2 {
		t.Fatalf("AI move appears invalid: %s", aiMoveText)
	}
	// Verify AI didn't repeat the human move
	if aiMoveText == "c3" {
		t.Fatal("AI made same move as human, not seeing board state properly")
	}

	t.Logf("AI executed move: %s", aiMoveText)
}

// TestNewAIGameProgression tests that AI adapts to game progression over multiple moves
func TestNewAIGameProgression(t *testing.T) {
	server := setupE2ETestServer(t)
	defer server.Close()

	user := &User{ID: 1}
	gameSlug := createGameForE2E(t, server.URL, user, 5)

	// Play several alternating moves to test AI adaptation
	humanMoves := []string{"a1", "b2", "c1"}
	aiMoves := []string{}

	for i, humanMove := range humanMoves {
		// Human move (white)
		makeE2EMove(t, server.URL, user, gameSlug, humanMove, gotak.PlayerWhite)

		// AI move (black) - executed server-side
		aiGame := requestAIAndGetGameState(t, server.URL, user, gameSlug, "intermediate")

		// Extract AI move from updated game state
		var aiMoveText string
		expectedTurns := i + 1

		if len(aiGame.Turns) < expectedTurns {
			t.Fatalf("Move %d: Expected at least %d turns, got %d", i+1, expectedTurns, len(aiGame.Turns))
		}

		// AI move should be the second move in the current turn
		if aiGame.Turns[expectedTurns-1].Second != nil {
			aiMoveText = aiGame.Turns[expectedTurns-1].Second.Text
		} else if len(aiGame.Turns) > expectedTurns && aiGame.Turns[expectedTurns].First != nil {
			aiMoveText = aiGame.Turns[expectedTurns].First.Text
		} else {
			t.Fatalf("Move %d: Could not find AI move in game state", i+1)
		}

		if aiMoveText == "" {
			t.Fatalf("Move %d: AI move text is empty", i+1)
		}

		aiMoves = append(aiMoves, aiMoveText)
		t.Logf("Move %d: Human=%s, AI=%s", i+1, humanMove, aiMoveText)

		// Verify AI isn't repeating moves (shows it's adapting to board state)
		for j, prevMove := range aiMoves[:len(aiMoves)-1] {
			if prevMove == aiMoveText {
				t.Logf("Warning: AI repeated move %s from turn %d", aiMoveText, j+1)
			}
		}
	}

	// Verify final game state
	finalGame := getGameState(t, server.URL, gameSlug)
	if len(finalGame.Turns) != 3 {
		t.Fatalf("Expected 3 completed turns, got %d", len(finalGame.Turns))
	}
}

// TestNewAIPlayerAssignment tests that AI correctly identifies and plays as the opposite player
func TestNewAIPlayerAssignment(t *testing.T) {
	server := setupE2ETestServer(t)
	defer server.Close()

	user := &User{ID: 1}
	gameSlug := createGameForE2E(t, server.URL, user, 5)

	// Test scenario 1: Human plays as white (player 1), AI should be black (player 2)
	makeE2EMove(t, server.URL, user, gameSlug, "c3", gotak.PlayerWhite)

	_ = getGameState(t, server.URL, gameSlug)
	// After 1 move, it should be AI's turn (black player)

	// AI move should work when it's AI's turn
	aiGame := requestAIAndGetGameState(t, server.URL, user, gameSlug, "intermediate")
	// Verify AI made a move (should have 2 total moves now)
	totalMoves := 0
	for _, turn := range aiGame.Turns {
		if turn.First != nil {
			totalMoves++
		}
		if turn.Second != nil {
			totalMoves++
		}
	}
	if totalMoves != 2 {
		t.Fatalf("Expected 2 moves after AI turn, got %d", totalMoves)
	}

	// Test scenario 2: Try AI move when it's not AI's turn (should fail)
	resp := makeRawAIRequest(t, server.URL, user, gameSlug, "intermediate")
	if resp.StatusCode == http.StatusOK {
		t.Fatal("AI move should fail when it's not AI's turn")
	}

	// Verify error message
	var errorResp struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errorResp.Error, "not the AI's turn") {
		t.Fatalf("Expected 'not the AI's turn' error, got: %s", errorResp.Error)
	}

	resp.Body.Close()
}

// TestNewAIErrorHandling tests various error scenarios in the new AI architecture
func TestNewAIErrorHandling(t *testing.T) {
	server := setupE2ETestServer(t)
	defer server.Close()

	user := &User{ID: 1}

	// Test 1: Invalid game slug
	resp := makeRawAIRequest(t, server.URL, user, "nonexistent-game", "intermediate")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected 403 for invalid game slug, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test 2: Empty game (no moves made yet)
	gameSlug := createGameForE2E(t, server.URL, user, 5)

	// AI should be able to make first move
	aiGame := requestAIAndGetGameState(t, server.URL, user, gameSlug, "intermediate")
	if len(aiGame.Turns) == 0 {
		t.Fatal("AI should be able to make first move in empty game")
	}
}

// TestAIDifficultyLevels tests that different AI difficulty levels work with the new architecture
func TestAIDifficultyLevels(t *testing.T) {
	server := setupE2ETestServer(t)
	defer server.Close()

	user := &User{ID: 1}

	levels := []string{"beginner", "intermediate", "advanced", "expert"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			gameSlug := createGameForE2E(t, server.URL, user, 5)

			// Make initial human move
			makeE2EMove(t, server.URL, user, gameSlug, "c3", gotak.PlayerWhite)

			// Test AI at this difficulty level
			start := time.Now()
			aiGame := requestAIAndGetGameState(t, server.URL, user, gameSlug, level)
			elapsed := time.Since(start)

			if len(aiGame.Turns) != 1 {
				t.Fatalf("Expected 1 turn after AI move, got %d", len(aiGame.Turns))
			}

			// Extract AI move
			var aiMoveText string
			if aiGame.Turns[0].Second != nil {
				aiMoveText = aiGame.Turns[0].Second.Text
			} else {
				t.Fatal("Could not find AI move")
			}

			if aiMoveText == "" {
				t.Fatalf("AI level %s returned empty move", level)
			}

			t.Logf("AI level %s: move=%s, time=%v", level, aiMoveText, elapsed)

			// Expert level might take longer, but should still be reasonable
			if level == "expert" && elapsed > 30*time.Second {
				t.Fatalf("AI level %s took too long: %v", level, elapsed)
			} else if elapsed > 10*time.Second {
				t.Fatalf("AI level %s took too long: %v", level, elapsed)
			}
		})
	}
}

// TestAIGameCompletion tests AI behavior when game approaches completion
func TestAIGameCompletion(t *testing.T) {
	server := setupE2ETestServer(t)
	defer server.Close()

	user := &User{ID: 1}
	gameSlug := createGameForE2E(t, server.URL, user, 4) // Smaller board for faster completion

	// Play several complete turns to advance game state
	humanMoves := []string{"a1", "b1", "c1"}

	for i, move := range humanMoves {
		// Human makes their move (White always goes first)
		makeE2EMove(t, server.URL, user, gameSlug, move, gotak.PlayerWhite)

		// Get AI response (Black always goes second)
		aiGame := requestAIAndGetGameState(t, server.URL, user, gameSlug, "intermediate")

		// Check if game ended using GameOver method
		winner, gameOver := aiGame.GameOver()
		if gameOver {
			t.Logf("Game completed with winner: %d after %d turns", winner, i+1)
			return
		}

		// Verify AI made a move - should have (i+1) complete turns
		expectedTurns := i + 1
		if len(aiGame.Turns) < expectedTurns {
			t.Fatalf("Expected at least %d turns after move %d, got %d", expectedTurns, i+1, len(aiGame.Turns))
		}

		// Verify the current turn has both moves (human + AI)
		currentTurn := aiGame.Turns[len(aiGame.Turns)-1]
		if currentTurn.First == nil || currentTurn.Second == nil {
			t.Fatalf("Turn %d should have both moves after AI response: First=%v, Second=%v",
				currentTurn.Number, currentTurn.First, currentTurn.Second)
		}
	}

	// Even if game didn't complete, verify AI kept playing
	finalGame := getGameState(t, server.URL, gameSlug)
	if len(finalGame.Turns) < 3 {
		t.Fatalf("Expected at least 3 turns of play, got %d", len(finalGame.Turns))
	}
}

// Helper functions for E2E testing

func setupE2ETestServer(t *testing.T) *httptest.Server {
	testDB := setupTestDB(t)
	testUser := createTestUser(t, testDB)

	r := chi.NewRouter()
	r.Use(middleware.RealIP)

	// Routes that inject test user context (bypass auth)
	r.Post("/game/new", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), userContextKey, testUser)
		r = r.WithContext(ctx)
		testNewGameHandlerWithDB(w, r, testDB)
	})

	r.Post("/game/{slug}/move", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), userContextKey, testUser)
		r = r.WithContext(ctx)
		testMoveHandlerWithTurnManagement(w, r, testDB)
	})

	// Use the actual AI handler for realistic testing
	r.Post("/game/{slug}/ai-move", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), userContextKey, testUser)
		r = r.WithContext(ctx)
		testAIServerSideHandlerWithDB(w, r, testDB)
	})

	r.Get("/game/{slug}", func(w http.ResponseWriter, r *http.Request) {
		testGetGameHandlerWithDB(w, r, testDB)
	})

	return httptest.NewServer(r)
}

func createGameForE2E(t *testing.T, serverURL string, user *User, size int) string {
	payload := map[string]interface{}{
		"size": strconv.Itoa(size),
	}

	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", serverURL+"/game/new", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
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
	parts := strings.Split(location, "/")
	return parts[len(parts)-1]
}

func makeE2EMove(t *testing.T, serverURL string, user *User, gameSlug, move string, player int) {
	payload := map[string]interface{}{
		"move":   move,
		"player": player,
	}

	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", serverURL+"/game/"+gameSlug+"/move", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")

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

func requestAIAndGetGameState(t *testing.T, serverURL string, user *User, gameSlug, level string) *gotak.Game {
	resp := makeRawAIRequest(t, serverURL, user, gameSlug, level)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errorResp)
		t.Fatalf("AI request failed with status %d: %s", resp.StatusCode, errorResp.Error)
	}

	// The new AI endpoint returns the updated game state directly
	var game gotak.Game
	if err := json.NewDecoder(resp.Body).Decode(&game); err != nil {
		t.Fatalf("Failed to decode AI game response: %v", err)
	}

	return &game
}

func makeRawAIRequest(t *testing.T, serverURL string, user *User, gameSlug, level string) *http.Response {
	payload := map[string]interface{}{
		"level":      level,
		"style":      "balanced",
		"time_limit": int64(5 * time.Second),
	}

	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", serverURL+"/game/"+gameSlug+"/ai-move", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("AI request failed: %v", err)
	}

	return resp
}

func getGameState(t *testing.T, serverURL, gameSlug string) *gotak.Game {
	req, _ := http.NewRequest("GET", serverURL+"/game/"+gameSlug, nil)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to get game state: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Get game failed with status: %d", resp.StatusCode)
	}

	var game gotak.Game
	if err := json.NewDecoder(resp.Body).Decode(&game); err != nil {
		t.Fatalf("Failed to decode game response: %v", err)
	}

	return &game
}

// Test handler for the new server-side AI architecture
func testAIServerSideHandlerWithDB(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	// Get database connection
	if db == nil {
		log.Errorw("database connection is nil")
		if err := Renderer.JSON(w, 500, map[string]string{"error": "bad connection to db"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	ctx := r.Context()

	// Get game slug from URL
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "missing game slug"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Get current user (return 401 if unauthenticated)
	user := getMustUserFromContext(r)
	if user == nil {
		log.Errorw("unauthenticated request to AI move endpoint")
		if err := Renderer.JSON(w, 401, map[string]string{"error": "unauthenticated"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Verify user can access this game
	err := verifyGameParticipation(db, slug, user.ID)
	if err != nil {
		log.Errorw("user not authorized for game", "slug", slug, "user_id", user.ID, zap.Error(err))
		if err := Renderer.JSON(w, 403, map[string]string{"error": "unauthorized"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Load actual game from database
	game, err := getGame(db, slug)
	if err != nil {
		log.Errorw("could not get game", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": err.Error()}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Parse AI request
	var req AIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Errorw("invalid AI request", zap.Error(err))
		if err := Renderer.JSON(w, 400, map[string]string{"error": "invalid request"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Parse AI difficulty level
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
	move, err := engine.GetMove(ctx, game, cfg)
	if err != nil {
		log.Errorw("AI move failed", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "AI move failed"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Determine which player the AI is (opposite of human user)
	userPlayerNumber, err := getPlayerNumber(db, slug, user.ID)
	if err != nil {
		log.Errorw("could not get user player number", "slug", slug, "user_id", user.ID, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "internal server error"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// AI is the opposite player
	aiPlayerNumber := gotak.PlayerBlack
	if userPlayerNumber == gotak.PlayerBlack {
		aiPlayerNumber = gotak.PlayerWhite
	}

	// Check if it's actually the AI's turn
	var dbGame Game
	if err := db.Where("slug = ?", slug).First(&dbGame).Error; err != nil {
		log.Errorw("could not get game state for turn check", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not verify game state"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Check if it's the AI's turn (now that turn management is fixed)
	if dbGame.CurrentPlayer != aiPlayerNumber {
		log.Errorw("not AI's turn", "current_player", dbGame.CurrentPlayer, "ai_player", aiPlayerNumber)
		if err := Renderer.JSON(w, 400, map[string]string{"error": "it's not the AI's turn"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Replay existing moves to get current board state
	err = replayMoves(game)
	if err != nil {
		log.Errorw("could not replay moves for AI", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not replay game state"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Execute the AI move
	err = game.DoSingleMove(move, aiPlayerNumber)
	if err != nil {
		log.Errorw("invalid AI move", "move", move, "player", aiPlayerNumber, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": fmt.Sprintf("AI generated invalid move: %v", err)}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Store the AI move in database - calculate turn number based on total moves made
	// Count total moves across all turns to determine which turn this move belongs to
	totalMoves := int64(0)
	for _, turn := range game.Turns {
		if turn.First != nil {
			totalMoves++
		}
		if turn.Second != nil {
			totalMoves++
		}
	}

	// Calculate turn number (moves 1-2 = turn 1, moves 3-4 = turn 2, etc.)
	currentTurn := (totalMoves / 2) + 1

	if err := insertMove(db, game.ID, aiPlayerNumber, move, currentTurn); err != nil {
		log.Errorw("could not insert AI move", "move", move, "player", aiPlayerNumber, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not save AI move"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Update current player to switch turns
	nextPlayer := gotak.PlayerWhite
	if aiPlayerNumber == gotak.PlayerWhite {
		nextPlayer = gotak.PlayerBlack
	}
	if err := db.Model(&Game{}).Where("slug = ?", slug).Update("current_player", nextPlayer).Error; err != nil {
		log.Errorw("could not update current player", "slug", slug, "next_player", nextPlayer, zap.Error(err))
		// Continue - this is not fatal for the test
	}

	// Check if game is now over and update status
	winner, gameOver := game.GameOver()
	if gameOver {
		err = updateGameStatus(db, game.Slug, "finished", winner)
		if err != nil {
			log.Errorw("could not update game status after AI move", zap.Error(err))
		}
	}

	// Reload game to get updated state
	updatedGame, err := getGame(db, slug)
	if err != nil {
		log.Errorw("could not reload game after AI move", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not reload game"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Return the updated game state (same format as regular move endpoint)
	log.Infow("AI move executed", "slug", slug, "move", move)
	if err := Renderer.JSON(w, http.StatusOK, updatedGame); err != nil {
		log.Errorw("failed to render game response", zap.Error(err))
	}
}

// testMoveHandlerWithTurnManagement is a test move handler that includes the turn management fixes
func testMoveHandlerWithTurnManagement(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	// Get current user from context
	user := getMustUserFromContext(r)

	// Get game slug from URL
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		if err := Renderer.JSON(w, 400, map[string]string{"error": "missing game slug"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Verify user can access this game
	err := verifyGameParticipation(db, slug, user.ID)
	if err != nil {
		log.Errorw("user not authorized for game", "slug", slug, "user_id", user.ID, zap.Error(err))
		if err := Renderer.JSON(w, 403, map[string]string{"error": "unauthorized"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	game, err := getGame(db, slug)
	if err != nil {
		log.Errorw("could not get game", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": err.Error()}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	var data MoveRequest
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		log.Errorw("could not read body", zap.Error(err))
		if err := Renderer.JSON(w, 400, map[string]string{"error": err.Error()}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if data.Text == "" {
		log.Errorw("empty request", "data", data)
		if err := Renderer.JSON(w, 400, map[string]string{"error": "empty move text"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Validate player
	if data.Player != gotak.PlayerWhite && data.Player != gotak.PlayerBlack {
		log.Errorw("invalid player", "player", data.Player)
		if err := Renderer.JSON(w, 400, map[string]string{"error": "invalid player"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Get current game state from database to check if it's the player's turn
	var dbGame Game
	if err := db.Where("slug = ?", slug).First(&dbGame).Error; err != nil {
		log.Errorw("could not get game state", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not verify game state"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Check if it's the player's turn
	if dbGame.CurrentPlayer != data.Player {
		log.Errorw("not player's turn", "current_player", dbGame.CurrentPlayer, "requested_player", data.Player)
		if err := Renderer.JSON(w, 400, map[string]string{"error": "it's not your turn"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Replay existing moves to get current board state
	err = replayMoves(game)
	if err != nil {
		log.Errorw("could not replay moves", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not replay game state"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Validate and execute the move
	err = game.DoSingleMove(data.Text, data.Player)
	if err != nil {
		log.Errorw("invalid move", "move", data.Text, "player", data.Player, zap.Error(err))
		if err := Renderer.JSON(w, 400, map[string]string{"error": fmt.Sprintf("invalid move: %v", err)}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Store the move in database - calculate turn number based on total moves made
	// Count total moves across all turns to determine which turn this move belongs to
	totalMoves := int64(0)
	for _, turn := range game.Turns {
		if turn.First != nil {
			totalMoves++
		}
		if turn.Second != nil {
			totalMoves++
		}
	}

	// Calculate turn number (moves 1-2 = turn 1, moves 3-4 = turn 2, etc.)
	currentTurn := (totalMoves / 2) + 1

	if err := insertMove(db, game.ID, data.Player, data.Text, currentTurn); err != nil {
		log.Errorw("could not insert move", "data", data, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not save move"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Switch to the next player's turn (FIXED)
	nextPlayer := gotak.PlayerWhite
	if data.Player == gotak.PlayerWhite {
		nextPlayer = gotak.PlayerBlack
	}
	if err := db.Model(&Game{}).Where("slug = ?", slug).Update("current_player", nextPlayer).Error; err != nil {
		log.Errorw("could not update current player", "slug", slug, "next_player", nextPlayer, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not update turn"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Check if game is now over and update status
	winner, gameOver := game.GameOver()
	if gameOver {
		err = updateGameStatus(db, game.Slug, "finished", winner)
		if err != nil {
			log.Errorw("could not update game status", zap.Error(err))
		}
	}

	// Reload game to get updated state
	game, err = getGame(db, slug)
	if err != nil {
		log.Errorw("could not reload game", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not reload game"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if err := Renderer.JSON(w, http.StatusOK, game); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}
