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
// produces up to two steps (one per player half-turn). Board is the
// state immediately after Move was applied.
type ReplayStep struct {
	Turn   int64                     `json:"turn"`
	Player int                       `json:"player"`
	Move   string                    `json:"move"`
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
// @Description Returns an ordered list of every half-turn played in the
// @Description game, along with the board state after each one, so a
// @Description client can step through without making per-turn requests.
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

	game, ok := loadGameForRead(w, r, l)
	if !ok {
		return
	}

	steps, err := buildReplaySteps(game)
	if err != nil {
		l.Errorw("could not build replay", "slug", game.Slug, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "could not build replay"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	if err := Renderer.JSON(w, http.StatusOK, ReplayResponse{
		Slug:  game.Slug,
		Size:  game.Board.Size,
		Steps: steps,
	}); err != nil {
		l.Errorw("failed to render replay response", zap.Error(err))
	}
}

// @Summary Get board state after N complete turns
// @Description Replays the game forward until it has applied every move
// @Description of every turn with Number <= turn, then returns the
// @Description resulting board. turn=0 yields the starting (empty)
// @Description position; turn beyond the final turn yields the final
// @Description position.
// @Tags game
// @Accept json
// @Produce json
// @Param slug path string true "Game slug identifier"
// @Param turn path int true "Turn number (0 = starting position)"
// @Success 200 {object} PositionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /game/{slug}/position/{turn} [get]
func getPositionHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	turnStr := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "turn"))
	turnNum, err := strconv.ParseInt(turnStr, 10, 64)
	if err != nil || turnNum < 0 {
		l.Warnw("invalid turn number", "turn", turnStr, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusBadRequest, ErrorResponse{Error: "turn must be a non-negative integer"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	game, ok := loadGameForRead(w, r, l)
	if !ok {
		return
	}

	board, err := boardAtTurn(game, turnNum)
	if err != nil {
		l.Errorw("could not replay to turn", "slug", game.Slug, "turn", turnNum, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "could not compute position"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	if err := Renderer.JSON(w, http.StatusOK, PositionResponse{
		Slug:  game.Slug,
		Size:  game.Board.Size,
		Turn:  turnNum,
		Board: board,
	}); err != nil {
		l.Errorw("failed to render position response", zap.Error(err))
	}
}

// loadGameForRead handles the getDB + getGame boilerplate shared by the
// read-only game endpoints. It writes the appropriate error response
// itself; callers should bail when it returns ok=false.
func loadGameForRead(w http.ResponseWriter, r *http.Request, l *zap.SugaredLogger) (*gotak.Game, bool) {
	ctx := r.Context()
	db, err := getDB()
	if err != nil {
		l.Errorw("could not get db", zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "bad connection to db"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return nil, false
	}

	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))
	game, err := getGame(db, slug)
	if err != nil {
		l.Errorw("could not get game", "slug", slug, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusNotFound, ErrorResponse{Error: "game not found"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return nil, false
	}
	return game, true
}

// buildReplaySteps walks the game and applies each half-turn to a fresh
// board, snapshotting the resulting state into a ReplayStep.
//
// We do not reuse game.Board (already populated by getGame's replayMoves
// call) because we need every intermediate state, not just the final one.
func buildReplaySteps(game *gotak.Game) ([]ReplayStep, error) {
	if game == nil || game.Board == nil {
		return nil, nil
	}
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
			if err := applyHalfTurn(board, turn, false); err != nil {
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
			if err := applyHalfTurn(board, turn, true); err != nil {
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

// boardAtTurn returns the board state after every move of every turn with
// Number <= turnNum has been applied. turnNum=0 yields the starting
// position; turnNum beyond the final recorded turn yields the final
// position.
func boardAtTurn(game *gotak.Game, turnNum int64) (map[string][]*gotak.Stone, error) {
	if game == nil || game.Board == nil {
		return nil, nil
	}
	board := &gotak.Board{Size: game.Board.Size}
	if err := board.Init(); err != nil {
		return nil, err
	}

	for _, turn := range game.Turns {
		if turn == nil {
			continue
		}
		if turn.Number > turnNum {
			continue // skip rather than break: don't assume Turns is sorted
		}
		if turn.First != nil {
			if err := applyHalfTurn(board, turn, false); err != nil {
				return nil, err
			}
		}
		if turn.Second != nil {
			if err := applyHalfTurn(board, turn, true); err != nil {
				return nil, err
			}
		}
	}
	return snapshotSquares(board), nil
}

// applyHalfTurn applies one move from a turn to the board with the
// correct color. The "first" / "second" boolean selects which move;
// turn 1 inverts the colors because each player places the opponent's
// stone on the opening turn.
func applyHalfTurn(b *gotak.Board, turn *gotak.Turn, second bool) error {
	var mv *gotak.Move
	var player int
	if second {
		mv = turn.Second
		player = gotak.PlayerBlack
		if turn.Number == 1 {
			player = gotak.PlayerWhite
		}
	} else {
		mv = turn.First
		player = gotak.PlayerWhite
		if turn.Number == 1 {
			player = gotak.PlayerBlack
		}
	}
	if mv == nil {
		return nil
	}
	return b.DoMove(mv, player)
}

// snapshotSquares deep-copies a board's square map so each ReplayStep
// holds an independent view that won't change as later moves are
// applied to the live board.
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
