package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
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

	// Get current user (required by authMiddleware)
	user := getMustUserFromContext(r)

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

	resp := AIMoveResponse{
		Move: move,
		Hint: hint,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Errorw("failed to encode response", zap.Error(err))
	}
}
