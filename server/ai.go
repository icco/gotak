package server

import (
	"context"
	"net/http"
	"time"

	"workspaces/gotak/ai"
	"workspaces/gotak/game"
)

// AIRequest represents a request for an AI move.
type AIRequest struct {
	Level      string        `json:"level"`
	Style      string        `json:"style"`
	TimeLimit  time.Duration `json:"time_limit"`
	Personality string       `json:"personality"`
}

// AIMoveResponse is the response for an AI move.
type AIMoveResponse struct {
	Move  string `json:"move"`
	Hint  string `json:"hint,omitempty"`
}

// PostAIMoveHandler handles AI move requests.
func PostAIMoveHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// TODO: Get game by slug, parse AIRequest from body
	// For now, use stub game and config
	var req AIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid request"}`))
		return
	}

	// TODO: Map string level/style to ai.DifficultyLevel/ai.Style
	cfg := ai.AIConfig{
		Level: ai.Beginner,
		Style: ai.Balanced,
		TimeLimit: time.Second,
		Personality: req.Personality,
	}
	g := &game.Game{} // TODO: Load actual game
	engine := &ai.StubEngine{}
	move, err := engine.GetMove(ctx, g, cfg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"AI move failed"}`))
		return
	}
	hint, _ := engine.ExplainMove(ctx, g, cfg)
	resp := AIMoveResponse{Move: move, Hint: hint}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
