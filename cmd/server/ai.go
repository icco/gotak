package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/icco/gotak"
	"github.com/icco/gotak/ai"
	"go.uber.org/zap"
)

// AIRequest represents a request for an AI move
type AIRequest struct {
	Level       string        `json:"level"`
	Style       string        `json:"style"`
	TimeLimit   time.Duration `json:"time_limit"`
	Personality string        `json:"personality"`
}

// AIMoveResponse is the response for an AI move
type AIMoveResponse struct {
	Move string `json:"move"`
	Hint string `json:"hint,omitempty"`
}

// PostAIMoveHandler handles AI move requests
func PostAIMoveHandler(w http.ResponseWriter, r *http.Request) {
	// Get database connection
	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "bad connection to db"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	ctx := r.Context()

	// Get game slug from URL
	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))

	// Get current user (return 401 if unauthenticated)
	user := getUserFromContext(r)
	if user == nil {
		log.Errorw("unauthenticated request to AI move endpoint")
		if err := Renderer.JSON(w, 401, map[string]string{"error": "unauthenticated"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Verify user can access this game
	err = verifyGameParticipation(db, slug, user.ID)
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

	hint, _ := engine.ExplainMove(ctx, game, cfg)

	// Now execute the AI move in the game
	// First, determine which player the AI is (opposite of human user)
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

	// Store the AI move in database - determine which turn this move belongs to
	// Check if we need to complete an existing turn or start a new one
	var currentTurn int64 = 1 // Default to turn 1 if no turns exist
	
	if len(game.Turns) > 0 {
		lastTurn := game.Turns[len(game.Turns)-1]
		if lastTurn.First != nil && lastTurn.Second == nil {
			// Incomplete turn - AI move should complete this turn
			currentTurn = int64(len(game.Turns))
		} else {
			// Complete turn - AI move should start a new turn
			currentTurn = int64(len(game.Turns)) + 1
		}
	}

	if err := insertMove(db, game.ID, aiPlayerNumber, move, currentTurn); err != nil {
		log.Errorw("could not insert AI move", "move", move, "player", aiPlayerNumber, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not save AI move"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Reload the game state to get updated turns from database
	game, err = getGame(db, slug)
	if err != nil {
		log.Errorw("could not reload game after AI move", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not reload game state"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Update current player to switch turns
	// After AI move, check if current turn is complete based on updated game state
	var nextPlayer int
	if len(game.Turns) > 0 {
		lastTurn := game.Turns[len(game.Turns)-1]
		if lastTurn.First != nil && lastTurn.Second != nil {
			// Turn is complete, next player is always White (start of next turn)
			nextPlayer = gotak.PlayerWhite
		} else {
			// Turn is incomplete, switch to the other player
			if aiPlayerNumber == gotak.PlayerWhite {
				nextPlayer = gotak.PlayerBlack
			} else {
				nextPlayer = gotak.PlayerWhite
			}
		}
	} else {
		// No turns yet, start with White
		nextPlayer = gotak.PlayerWhite
	}
	
	if err := db.Model(&Game{}).Where("slug = ?", slug).Update("current_player", nextPlayer).Error; err != nil {
		log.Errorw("could not update current player after AI move", "slug", slug, "next_player", nextPlayer, zap.Error(err))
		// Continue - this is not fatal for AI move execution
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
	log.Infow("AI move executed", "slug", slug, "move", move, "hint", hint)
	if err := Renderer.JSON(w, http.StatusOK, updatedGame); err != nil {
		log.Errorw("failed to render game response", zap.Error(err))
	}
}
