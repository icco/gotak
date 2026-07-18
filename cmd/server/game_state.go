package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/icco/gotak"
	"gorm.io/gorm"
)

// GameStateResponse is the enriched game payload returned to API clients.
// It embeds gotak.Game so existing fields (ID, Slug, Board, Turns, Meta)
// keep their established JSON shape, and adds session fields the mobile
// client needs without a second round-trip.
type GameStateResponse struct {
	*gotak.Game
	CurrentPlayer int    `json:"current_player"`
	Status        string `json:"status"`
	Winner        int    `json:"winner"`
	WhitePlayerID *int64 `json:"white_player_id,omitempty"`
	BlackPlayerID *int64 `json:"black_player_id,omitempty"`
	Mode          string `json:"mode"`
}

var errInvalidGameMode = errors.New(`invalid mode: must be "human" or "ai"`)

// wantsJSON reports whether the client prefers a JSON body over a redirect.
func wantsJSON(r *http.Request) bool {
	if r.URL.Query().Get("format") == "json" {
		return true
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

// normalizeGameMode returns "human" or "ai". Empty input defaults to human.
func normalizeGameMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "human":
		return "human", nil
	case "ai":
		return "ai", nil
	default:
		return "", errInvalidGameMode
	}
}

// buildGameStateResponse loads the full game plus DB session fields.
func buildGameStateResponse(db *gorm.DB, slug string) (*GameStateResponse, error) {
	game, err := getGame(db, slug)
	if err != nil {
		return nil, err
	}

	var dbGame Game
	if err := db.Where("slug = ?", slug).First(&dbGame).Error; err != nil {
		return nil, err
	}

	mode := "human"
	for _, tag := range game.Meta {
		if tag != nil && tag.Key == "Mode" && tag.Value != "" {
			mode = tag.Value
			break
		}
	}

	return &GameStateResponse{
		Game:          game,
		CurrentPlayer: dbGame.CurrentPlayer,
		Status:        dbGame.Status,
		Winner:        dbGame.Winner,
		WhitePlayerID: dbGame.WhitePlayerID,
		BlackPlayerID: dbGame.BlackPlayerID,
		Mode:          mode,
	}, nil
}
