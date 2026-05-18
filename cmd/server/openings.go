package main

import (
	"net/http"
	"sort"
	"strings"

	"github.com/icco/gotak"
	"github.com/icco/gutil/logging"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type OpeningContinuation struct {
	Move  string `json:"move"`
	Count int    `json:"count"`
}

// OpeningsResponse continuations are sorted by Count desc, then Move asc.
type OpeningsResponse struct {
	Prefix        []string              `json:"prefix"`
	GameCount     int                   `json:"game_count"`
	Continuations []OpeningContinuation `json:"continuations"`
}

// @Summary Look up opening continuations
// @Description Given a prefix of moves (in PTN order — White on turn 1
// @Description first, Black second, then alternating), returns the count
// @Description of stored games matching the prefix and the frequency of
// @Description moves played in the next half-turn. Empty prefix lists
// @Description first-move distribution across all games.
// @Tags analysis
// @Accept json
// @Produce json
// @Param prefix query string false "Comma-separated PTN moves" example("a1,e5")
// @Success 200 {object} OpeningsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analyze/openings [get]
func getOpeningsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	prefix, err := parseOpeningPrefix(r.URL.Query().Get("prefix"))
	if err != nil {
		l.Warnw("invalid opening prefix", zap.Error(err))
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

	count, conts, err := computeOpenings(db, prefix)
	if err != nil {
		l.Errorw("could not compute openings", zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "could not compute openings"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	resp := OpeningsResponse{
		Prefix:        prefix,
		GameCount:     count,
		Continuations: conts,
	}
	if err := Renderer.JSON(w, http.StatusOK, resp); err != nil {
		l.Errorw("failed to render openings response", zap.Error(err))
	}
}

// parseOpeningPrefix returns moves canonicalised through gotak.NewMove
// so PTN annotations (`!`, `?`) match how the DB stores them.
func parseOpeningPrefix(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		mv, err := gotak.NewMove(p)
		if err != nil {
			return nil, err
		}
		out = append(out, mv.Text)
	}
	return out, nil
}

// computeOpenings does the aggregation in Go; move to a SQL CTE once
// the Move table grows past a few thousand rows.
func computeOpenings(db *gorm.DB, prefix []string) (int, []OpeningContinuation, error) {
	var rows []Move
	if err := db.Model(&Move{}).
		Order("game_id ASC, turn ASC, player ASC").
		Find(&rows).Error; err != nil {
		return 0, nil, err
	}

	byGame := map[int64][]string{}
	for _, r := range rows {
		byGame[r.GameID] = append(byGame[r.GameID], r.Text)
	}

	count := 0
	contCounts := map[string]int{}
	for _, moves := range byGame {
		if len(moves) < len(prefix) {
			continue
		}
		match := true
		for i, p := range prefix {
			if moves[i] != p {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		count++
		if len(moves) > len(prefix) {
			contCounts[moves[len(prefix)]]++
		}
	}

	conts := make([]OpeningContinuation, 0, len(contCounts))
	for m, c := range contCounts {
		conts = append(conts, OpeningContinuation{Move: m, Count: c})
	}
	sort.Slice(conts, func(i, j int) bool {
		if conts[i].Count != conts[j].Count {
			return conts[i].Count > conts[j].Count
		}
		return conts[i].Move < conts[j].Move
	})
	return count, conts, nil
}
