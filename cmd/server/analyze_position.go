package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.icco.me/gotak/ai"
	"go.icco.me/gutil/logging"
	"go.uber.org/zap"
)

// maxPositionBody bounds request bodies for the position endpoints.
const maxPositionBody = 64 << 10

// PositionAnalysisRequest asks for the engine's view of one TPS position.
// Level, Style, and TimeLimit behave as in AnalyzeRequest.
type PositionAnalysisRequest struct {
	TPS       string        `json:"tps" example:"x5/x5/x5/2,2,x3/1,1,1,1,x 1 5"`
	Level     string        `json:"level" example:"advanced"`
	Style     string        `json:"style" example:"balanced"`
	TimeLimit time.Duration `json:"time_limit" example:"2000000000"`
}

// PositionAnalysisResponse is the payload returned by POST /analyze/position.
type PositionAnalysisResponse struct {
	ai.PositionAnalysis
	Level string `json:"level"`
}

// CompareMovesRequest asks the engine to score candidate PTN moves from
// one TPS position. At most 16 moves.
type CompareMovesRequest struct {
	TPS       string        `json:"tps" example:"x5/x5/x5/2,2,x3/1,1,1,1,x 1 5"`
	Moves     []string      `json:"moves" example:"e1,a3"`
	Level     string        `json:"level" example:"advanced"`
	Style     string        `json:"style" example:"balanced"`
	TimeLimit time.Duration `json:"time_limit" example:"2000000000"`
}

// CompareMovesResponse is the payload returned by POST /analyze/compare-moves.
type CompareMovesResponse struct {
	ai.Comparison
	Level string `json:"level"`
}

// TreeAnalysisRequest asks for a move tree exploring alternative lines
// from one TPS position.
type TreeAnalysisRequest struct {
	TPS       string        `json:"tps" example:"x5/x5/x5/2,2,x3/1,1,1,1,x 1 5"`
	Breadth   int           `json:"breadth" example:"3"`
	Depth     int           `json:"depth" example:"2"`
	Level     string        `json:"level" example:"advanced"`
	Style     string        `json:"style" example:"balanced"`
	TimeLimit time.Duration `json:"time_limit" example:"2000000000"`
}

// TreeAnalysisResponse is the payload returned by POST /analyze/tree.
type TreeAnalysisResponse struct {
	ai.MoveTree
	Level string `json:"level"`
}

// @Summary Analyze a TPS position
// @Description Scores an arbitrary position given in TPS. Returns the
// @Description engine's best move and line, a White-relative score (about
// @Description 400 per flat; forced wins are +/-100000 with forced_win set),
// @Description and squares where either side could complete a road with one
// @Description flat. Boards from 4x4 to 8x8.
// @Tags analysis
// @Accept json
// @Produce json
// @Param request body PositionAnalysisRequest true "Position and engine config"
// @Success 200 {object} PositionAnalysisResponse
// @Failure 400 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /analyze/position [post]
func postAnalyzePositionHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	var req PositionAnalysisRequest
	if err := decodePositionBody(w, r, &req); err != nil {
		writeAnalysisError(w, l, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.TPS) == "" {
		writeAnalysisError(w, l, http.StatusBadRequest, errors.New("tps is required"))
		return
	}

	cfg, levelName := analyzeConfigFromRequest(AnalyzeRequest{Level: req.Level, Style: req.Style, TimeLimit: req.TimeLimit})
	extendWriteDeadline(w, l, cfg.TimeLimit+analyzeDeadlineSlack)
	actx, cancel := context.WithTimeout(ctx, cfg.TimeLimit+analyzeDeadlineSlack/2)
	defer cancel()

	release, err := acquireEngineSlot(actx)
	if err != nil {
		writeEngineBusy(w, r, err)
		return
	}
	defer release()
	res, err := ai.AnalyzeTPS(actx, req.TPS, cfg)
	if err != nil {
		writeAnalysisError(w, l, analysisErrorStatus(err), err)
		return
	}

	if err := Renderer.JSON(w, http.StatusOK, PositionAnalysisResponse{PositionAnalysis: *res, Level: levelName}); err != nil {
		l.Errorw("failed to render position analysis", zap.Error(err))
	}
}

// @Summary Compare candidate moves
// @Description Plays each candidate PTN move from a TPS position and scores
// @Description the result. Each move reports a White-relative score, its
// @Description loss against the engine's best move for the player to move,
// @Description and the engine's best reply. Sorted best first; illegal moves
// @Description carry an error and sort last. At most 16 moves.
// @Tags analysis
// @Accept json
// @Produce json
// @Param request body CompareMovesRequest true "Position, candidate moves, and engine config"
// @Success 200 {object} CompareMovesResponse
// @Failure 400 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /analyze/compare-moves [post]
func postCompareMovesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	var req CompareMovesRequest
	if err := decodePositionBody(w, r, &req); err != nil {
		writeAnalysisError(w, l, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.TPS) == "" {
		writeAnalysisError(w, l, http.StatusBadRequest, errors.New("tps is required"))
		return
	}
	if len(req.Moves) == 0 || len(req.Moves) > ai.MaxCompareMoves {
		writeAnalysisError(w, l, http.StatusBadRequest, errors.New("moves must list 1 to 16 PTN moves"))
		return
	}

	cfg, levelName := analyzeConfigFromRequest(AnalyzeRequest{Level: req.Level, Style: req.Style, TimeLimit: req.TimeLimit})
	budget := time.Duration(len(req.Moves)+1) * cfg.TimeLimit
	extendWriteDeadline(w, l, budget+analyzeDeadlineSlack)
	actx, cancel := context.WithTimeout(ctx, budget+analyzeDeadlineSlack/2)
	defer cancel()

	release, err := acquireEngineSlot(actx)
	if err != nil {
		writeEngineBusy(w, r, err)
		return
	}
	defer release()
	res, err := ai.CompareMovesTPS(actx, req.TPS, req.Moves, cfg)
	if err != nil {
		writeAnalysisError(w, l, analysisErrorStatus(err), err)
		return
	}

	if err := Renderer.JSON(w, http.StatusOK, CompareMovesResponse{Comparison: *res, Level: levelName}); err != nil {
		l.Errorw("failed to render move comparison", zap.Error(err))
	}
}

// @Summary Generate an alternative move tree
// @Description Builds a tree of candidate moves and variations from a TPS
// @Description position. Each node reports score and forced win; leaves carry
// @Description the engine's principal variation. Breadth up to 4, depth up to 3.
// @Tags analysis
// @Accept json
// @Produce json
// @Param request body TreeAnalysisRequest true "Position, tree shape, and engine config"
// @Success 200 {object} TreeAnalysisResponse
// @Failure 400 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /analyze/tree [post]
func postAnalyzeTreeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	var req TreeAnalysisRequest
	if err := decodePositionBody(w, r, &req); err != nil {
		writeAnalysisError(w, l, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.TPS) == "" {
		writeAnalysisError(w, l, http.StatusBadRequest, errors.New("tps is required"))
		return
	}
	if req.Breadth < 1 || req.Breadth > ai.MaxTreeBreadth {
		writeAnalysisError(w, l, http.StatusBadRequest, fmt.Errorf("breadth must be between 1 and %d", ai.MaxTreeBreadth))
		return
	}
	if req.Depth < 1 || req.Depth > ai.MaxTreeDepth {
		writeAnalysisError(w, l, http.StatusBadRequest, fmt.Errorf("depth must be between 1 and %d", ai.MaxTreeDepth))
		return
	}

	cfg, levelName := analyzeConfigFromRequest(AnalyzeRequest{Level: req.Level, Style: req.Style, TimeLimit: req.TimeLimit})
	maxNodes := maxTreeNodesFor(req.Breadth, req.Depth)
	budget := time.Duration(maxNodes) * cfg.TimeLimit
	extendWriteDeadline(w, l, budget+analyzeDeadlineSlack)
	actx, cancel := context.WithTimeout(ctx, budget+analyzeDeadlineSlack/2)
	defer cancel()

	release, err := acquireEngineSlot(actx)
	if err != nil {
		writeEngineBusy(w, r, err)
		return
	}
	defer release()

	res, err := ai.MoveTreeTPS(actx, req.TPS, req.Breadth, req.Depth, cfg)
	if err != nil {
		writeAnalysisError(w, l, analysisErrorStatus(err), err)
		return
	}

	if err := Renderer.JSON(w, http.StatusOK, TreeAnalysisResponse{MoveTree: *res, Level: levelName}); err != nil {
		l.Errorw("failed to render tree analysis", zap.Error(err))
	}
}

func maxTreeNodesFor(breadth, depth int) int {
	if breadth <= 1 {
		return min(1+depth, ai.MaxTreeNodes)
	}
	total := 1
	layer := 1
	for range depth {
		layer *= breadth
		total += layer
		if total >= ai.MaxTreeNodes {
			return ai.MaxTreeNodes
		}
	}
	return min(total, ai.MaxTreeNodes)
}

func decodePositionBody(w http.ResponseWriter, r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("request body is required")
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPositionBody)).Decode(dst); err != nil {
		return errors.New("invalid request body")
	}
	return nil
}

// analysisErrorStatus maps engine failures to HTTP: running out of time is
// the server's problem, anything else is a bad position or move.
func analysisErrorStatus(err error) int {
	if errors.Is(err, ai.ErrNoMove) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadRequest
}

func writeAnalysisError(w http.ResponseWriter, l *zap.SugaredLogger, status int, err error) {
	if status >= http.StatusInternalServerError {
		l.Errorw("analysis failed", zap.Error(err))
	} else {
		l.Warnw("analysis rejected", zap.Error(err))
	}
	if jerr := Renderer.JSON(w, status, ErrorResponse{Error: err.Error()}); jerr != nil {
		l.Errorw("failed to render JSON", zap.Error(jerr))
	}
}
