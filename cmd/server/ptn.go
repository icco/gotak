package main

import (
	"net/http"

	"github.com/icco/gutil/logging"
	"go.uber.org/zap"
)

// @Summary Download game as PTN
// @Description Serialises the game as Portable Tak Notation text.
// @Tags game
// @Produce plain
// @Param slug path string true "Game slug identifier"
// @Success 200 {string} string "PTN text"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /game/{slug}/ptn [get]
func getPTNHandler(w http.ResponseWriter, r *http.Request) {
	l := logging.FromContext(r.Context())

	game, ok := loadGameForRead(w, r, l)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+game.Slug+`.ptn"`)
	// #nosec G705 -- response is served as text/plain with global X-Content-Type-Options: nosniff.
	if _, err := w.Write([]byte(game.PTN())); err != nil {
		l.Errorw("failed to write ptn", "slug", game.Slug, zap.Error(err))
	}
}
