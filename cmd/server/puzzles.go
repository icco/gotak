package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.icco.me/gotak"
	"go.icco.me/gotak/ai"
	"go.icco.me/gutil/logging"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	puzzleMineTimeLimit = 1 * time.Second
	puzzleDeadlineSlack = 10 * time.Second
)

// TacticalPuzzleResponse is the payload returned by GET /puzzles/tactical.
type TacticalPuzzleResponse struct {
	ID     int64  `json:"id"`
	GameID int64  `json:"game_id"`
	Ply    int    `json:"ply"`
	Turn   int64  `json:"turn"`
	Player int    `json:"player"`
	Size   int64  `json:"size"`
	TPS    string `json:"tps"`
	Depth  int    `json:"depth"`
	Source string `json:"source"`
}

// PuzzleAttemptRequest is the payload sent to POST /puzzles/{id}/attempt.
type PuzzleAttemptRequest struct {
	Move   string `json:"move"`
	Reveal bool   `json:"reveal"`
}

// PuzzleAttemptResponse is the payload returned by POST /puzzles/{id}/attempt.
type PuzzleAttemptResponse struct {
	Correct  bool     `json:"correct"`
	Solution []string `json:"solution,omitempty"`
}

// MinePuzzlesResponse is the payload returned by POST /puzzles/mine/{slug}.
type MinePuzzlesResponse struct {
	Slug    string `json:"slug"`
	Mined   int    `json:"mined"`
	Puzzles int    `json:"puzzles"`
}

// minePuzzles walks a game's positions and records positions where the side
// to move has ForcedWin == side to move. It keeps only the first position
// of each forced sequence and prefers missed wins.
func minePuzzles(ctx context.Context, ev positionEvaluator, g *gotak.Game) []Puzzle {
	if g == nil || g.Board == nil || len(g.Turns) == 0 {
		return nil
	}

	plies := gamePlies(g)
	if len(plies) == 0 {
		return nil
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

	var candidates []Puzzle
	var inWinningSequence bool

	for i, p := range plies {
		if errs[i] != nil || evals[i] == nil {
			inWinningSequence = false
			continue
		}

		cur := evals[i]
		if cur.ForcedWin != p.player {
			inWinningSequence = false
			continue
		}

		if inWinningSequence {
			continue
		}
		inWinningSequence = true

		if len(cur.PV) == 0 {
			continue
		}

		source := "winning_line"
		if errs[i+1] == nil && evals[i+1] != nil {
			if evals[i+1].ForcedWin != p.player {
				source = "missed_win"
			}
		}

		solLen := len(cur.PV)
		if solLen%2 == 0 {
			solLen--
		}
		if solLen <= 0 {
			solLen = 1
		}
		sol := cur.PV[:solLen]

		solJSON, err := json.Marshal(sol)
		if err != nil {
			continue
		}

		candidates = append(candidates, Puzzle{
			GameID:   g.ID,
			Ply:      i,
			Turn:     p.turn,
			Player:   p.player,
			Size:     g.Board.Size,
			TPS:      cur.TPS,
			Solution: string(solJSON),
			Depth:    cur.Depth,
			Source:   source,
		})
	}

	return candidates
}

func savePuzzles(db *gorm.DB, l *zap.SugaredLogger, puzzles []Puzzle) (int, error) {
	if len(puzzles) == 0 {
		return 0, nil
	}
	inserted := 0
	for _, p := range puzzles {
		res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&p)
		if res.Error != nil {
			l.Warnw("failed to save puzzle", "game_id", p.GameID, "ply", p.Ply, zap.Error(res.Error))
			return inserted, res.Error
		}
		if res.RowsAffected > 0 {
			inserted++
		}
	}
	return inserted, nil
}

func postMinePuzzlesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	db, err := getDB()
	if err != nil {
		l.Errorw("could not get db", zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "bad connection to db"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	user := getMustUserFromContext(r)
	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))

	if err := verifyGameParticipation(db, slug, user.ID); err != nil {
		l.Errorw("game participation verification failed", "slug", slug, "user_id", user.ID, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusForbidden, ErrorResponse{Error: "access denied: you are not a participant in this game"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	game, err := getGame(db, slug)
	if err != nil {
		l.Errorw("could not get game", "slug", slug, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusNotFound, ErrorResponse{Error: "game not found"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	size, err := analyzableSize(game)
	if err != nil {
		l.Warnw("cannot mine puzzles from game", "slug", slug, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	cfg := ai.AIConfig{
		Level:     ai.Intermediate,
		TimeLimit: puzzleMineTimeLimit,
	}
	analyzer, err := ai.NewAnalyzer(size, cfg)
	if err != nil {
		l.Warnw("cannot create analyzer", "slug", slug, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	budget := time.Duration(countPlies(game)+1) * cfg.TimeLimit
	extendWriteDeadline(w, l, budget+puzzleDeadlineSlack)
	actx, cancel := context.WithTimeout(ctx, budget+puzzleDeadlineSlack/2)
	defer cancel()

	puzzles := minePuzzles(actx, analyzer, game)
	inserted, err := savePuzzles(db, l, puzzles)
	if err != nil {
		l.Errorw("failed to save puzzles", "slug", slug, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to save puzzles"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	if jerr := Renderer.JSON(w, http.StatusOK, MinePuzzlesResponse{
		Slug:    slug,
		Mined:   len(puzzles),
		Puzzles: inserted,
	}); jerr != nil {
		l.Errorw("failed to render JSON", zap.Error(jerr))
	}
}

func getTacticalPuzzleHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	db, err := getDB()
	if err != nil {
		l.Errorw("could not get db", zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "bad connection to db"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	sizeStr := r.URL.Query().Get("size")
	query := db.Model(&Puzzle{})
	if sizeStr != "" {
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil || size < 3 || size > 8 {
			if jerr := Renderer.JSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid size parameter"}); jerr != nil {
				l.Errorw("failed to render JSON", zap.Error(jerr))
			}
			return
		}
		query = query.Where("size = ?", size)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		l.Errorw("failed to count puzzles", zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to load puzzle"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	if count == 0 {
		if jerr := Renderer.JSON(w, http.StatusNotFound, ErrorResponse{Error: "no puzzles found"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	n, err := rand.Int(rand.Reader, big.NewInt(count))
	if err != nil {
		l.Errorw("failed to generate random offset", zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to load puzzle"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	var puzzle Puzzle
	if err := query.Offset(int(n.Int64())).First(&puzzle).Error; err != nil {
		l.Errorw("failed to load random puzzle", zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to load puzzle"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	resp := TacticalPuzzleResponse{
		ID:     puzzle.ID,
		GameID: puzzle.GameID,
		Ply:    puzzle.Ply,
		Turn:   puzzle.Turn,
		Player: puzzle.Player,
		Size:   puzzle.Size,
		TPS:    puzzle.TPS,
		Depth:  puzzle.Depth,
		Source: puzzle.Source,
	}

	if jerr := Renderer.JSON(w, http.StatusOK, resp); jerr != nil {
		l.Errorw("failed to render JSON", zap.Error(jerr))
	}
}

func postPuzzleAttemptHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	db, err := getDB()
	if err != nil {
		l.Errorw("could not get db", zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "bad connection to db"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	idStr := chi.URLParamFromCtx(ctx, "id")
	puzzleID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		if jerr := Renderer.JSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid puzzle id"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	var req PuzzleAttemptRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			if jerr := Renderer.JSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"}); jerr != nil {
				l.Errorw("failed to render JSON", zap.Error(jerr))
			}
			return
		}
	}

	var puzzle Puzzle
	if err := db.First(&puzzle, puzzleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if jerr := Renderer.JSON(w, http.StatusNotFound, ErrorResponse{Error: "puzzle not found"}); jerr != nil {
				l.Errorw("failed to render JSON", zap.Error(jerr))
			}
			return
		}
		l.Errorw("could not get puzzle", "id", puzzleID, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "database error"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	var solution []string
	if err := json.Unmarshal([]byte(puzzle.Solution), &solution); err != nil {
		l.Errorw("corrupt puzzle solution JSON", "id", puzzleID, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "corrupt puzzle solution"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	if len(solution) == 0 {
		l.Errorw("empty puzzle solution", "id", puzzleID)
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "empty puzzle solution"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	correct := ai.SameMove(strings.TrimSpace(req.Move), solution[0], int(puzzle.Size))

	resp := PuzzleAttemptResponse{
		Correct: correct,
	}
	if correct || req.Reveal {
		resp.Solution = solution
	}

	if jerr := Renderer.JSON(w, http.StatusOK, resp); jerr != nil {
		l.Errorw("failed to render JSON", zap.Error(jerr))
	}
}
