package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/icco/gotak"
	"github.com/icco/gotak/ai"
	"github.com/icco/gutil/logging"
	"go.uber.org/zap"
)

// AnalyzeRequest configures the analysis engine. All fields are optional;
// sensible defaults are used.
type AnalyzeRequest struct {
	Level     string        `json:"level"`
	Style     string        `json:"style"`
	TimeLimit time.Duration `json:"time_limit"`
}

// PlyAnalysis is the result for a single half-turn.
type PlyAnalysis struct {
	Turn   int64  `json:"turn"`
	Player int    `json:"player"`
	Played string `json:"played"`
	Best   string `json:"best"`
	Agreed bool   `json:"agreed"`
	// Error captures why the engine couldn't evaluate this ply, if any.
	// When non-empty Best and Agreed are meaningless.
	Error string `json:"error,omitempty"`
}

// AnalyzeResponse is the payload returned by POST /analyze/game/{slug}.
type AnalyzeResponse struct {
	Slug   string        `json:"slug"`
	Size   int64         `json:"size"`
	Level  string        `json:"level"`
	Plies  []PlyAnalysis `json:"plies"`
	Played int           `json:"played"`
	Agreed int           `json:"agreed"`
}

// @Summary Analyze a game move-by-move
// @Description For each ply, asks the AI engine what it would play in that
// @Description position and records whether the player's move matches. A
// @Description rough "blunder detector"; intended as a starting point for
// @Description deeper analysis features.
// @Tags analysis
// @Accept json
// @Produce json
// @Param slug path string true "Game slug identifier"
// @Param request body AnalyzeRequest false "Engine config (optional)"
// @Success 200 {object} AnalyzeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analyze/game/{slug} [post]
func postAnalyzeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	db, err := getDB()
	if err != nil {
		l.Errorw("could not get db", zap.Error(err))
		if err := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "bad connection to db"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))
	game, err := getGame(db, slug)
	if err != nil {
		l.Errorw("could not get game", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, http.StatusNotFound, ErrorResponse{Error: "game not found"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Empty body is OK; defaults apply.
	var req AnalyzeRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	cfg := analyzeConfigFromRequest(req)

	plies := analyzeGame(ctx, &ai.TakticianEngine{}, game, cfg)
	resp := AnalyzeResponse{
		Slug:   slug,
		Size:   game.Board.Size,
		Level:  string(req.Level),
		Plies:  plies,
		Played: len(plies),
	}
	for _, p := range plies {
		if p.Agreed {
			resp.Agreed++
		}
	}
	if resp.Level == "" {
		resp.Level = "advanced"
	}

	if err := Renderer.JSON(w, http.StatusOK, resp); err != nil {
		l.Errorw("failed to render analyze response", zap.Error(err))
	}
}

// analyzeConfigFromRequest maps the wire-level request to an ai.AIConfig,
// applying defaults aimed at making the analysis stronger than typical
// gameplay (we'd rather flag a real blunder than miss one).
func analyzeConfigFromRequest(req AnalyzeRequest) ai.AIConfig {
	level := ai.Advanced
	switch req.Level {
	case "beginner":
		level = ai.Beginner
	case "intermediate":
		level = ai.Intermediate
	case "advanced":
		level = ai.Advanced
	case "expert":
		level = ai.Expert
	}

	style := ai.Balanced
	switch req.Style {
	case "aggressive":
		style = ai.Aggressive
	case "defensive":
		style = ai.Defensive
	}

	timeLimit := 2 * time.Second
	if req.TimeLimit > 0 {
		timeLimit = req.TimeLimit
	}
	return ai.AIConfig{Level: level, Style: style, TimeLimit: timeLimit}
}

// analyzeGame walks `g` ply by ply and asks `engine` for the best move at
// each position. The original game is not mutated.
func analyzeGame(ctx context.Context, engine ai.Engine, g *gotak.Game, cfg ai.AIConfig) []PlyAnalysis {
	plies := []PlyAnalysis{}
	if g == nil || len(g.Turns) == 0 {
		return plies
	}

	for turnIdx, turn := range g.Turns {
		if turn == nil {
			continue
		}
		if turn.First != nil {
			plies = append(plies, evaluatePly(ctx, engine, g, turnIdx, false, turn.Number, gotak.PlayerWhite, turn.First.Text, cfg))
		}
		if turn.Second != nil {
			plies = append(plies, evaluatePly(ctx, engine, g, turnIdx, true, turn.Number, gotak.PlayerBlack, turn.Second.Text, cfg))
		}
	}
	return plies
}

func evaluatePly(
	ctx context.Context,
	engine ai.Engine,
	orig *gotak.Game,
	turnIdx int,
	second bool,
	turnNumber int64,
	player int,
	played string,
	cfg ai.AIConfig,
) PlyAnalysis {
	out := PlyAnalysis{Turn: turnNumber, Player: player, Played: played}

	pre, err := gameBeforePly(orig, turnIdx, second)
	if err != nil {
		out.Error = err.Error()
		return out
	}

	best, err := engine.GetMove(ctx, pre, cfg)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.Best = best
	out.Agreed = best == played
	return out
}

// gameBeforePly returns a fresh *gotak.Game whose Turns slice represents
// the state of `orig` immediately before the ply identified by
// (turnIdx, second). If `second` is true, the first move of orig.Turns[turnIdx]
// is included; otherwise only orig.Turns[0..turnIdx-1] are included.
//
// The returned game shares move pointers with the original (the engine
// reads moves but does not mutate them).
func gameBeforePly(orig *gotak.Game, turnIdx int, second bool) (*gotak.Game, error) {
	g, err := gotak.NewGame(orig.Board.Size, orig.ID, orig.Slug)
	if err != nil {
		return nil, err
	}
	g.Meta = orig.Meta

	for i := 0; i < turnIdx; i++ {
		t := orig.Turns[i]
		if t == nil {
			continue
		}
		g.Turns = append(g.Turns, &gotak.Turn{
			Number: t.Number,
			First:  t.First,
			Second: t.Second,
		})
	}
	if second && turnIdx < len(orig.Turns) && orig.Turns[turnIdx] != nil {
		t := orig.Turns[turnIdx]
		g.Turns = append(g.Turns, &gotak.Turn{
			Number: t.Number,
			First:  t.First,
		})
	}
	return g, nil
}
