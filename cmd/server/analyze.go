package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"go.icco.me/gotak"
	"go.icco.me/gotak/ai"
	"go.icco.me/gutil/logging"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// defaultAnalyzeTimeLimit is the per-move budget the engine gets when the
// caller doesn't supply one. Two seconds at Advanced is enough to flag
// most obvious blunders without making a 50-move analysis last forever.
const defaultAnalyzeTimeLimit = 2 * time.Second

// maxAnalyzeTimeLimit caps the per-position budget a caller can ask for,
// since analysis endpoints are public.
const maxAnalyzeTimeLimit = 10 * time.Second

// analyzeDeadlineSlack covers DB reads and rendering on top of search time.
const analyzeDeadlineSlack = 10 * time.Second

// maxCriticalMoments is how many of the worst moves a game analysis lists.
const maxCriticalMoments = 3

// Move quality labels, from a loss measured in engine points (about 400
// per flat on top).
const (
	QualityBest       = "best"
	QualityGood       = "good"
	QualityInaccuracy = "inaccuracy"
	QualityMistake    = "mistake"
	QualityBlunder    = "blunder"
)

// AnalyzeRequest configures the analysis engine. All fields are optional.
// TimeLimit is a per-position budget in nanoseconds, capped at 10s.
type AnalyzeRequest struct {
	Level     string        `json:"level"`
	Style     string        `json:"style"`
	TimeLimit time.Duration `json:"time_limit"`
}

// MoveAnalysis is the engine's verdict on a single move (one half-turn).
// Played is what the player did and Best is what the engine would have
// done in the same position. Scores are from White's point of view: Score
// before the move, ScoreAfter once it is played. Loss is what the move
// cost the player who made it and Quality buckets that loss.
type MoveAnalysis struct {
	Turn       int64    `json:"turn"`
	Player     int      `json:"player"`
	Played     string   `json:"played"`
	Best       string   `json:"best"`
	Agreed     bool     `json:"agreed"`
	BestLine   []string `json:"best_line,omitempty"`
	TPS        string   `json:"tps,omitempty"`
	Score      int64    `json:"score"`
	ScoreAfter int64    `json:"score_after"`
	Loss       int64    `json:"loss"`
	Quality    string   `json:"quality,omitempty"`
	// Error captures why the engine couldn't evaluate this move, if any.
	// When non-empty, the engine fields should be ignored.
	Error string `json:"error,omitempty"`
}

// CriticalMoment points at one of the costliest moves in a game. Index is
// the position of the move in AnalyzeResponse.Moves.
type CriticalMoment struct {
	Index   int    `json:"index"`
	Turn    int64  `json:"turn"`
	Player  int    `json:"player"`
	Played  string `json:"played"`
	Best    string `json:"best"`
	Loss    int64  `json:"loss"`
	Quality string `json:"quality"`
}

// AnalyzeResponse is the payload returned by POST /analyze/game/{slug}.
// CriticalMoments lists the worst inaccuracies, mistakes, and blunders,
// costliest first.
type AnalyzeResponse struct {
	Slug            string           `json:"slug"`
	Size            int64            `json:"size"`
	Level           string           `json:"level"`
	Moves           []MoveAnalysis   `json:"moves"`
	MoveCount       int              `json:"move_count"`
	Agreed          int              `json:"agreed"`
	CriticalMoments []CriticalMoment `json:"critical_moments"`
}

// positionEvaluator is the slice of ai.Analyzer that game analysis needs,
// so tests can script engine output.
type positionEvaluator interface {
	Evaluate(ctx context.Context, g *gotak.Game) (*ai.PositionAnalysis, error)
}

// @Summary Analyze a game move-by-move
// @Description Walks the game move-by-move and scores every position with
// @Description the engine. Each move gets the engine's preferred move and
// @Description line, White-relative scores before and after, the loss it
// @Description cost its player (about 400 per flat), and a quality label.
// @Description critical_moments lists the costliest moves. Boards up to 8x8.
// @Tags analysis
// @Accept json
// @Produce json
// @Param slug path string true "Game slug identifier"
// @Param request body AnalyzeRequest false "Engine config (optional)"
// @Success 200 {object} AnalyzeResponse
// @Success 202 {object} AnalyzeJobResponse "Long game: analysis runs in the background; poll status_url"
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

	size, err := analyzableSize(game)
	if err != nil {
		l.Warnw("cannot analyze game", "slug", slug, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}
	budget := time.Duration(countPlies(game)+1) * cfg.TimeLimit
	if budget > syncAnalyzeBudget {
		job, start, err := claimAnalysisJob(db, key, budget, time.Now())
		if err != nil {
			l.Errorw("could not claim analysis job", "slug", slug, zap.Error(err))
			if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "could not start analysis"}); jerr != nil {
				l.Errorw("failed to render JSON", zap.Error(jerr))
			}
			return
		}
		if start {
			newEv := func() (positionEvaluator, error) { return ai.NewAnalyzer(size, cfg) }
			go runAnalysisJob(context.WithoutCancel(ctx), db, l, job, key, newEv, game, budget)
		}
		resp := jobResponse(db, l, job, slug, game.Board.Size, time.Now())
		w.Header().Set("Location", resp.StatusURL)
		if jerr := Renderer.JSON(w, http.StatusAccepted, resp); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	analyzer, err := ai.NewAnalyzer(size, cfg)
	if err != nil {
		l.Warnw("cannot analyze game", "slug", slug, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	extendWriteDeadline(w, l, budget+analyzeDeadlineSlack)
	actx, cancel := context.WithTimeout(ctx, budget+analyzeDeadlineSlack/2)
	defer cancel()

	release, err := acquireEngineSlot(actx)
	if err != nil {
		writeEngineBusy(w, r, err)
		return
	}
	defer release()
	moves := analyzeGame(actx, analyzer, game)
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

func writeAnalyzeResponse(w http.ResponseWriter, l *zap.SugaredLogger, slug string, size int64, level string, resp AnalyzeResponse) {
	resp.Slug = slug
	resp.Size = size
	resp.Level = level
	resp.CriticalMoments = criticalMoments(resp.Moves)
	if err := Renderer.JSON(w, http.StatusOK, resp); err != nil {
		l.Errorw("failed to render analyze response", zap.Error(err))
	}
}

// extendWriteDeadline lifts the server-wide WriteTimeout for handlers
// whose work legitimately takes longer.
func extendWriteDeadline(w http.ResponseWriter, l *zap.SugaredLogger, d time.Duration) {
	err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(d))
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		l.Warnw("could not extend write deadline", zap.Error(err))
	}
}

// gameCacheVersion is the cache-invalidation fingerprint. v2 added scores
// to every move, so v1 rows are never served.
func gameCacheVersion(g *gotak.Game) string {
	return fmt.Sprintf("v2:moves=%d", countPlies(g))
}

func countPlies(g *gotak.Game) int {
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
	return count
}

type analysisCacheKey struct {
	gameID      int64
	level       string
	style       string
	timeLimitNs int64
	gameVersion string
}

// loadAnalysisCache logs non-NotFound errors so a flaky cache is observable,
// but always degrades to a miss so caching never blocks analysis.
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

// saveAnalysisCache is best-effort; concurrent misses can race so writes
// use ON CONFLICT DO NOTHING. Runs cut short by a timeout are not cached,
// so a retry with a longer budget can fill them in.
func saveAnalysisCache(db *gorm.DB, l *zap.SugaredLogger, k analysisCacheKey, agreed int, moves []MoveAnalysis) {
	for _, m := range moves {
		if m.Error != "" {
			return
		}
	}
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
		timeLimit = min(req.TimeLimit, maxAnalyzeTimeLimit)
	}
	return ai.AIConfig{Level: level, Style: style, TimeLimit: timeLimit}, levelName
}

type ply struct {
	turnIdx  int
	isSecond bool
	turn     int64
	player   int
	played   string
}

func gamePlies(g *gotak.Game) []ply {
	var out []ply
	for turnIdx, turn := range g.Turns {
		if turn == nil {
			continue
		}
		if turn.First != nil {
			out = append(out, ply{turnIdx, false, turn.Number, gotak.PlayerWhite, turn.First.Text})
		}
		if turn.Second != nil {
			out = append(out, ply{turnIdx, true, turn.Number, gotak.PlayerBlack, turn.Second.Text})
		}
	}
	return out
}

// analyzeGame scores the position before every recorded move plus the
// final position, then judges each move by how the score moved. Returns
// one MoveAnalysis per recorded move (one per player half-turn). The
// original game is not mutated.
//
// Honours ctx cancellation: once the context is done, the remaining
// moves carry Error so callers see the budget was exhausted rather than
// getting a silently truncated list.
func analyzeGame(ctx context.Context, ev positionEvaluator, g *gotak.Game) []MoveAnalysis {
	out := []MoveAnalysis{}
	if g == nil || g.Board == nil || len(g.Turns) == 0 {
		return out
	}

	plies := gamePlies(g)
	if len(plies) == 0 {
		return out
	}
	size, err := analyzableSize(g)
	if err != nil {
		for _, p := range plies {
			out = append(out, MoveAnalysis{Turn: p.turn, Player: p.player, Played: p.played, Error: err.Error()})
		}
		return out
	}
	evals := make([]*ai.PositionAnalysis, len(plies)+1)
	errs := make([]error, len(plies)+1)
	for i := range evals {
		turnIdx, isSecond := len(g.Turns), false
		if i < len(plies) {
			turnIdx, isSecond = plies[i].turnIdx, plies[i].isSecond
		}
		evals[i], errs[i] = evaluateBefore(ctx, ev, g, turnIdx, isSecond)
	}

	for i, p := range plies {
		m := MoveAnalysis{Turn: p.turn, Player: p.player, Played: p.played}
		if errs[i] != nil {
			m.Error = errs[i].Error()
			out = append(out, m)
			continue
		}
		before := evals[i]
		m.Best = before.Best
		m.BestLine = before.PV
		m.TPS = before.TPS
		m.Score = before.Score
		m.Agreed = ai.SameMove(p.played, before.Best, size)
		if errs[i+1] != nil {
			m.Error = errs[i+1].Error()
			out = append(out, m)
			continue
		}
		m.ScoreAfter = evals[i+1].Score
		if !m.Agreed {
			m.Loss = ai.Loss(m.Score, m.ScoreAfter, p.player)
		}
		m.Quality = moveQuality(m.Agreed, m.Loss)
		out = append(out, m)
	}
	return out
}

// analyzableSize returns the board size as an int once it is known to be
// one the engine can search.
func analyzableSize(g *gotak.Game) (int, error) {
	if g == nil || g.Board == nil {
		return 0, errors.New("game has no board")
	}
	s := g.Board.Size
	if s < 3 || s > 8 {
		return 0, fmt.Errorf("analysis supports boards from 3x3 to 8x8, not %dx%d", s, s)
	}
	return int(s), nil
}

func evaluateBefore(ctx context.Context, ev positionEvaluator, g *gotak.Game, turnIdx int, isSecond bool) (*ai.PositionAnalysis, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pre, err := gameBeforeMove(g, turnIdx, isSecond)
	if err != nil {
		return nil, err
	}
	return ev.Evaluate(ctx, pre)
}

// moveQuality buckets a loss. Thresholds are fractions of a flat (about
// 400 points): under a quarter is noise, past two flats is a blunder.
func moveQuality(agreed bool, loss int64) string {
	switch {
	case agreed:
		return QualityBest
	case loss < 100:
		return QualityGood
	case loss < 300:
		return QualityInaccuracy
	case loss < 800:
		return QualityMistake
	default:
		return QualityBlunder
	}
}

func criticalMoments(moves []MoveAnalysis) []CriticalMoment {
	out := []CriticalMoment{}
	for i, m := range moves {
		switch m.Quality {
		case QualityInaccuracy, QualityMistake, QualityBlunder:
			out = append(out, CriticalMoment{
				Index:   i,
				Turn:    m.Turn,
				Player:  m.Player,
				Played:  m.Played,
				Best:    m.Best,
				Loss:    m.Loss,
				Quality: m.Quality,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Loss > out[j].Loss })
	if len(out) > maxCriticalMoments {
		out = out[:maxCriticalMoments]
	}
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
