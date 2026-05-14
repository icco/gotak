package main

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/icco/gotak"
	"github.com/icco/gutil/logging"
	"go.uber.org/zap"
)

// ReplayStep is a single recorded position in a game replay. Each turn
// produces up to two steps (one per player move). The board snapshot is
// the state after `Move` was applied.
type ReplayStep struct {
	Turn   int64                   `json:"turn"`
	Player int                     `json:"player"`
	Move   string                  `json:"move"`
	Board  map[string][]*gotak.Stone `json:"board"`
}

// ReplayResponse is the payload returned by GET /game/{slug}/replay.
type ReplayResponse struct {
	Slug  string       `json:"slug"`
	Size  int64        `json:"size"`
	Steps []ReplayStep `json:"steps"`
}

// PositionResponse is the payload returned by GET /game/{slug}/position/{turn}.
type PositionResponse struct {
	Slug  string                    `json:"slug"`
	Size  int64                     `json:"size"`
	Turn  int64                     `json:"turn"`
	Board map[string][]*gotak.Stone `json:"board"`
}

// @Summary Get full game replay
// @Description Returns an ordered list of every move in the game along with
// @Description the board state after each move, so a client can step through
// @Description without making per-turn API calls.
// @Tags game
// @Accept json
// @Produce json
// @Param slug path string true "Game slug identifier"
// @Success 200 {object} ReplayResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /game/{slug}/replay [get]
func getReplayHandler(w http.ResponseWriter, r *http.Request) {
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

	steps, err := buildReplaySteps(game)
	if err != nil {
		l.Errorw("could not build replay", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "could not build replay"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if err := Renderer.JSON(w, http.StatusOK, ReplayResponse{
		Slug:  slug,
		Size:  game.Board.Size,
		Steps: steps,
	}); err != nil {
		l.Errorw("failed to render replay response", zap.Error(err))
	}
}

// @Summary Get board state at a specific turn
// @Description Replays moves up to and including the requested turn number
// @Description and returns the resulting board state. Turn 0 returns the
// @Description starting position.
// @Tags game
// @Accept json
// @Produce json
// @Param slug path string true "Game slug identifier"
// @Param turn path int true "Turn number (0 = empty board)"
// @Success 200 {object} PositionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /game/{slug}/position/{turn} [get]
func getPositionHandler(w http.ResponseWriter, r *http.Request) {
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
	turnStr := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "turn"))
	turnNum, err := strconv.ParseInt(turnStr, 10, 64)
	if err != nil || turnNum < 0 {
		l.Errorw("invalid turn number", "slug", slug, "turn", turnStr, zap.Error(err))
		if err := Renderer.JSON(w, http.StatusBadRequest, ErrorResponse{Error: "turn must be a non-negative integer"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	game, err := getGame(db, slug)
	if err != nil {
		l.Errorw("could not get game", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, http.StatusNotFound, ErrorResponse{Error: "game not found"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	board, err := boardAtTurn(game, turnNum)
	if err != nil {
		l.Errorw("could not replay to turn", "slug", slug, "turn", turnNum, zap.Error(err))
		if err := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "could not compute position"}); err != nil {
			l.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if err := Renderer.JSON(w, http.StatusOK, PositionResponse{
		Slug:  slug,
		Size:  game.Board.Size,
		Turn:  turnNum,
		Board: board,
	}); err != nil {
		l.Errorw("failed to render position response", zap.Error(err))
	}
}

// buildReplaySteps walks the game move-by-move, applying each to a fresh
// board and snapshotting the resulting state.
func buildReplaySteps(game *gotak.Game) ([]ReplayStep, error) {
	board := &gotak.Board{Size: game.Board.Size}
	if err := board.Init(); err != nil {
		return nil, err
	}

	steps := make([]ReplayStep, 0, len(game.Turns)*2)
	for _, turn := range game.Turns {
		if turn == nil {
			continue
		}
		if turn.First != nil {
			player := gotak.PlayerWhite
			if turn.Number == 1 {
				// Tak rule: the first turn places the opponent's stone.
				player = gotak.PlayerBlack
			}
			if err := board.DoMove(turn.First, player); err != nil {
				return nil, err
			}
			steps = append(steps, ReplayStep{
				Turn:   turn.Number,
				Player: gotak.PlayerWhite,
				Move:   turn.First.Text,
				Board:  snapshotSquares(board),
			})
		}
		if turn.Second != nil {
			player := gotak.PlayerBlack
			if turn.Number == 1 {
				player = gotak.PlayerWhite
			}
			if err := board.DoMove(turn.Second, player); err != nil {
				return nil, err
			}
			steps = append(steps, ReplayStep{
				Turn:   turn.Number,
				Player: gotak.PlayerBlack,
				Move:   turn.Second.Text,
				Board:  snapshotSquares(board),
			})
		}
	}
	return steps, nil
}

// boardAtTurn returns the board state after `turnNum` complete turns have
// been played. turnNum=0 returns the starting position; turnNum >= len(Turns)
// returns the final position.
func boardAtTurn(game *gotak.Game, turnNum int64) (map[string][]*gotak.Stone, error) {
	board := &gotak.Board{Size: game.Board.Size}
	if err := board.Init(); err != nil {
		return nil, err
	}

	for _, turn := range game.Turns {
		if turn == nil || turn.Number > turnNum {
			break
		}
		if turn.First != nil {
			player := gotak.PlayerWhite
			if turn.Number == 1 {
				player = gotak.PlayerBlack
			}
			if err := board.DoMove(turn.First, player); err != nil {
				return nil, err
			}
		}
		if turn.Second != nil {
			player := gotak.PlayerBlack
			if turn.Number == 1 {
				player = gotak.PlayerWhite
			}
			if err := board.DoMove(turn.Second, player); err != nil {
				return nil, err
			}
		}
	}
	return snapshotSquares(board), nil
}

// snapshotSquares deep-copies a board's square map so callers can keep
// references that survive further mutation of the original board.
func snapshotSquares(b *gotak.Board) map[string][]*gotak.Stone {
	out := make(map[string][]*gotak.Stone, len(b.Squares))
	for sq, stones := range b.Squares {
		if len(stones) == 0 {
			out[sq] = []*gotak.Stone{}
			continue
		}
		copied := make([]*gotak.Stone, len(stones))
		for i, s := range stones {
			if s == nil {
				continue
			}
			st := *s
			copied[i] = &st
		}
		out[sq] = copied
	}
	return out
}
