package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/icco/gotak"
	"github.com/icco/gotak/ai"
	"github.com/icco/gutil/logging"
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
	ctx := r.Context()
	l := logging.FromContext(ctx)
	db, err := getDB()
	if err != nil {
		l.Errorw("could not get db", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "bad connection to db"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))

	user := getUserFromContext(r)
	if user == nil {
		l.Errorw("unauthenticated request to AI move endpoint")
		if err := Renderer.JSON(w, 401, map[string]string{"error": "unauthenticated"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	err = verifyGameParticipation(db, slug, user.ID)
	if err != nil {
		l.Errorw("user not authorized for game", "slug", slug, "user_id", user.ID, zap.Error(err))
		if err := Renderer.JSON(w, 403, map[string]string{"error": "unauthorized"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	game, err := getGame(db, slug)
	if err != nil {
		l.Errorw("could not get game", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": err.Error()}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	var req AIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		l.Errorw("invalid AI request", zap.Error(err))
		if err := Renderer.JSON(w, 400, map[string]string{"error": "invalid request"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
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

	engine := &ai.TakticianEngine{}
	move, err := engine.GetMove(ctx, game, cfg)
	if err != nil {
		l.Errorw("AI move failed", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "AI move failed"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	hint, _ := engine.ExplainMove(ctx, game, cfg)

	userPlayerNumber, err := getPlayerNumber(db, slug, user.ID)
	if err != nil {
		l.Errorw("could not get user player number", "slug", slug, "user_id", user.ID, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "internal server error"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	aiPlayerNumber := gotak.PlayerBlack
	if userPlayerNumber == gotak.PlayerBlack {
		aiPlayerNumber = gotak.PlayerWhite
	}

	var dbGame Game
	if err := db.Where("slug = ?", slug).First(&dbGame).Error; err != nil {
		l.Errorw("could not get game state for turn check", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not verify game state"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if dbGame.CurrentPlayer != aiPlayerNumber {
		l.Errorw("not AI's turn", "current_player", dbGame.CurrentPlayer, "ai_player", aiPlayerNumber)
		if err := Renderer.JSON(w, 400, map[string]string{"error": "it's not the AI's turn"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	err = replayMoves(game)
	if err != nil {
		l.Errorw("could not replay moves for AI", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not replay game state"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	err = game.DoSingleMove(move, aiPlayerNumber)
	if err != nil {
		l.Errorw("invalid AI move", "move", move, "player", aiPlayerNumber, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": fmt.Sprintf("AI generated invalid move: %v", err)}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	currentTurn := int64(len(game.Turns))
	if currentTurn == 0 {
		currentTurn = 1
	}

	if err := insertMove(db, game.ID, aiPlayerNumber, move, currentTurn); err != nil {
		l.Errorw("could not insert AI move", "move", move, "player", aiPlayerNumber, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not save AI move"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	winner, gameOver := game.GameOver()
	if gameOver {
		err = updateGameStatus(db, game.Slug, "finished", winner)
		if err != nil {
			l.Errorw("could not update game status after AI move", zap.Error(err))
		}
	}

	updatedGame, err := getGame(db, slug)
	if err != nil {
		l.Errorw("could not reload game after AI move", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not reload game"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	l.Infow("AI move executed", "slug", slug, "move", move, "hint", hint)
	if err := Renderer.JSON(w, http.StatusOK, updatedGame); err != nil {
		l.Errorw("failed to render game response", zap.Error(err))
	}
}
