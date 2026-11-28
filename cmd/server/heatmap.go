package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.icco.me/gotak"
	"go.icco.me/gutil/logging"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// PieceCounts holds placement counts broken down by stone type.
type PieceCounts struct {
	Flat     int `json:"flat"`
	Standing int `json:"standing"`
	Capstone int `json:"capstone"`
}

// SquareCounts holds placement counts on a square split by player color.
type SquareCounts struct {
	White PieceCounts `json:"white"`
	Black PieceCounts `json:"black"`
}

// HeatmapResponse contains aggregate piece placement counts across games of a given board size.
type HeatmapResponse struct {
	Size       int                     `json:"size"`
	GameCount  int                     `json:"game_count"`
	Placements int                     `json:"placements"`
	Squares    map[string]SquareCounts `json:"squares"`
}

// @Summary Look up piece placement heatmap
// @Description Returns per-square piece placement counts across all stored games of
// @Description the given board size, split by player color and stone type.
// @Description Turn 1 placements in Tak place the opponent's piece, so turn 1 placements
// @Description are attributed to the stone's color (the opponent).
// @Tags analysis
// @Accept json
// @Produce json
// @Param size query int true "Board size (4-9)" example(6)
// @Success 200 {object} HeatmapResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analyze/heatmap [get]
func getHeatmapHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	if _, err := parseHeatmapSize(r.URL.Query().Get("size")); err != nil {
		l.Warnw("invalid heatmap size", zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()}); jerr != nil {
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

	getHeatmapWithDB(w, r, db)
}

func getHeatmapWithDB(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	size, err := parseHeatmapSize(r.URL.Query().Get("size"))
	if err != nil {
		l.Warnw("invalid heatmap size", zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	resp, err := computeHeatmap(db, size)
	if err != nil {
		l.Errorw("could not compute heatmap", zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "could not compute heatmap"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	if err := Renderer.JSON(w, http.StatusOK, resp); err != nil {
		l.Errorw("failed to render heatmap response", zap.Error(err))
	}
}

func parseHeatmapSize(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, fmt.Errorf("size parameter is required")
	}
	size, err := strconv.Atoi(trimmed)
	if err != nil || size < 4 || size > 9 {
		return 0, fmt.Errorf("size must be an integer between 4 and 9")
	}
	return size, nil
}

func computeHeatmap(db *gorm.DB, size int) (*HeatmapResponse, error) {
	sizeStr := strconv.Itoa(size)

	var gameCount int64
	if err := db.Model(&Tag{}).
		Where("key = ? AND value = ?", "Size", sizeStr).
		Distinct("game_id").
		Count(&gameCount).Error; err != nil {
		return nil, fmt.Errorf("count games by size: %w", err)
	}

	subquery := db.Model(&Tag{}).
		Select("game_id").
		Where("key = ? AND value = ?", "Size", sizeStr)

	var rows []Move
	if err := db.Model(&Move{}).
		Where("game_id IN (?)", subquery).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("find moves for size: %w", err)
	}

	squares := make(map[string]SquareCounts, size*size)
	for col := range size {
		colLetter := string(rune('a' + col))
		for row := 1; row <= size; row++ {
			sqName := fmt.Sprintf("%s%d", colLetter, row)
			squares[sqName] = SquareCounts{}
		}
	}

	totalPlacements := 0
	for _, r := range rows {
		mv, err := gotak.NewMove(r.Text)
		if err != nil {
			continue
		}
		if mv.MoveDirection != "" {
			continue
		}

		sqCounts, ok := squares[mv.Square]
		if !ok {
			continue
		}

		color := r.Player
		if r.Turn == 1 {
			if r.Player == gotak.PlayerWhite {
				color = gotak.PlayerBlack
			} else if r.Player == gotak.PlayerBlack {
				color = gotak.PlayerWhite
			}
		}

		var pieces *PieceCounts
		switch color {
		case gotak.PlayerWhite:
			pieces = &sqCounts.White
		case gotak.PlayerBlack:
			pieces = &sqCounts.Black
		default:
			continue
		}

		switch mv.Stone {
		case gotak.StoneFlat:
			pieces.Flat++
		case gotak.StoneStanding:
			pieces.Standing++
		case gotak.StoneCap:
			pieces.Capstone++
		default:
			continue
		}

		squares[mv.Square] = sqCounts
		totalPlacements++
	}

	return &HeatmapResponse{
		Size:       size,
		GameCount:  int(gameCount),
		Placements: totalPlacements,
		Squares:    squares,
	}, nil
}
