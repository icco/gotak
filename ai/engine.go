package ai

import (
	"context"
	"time"
	"workspaces/gotak/game"
)

// DifficultyLevel represents AI strength.
type DifficultyLevel int

const (
	Beginner DifficultyLevel = iota
	Intermediate
	Advanced
	Expert
)

// Style represents AI playing style.
type Style string

const (
	Aggressive Style = "aggressive"
	Defensive  Style = "defensive"
	Balanced   Style = "balanced"
)

// AIConfig holds configuration for an AI player.
type AIConfig struct {
	Level      DifficultyLevel
	Style      Style
	TimeLimit  time.Duration
	Personality string // Custom personality name
}

// Engine is the interface for AI move generation.
type Engine interface {
	GetMove(ctx context.Context, g *game.Game, cfg AIConfig) (string, error)
	ExplainMove(ctx context.Context, g *game.Game, cfg AIConfig) (string, error)
}

// StubEngine is a placeholder AI engine for development.
type StubEngine struct{}

func (e *StubEngine) GetMove(ctx context.Context, g *game.Game, cfg AIConfig) (string, error) {
	// TODO: Integrate real AI logic
	return "a1", nil
}

func (e *StubEngine) ExplainMove(ctx context.Context, g *game.Game, cfg AIConfig) (string, error) {
	// TODO: Provide move explanation
	return "This is a stub explanation.", nil
}
