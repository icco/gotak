package ai

import (
	"context"
	"fmt"
	"time"

	"github.com/icco/gotak"
	taktician "github.com/nelhage/taktician/ai"
	"github.com/nelhage/taktician/ai/mcts"
	"github.com/nelhage/taktician/tak"
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
	GetMove(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, error)
	ExplainMove(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, error)
}

// StubEngine is a placeholder AI engine for development.
type StubEngine struct{}

func (e *StubEngine) GetMove(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, error) {
	return "a1", nil
}

func (e *StubEngine) ExplainMove(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, error) {
	return "This is a stub explanation.", nil
}

// TakticianEngine wraps the Taktician AI library
type TakticianEngine struct{}

func (e *TakticianEngine) GetMove(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, error) {
	// Convert gotak.Game to tak.Position
	position, err := convertGameToPosition(g)
	if err != nil {
		return "", fmt.Errorf("failed to convert game state: %w", err)
	}

	// Create appropriate AI based on configuration
	var ai taktician.TakPlayer
	switch cfg.Level {
	case Beginner:
		ai = taktician.NewRandom(42)
	case Intermediate:
		ai = taktician.NewMinimax(taktician.MinimaxConfig{
			Size:  int(g.Board.Size),
			Depth: 3,
		})
	case Advanced:
		ai = taktician.NewMinimax(taktician.MinimaxConfig{
			Size:  int(g.Board.Size),
			Depth: 5,
		})
	case Expert:
		ai = mcts.NewMonteCarlo(mcts.MCTSConfig{
			Size:  int(g.Board.Size),
			Limit: cfg.TimeLimit,
			C:     1.4, // Exploration parameter
		})
	}

	// Get move from AI
	move := ai.GetMove(ctx, position)
	
	// Convert tak.Move to PTN string
	ptnMove, err := convertMoveToString(move, int(g.Board.Size))
	if err != nil {
		return "", fmt.Errorf("failed to convert move: %w", err)
	}

	return ptnMove, nil
}

func (e *TakticianEngine) ExplainMove(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, error) {
	return fmt.Sprintf("AI move generated using %v level with %v style", cfg.Level, cfg.Style), nil
}

// convertGameToPosition converts our gotak.Game to Taktician's tak.Position
func convertGameToPosition(g *gotak.Game) (*tak.Position, error) {
	// TODO: Implement conversion from gotak representation to tak.Position
	// This is a complex conversion that needs to map:
	// - Board state and pieces
	// - Move history
	// - Current player
	// - Piece counts
	
	// For now, create a new position with the same size
	config := tak.Config{
		Size: int(g.Board.Size),
	}
	position := tak.New(config)
	
	// TODO: Apply moves from game history to build current position
	// This would require converting each PTN move to tak.Move and applying it
	
	return position, nil
}

// convertMoveToString converts Taktician's tak.Move to PTN string notation
func convertMoveToString(move tak.Move, boardSize int) (string, error) {
	// TODO: Implement conversion from tak.Move to PTN notation
	// This needs to handle:
	// - Placement moves (a1, Ca1, Sa1)
	// - Slide moves (3a3+3, 4a4>121)
	// - Coordinate conversion (Taktician uses int8 x,y, PTN uses algebraic notation)
	
	// For now, return a simple placeholder
	x := move.X
	y := move.Y
	
	// Convert coordinates to algebraic notation (a1, b2, etc.)
	if x < 0 || y < 0 || int(x) >= boardSize || int(y) >= boardSize {
		return "", fmt.Errorf("invalid move coordinates: %d,%d", x, y)
	}
	
	col := string(rune('a' + x))
	row := fmt.Sprintf("%d", y+1)
	
	// TODO: Handle different move types properly
	return col + row, nil
}
