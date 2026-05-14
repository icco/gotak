package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"fmt"

	"github.com/go-chi/chi/v5"
	"github.com/icco/gotak"
	"github.com/icco/gotak/ai"
	"github.com/icco/gutil/logging"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// defaultAnalyzeTimeLimit is the per-move budget the engine gets when the
// caller doesn't supply one. Two seconds at Advanced is enough to flag
// most obvious blunders without making a 50-move analysis last forever.
const defaultAnalyzeTimeLimit = 2 * time.Second

// AnalyzeRequest configures the analysis engine. All fields are optional.
type AnalyzeRequest struct {
	Level     string        `json:"level"`
	Style     string        `json:"style"`
	TimeLimit time.Duration `json:"time_limit"`
}

// MoveAnalysis is the engine's verdict on a single move (one half-turn).
// `Played` is what the player did; `Best` is what the engine would have
// done in the same position; `Agreed` is `Played == Best`.
type MoveAnalysis struct {
	Turn   int64  `json:"turn"`
	Player int    `json:"player"`
	Played string `json:"played"`
	Best   string `json:"best"`
	Agreed bool   `json:"agreed"`
	// Error captures why the engine couldn't evaluate this move, if any.
	// When non-empty, Best and Agreed should be ignored.
	Error string `json:"error,omitempty"`
}

// AnalyzeResponse is the payload returned by POST /analyze/game/{slug}.
type AnalyzeResponse struct {
	Slug      string         `json:"slug"`
	Size      int64          `json:"size"`
	Level     string         `json:"level"`
	Moves     []MoveAnalysis `json:"moves"`
	MoveCount int            `json:"move_count"`
	Agreed    int            `json:"agreed"`
}

// @Summary Analyze a game move-by-move
// @Description Walks the game move-by-move, asking the AI engine what it
// @Description would play at each position, and records whether the player's
// @Description move matches. Useful as a rough "blunder detector".
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

	req, err := decodeAnalyzeRequest(r)
	if err != nil {
		l.Warnw("invalid analyze request body", zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	db, err := getDB()
	if err != nil {
		l.Errorw("could not get db", zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "bad connection to db"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))
	game, err := getGame(db, slug)
	if err != nil {
		l.Errorw("could not get game", "slug", slug, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusNotFound, ErrorResponse{Error: "game not found"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	cfg, levelName := analyzeConfigFromRequest(req)
	key := analysisCacheKey{
		gameID:      game.ID,
		level:       levelName,
		style:       string(cfg.Style),
		timeLimitNs: int64(cfg.TimeLimit),
		gameVersion: gameCacheVersion(game),
	}

	if cached, ok := loadAnalysisCache(db, l, key); ok {
		writeAnalyzeResponse(w, l, slug, game.Board.Size, levelName, cached)
		return
	}

	moves := analyzeGame(ctx, &ai.TakticianEngine{}, game, cfg)
	agreed := 0
	for _, m := range moves {
		if m.Agreed {
			agreed++
		}
	}

	saveAnalysisCache(db, l, key, agreed, moves)

	writeAnalyzeResponse(w, l, slug, game.Board.Size, levelName, AnalyzeResponse{
		Moves:     moves,
		MoveCount: len(moves),
		Agreed:    agreed,
	})
}

// writeAnalyzeResponse fills the shared envelope fields and renders.
func writeAnalyzeResponse(w http.ResponseWriter, l *zap.SugaredLogger, slug string, size int64, level string, resp AnalyzeResponse) {
	resp.Slug = slug
	resp.Size = size
	resp.Level = level
	if err := Renderer.JSON(w, http.StatusOK, resp); err != nil {
		l.Errorw("failed to render analyze response", zap.Error(err))
	}
}

// gameCacheVersion is a fingerprint of the game state used as part of
// the analysis cache key. Today it's just the half-turn count, which
// invalidates the cache when a game grows. The string layout leaves
// room to mix in g.UpdatedAt or a content hash later if edit-in-place
// becomes possible.
func gameCacheVersion(g *gotak.Game) string {
	count := 0
	for _, t := range g.Turns {
		if t == nil {
			continue
		}
		if t.First != nil {
			count++
		}
		if t.Second != nil {
			count++
		}
	}
	return fmt.Sprintf("v1:moves=%d", count)
}

// analysisCacheKey is the composite key used to look up a stored result.
type analysisCacheKey struct {
	gameID      int64
	level       string
	style       string
	timeLimitNs int64
	gameVersion string
}

// loadAnalysisCache returns a cached analysis result for the given key.
// ok=false means cache miss. Real DB errors (other than "not found") are
// logged so a flaky cache is at least observable.
func loadAnalysisCache(db *gorm.DB, l *zap.SugaredLogger, k analysisCacheKey) (AnalyzeResponse, bool) {
	var row AnalysisCache
	err := db.Where("game_id = ? AND level = ? AND style = ? AND time_limit_ns = ? AND game_version = ?",
		k.gameID, k.level, k.style, k.timeLimitNs, k.gameVersion).First(&row).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			l.Warnw("analysis cache lookup failed", zap.Error(err))
		}
		return AnalyzeResponse{}, false
	}
	var moves []MoveAnalysis
	if err := json.Unmarshal([]byte(row.Moves), &moves); err != nil {
		l.Warnw("analysis cache row failed to decode", "id", row.ID, zap.Error(err))
		return AnalyzeResponse{}, false
	}
	return AnalyzeResponse{
		Moves:     moves,
		MoveCount: len(moves),
		Agreed:    row.Agreed,
	}, true
}

// saveAnalysisCache writes a result into the cache. Failures are logged
// but not propagated — caching is best-effort. Uses ON CONFLICT DO
// NOTHING so concurrent misses don't error on the unique index.
func saveAnalysisCache(db *gorm.DB, l *zap.SugaredLogger, k analysisCacheKey, agreed int, moves []MoveAnalysis) {
	encoded, err := json.Marshal(moves)
	if err != nil {
		l.Warnw("could not encode analysis for cache", zap.Error(err))
		return
	}
	row := AnalysisCache{
		GameID:      k.gameID,
		Level:       k.level,
		Style:       k.style,
		TimeLimitNs: k.timeLimitNs,
		GameVersion: k.gameVersion,
		Agreed:      agreed,
		Moves:       string(encoded),
	}
	err = db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
	if err != nil {
		l.Warnw("could not save analysis cache row", zap.Error(err))
	}
}

// decodeAnalyzeRequest reads the optional JSON body. An empty body is fine
// and yields a zero-value AnalyzeRequest; anything else that fails to
// decode is a 400.
func decodeAnalyzeRequest(r *http.Request) (AnalyzeRequest, error) {
	var req AnalyzeRequest
	if r.Body == nil {
		return req, nil
	}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil && !errors.Is(err, io.EOF) {
		return req, err
	}
	return req, nil
}

// analyzeConfigFromRequest maps the wire request to an ai.AIConfig.
// Returns both the config and the canonical level name (so the response
// can echo what was actually used rather than what the request asked
// for, which may differ when defaults kick in).
func analyzeConfigFromRequest(req AnalyzeRequest) (ai.AIConfig, string) {
	level, levelName := ai.Advanced, "advanced"
	switch req.Level {
	case "beginner":
		level, levelName = ai.Beginner, "beginner"
	case "intermediate":
		level, levelName = ai.Intermediate, "intermediate"
	case "advanced", "":
		level, levelName = ai.Advanced, "advanced"
	case "expert":
		level, levelName = ai.Expert, "expert"
	}

	style := ai.Balanced
	switch req.Style {
	case "aggressive":
		style = ai.Aggressive
	case "defensive":
		style = ai.Defensive
	}

	timeLimit := defaultAnalyzeTimeLimit
	if req.TimeLimit > 0 {
		timeLimit = req.TimeLimit
	}
	return ai.AIConfig{Level: level, Style: style, TimeLimit: timeLimit}, levelName
}

// analyzeGame walks `g` move by move and asks `engine` for the best move at
// each position. Returns one MoveAnalysis per recorded move (one per
// player half-turn). The original game is not mutated.
//
// Honours ctx cancellation: as soon as the context is done, the remaining
// moves are skipped with Error="canceled" so callers see the budget was
// exhausted rather than getting a silently truncated list.
func analyzeGame(ctx context.Context, engine ai.Engine, g *gotak.Game, cfg ai.AIConfig) []MoveAnalysis {
	out := []MoveAnalysis{}
	if g == nil || g.Board == nil || len(g.Turns) == 0 {
		return out
	}

	for turnIdx, turn := range g.Turns {
		if turn == nil {
			continue
		}
		if turn.First != nil {
			out = append(out, evaluateMove(ctx, engine, g, turnIdx, false, turn.Number, gotak.PlayerWhite, turn.First.Text, cfg))
		}
		if turn.Second != nil {
			out = append(out, evaluateMove(ctx, engine, g, turnIdx, true, turn.Number, gotak.PlayerBlack, turn.Second.Text, cfg))
		}
	}
	return out
}

func evaluateMove(
	ctx context.Context,
	engine ai.Engine,
	orig *gotak.Game,
	turnIdx int,
	isSecond bool,
	turnNumber int64,
	player int,
	played string,
	cfg ai.AIConfig,
) MoveAnalysis {
	out := MoveAnalysis{Turn: turnNumber, Player: player, Played: played}

	if err := ctx.Err(); err != nil {
		out.Error = err.Error()
		return out
	}

	pre, err := gameBeforeMove(orig, turnIdx, isSecond)
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

// gameBeforeMove returns a fresh *gotak.Game whose Turns slice represents
// the state of `orig` immediately before the move identified by
// (turnIdx, isSecond). When isSecond is true the first move of
// orig.Turns[turnIdx] is included; otherwise only orig.Turns[0..turnIdx-1]
// are included.
//
// The returned game shares move pointers with the original (the engine
// reads them but does not mutate).
func gameBeforeMove(orig *gotak.Game, turnIdx int, isSecond bool) (*gotak.Game, error) {
	if orig == nil || orig.Board == nil {
		return nil, errors.New("original game has no board")
	}
	g, err := gotak.NewGame(orig.Board.Size, orig.ID, orig.Slug)
	if err != nil {
		return nil, err
	}
	g.Meta = orig.Meta

	for i := 0; i < turnIdx && i < len(orig.Turns); i++ {
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
	if isSecond && turnIdx < len(orig.Turns) && orig.Turns[turnIdx] != nil {
		t := orig.Turns[turnIdx]
		g.Turns = append(g.Turns, &gotak.Turn{
			Number: t.Number,
			First:  t.First,
		})
	}
	return g, nil
}
