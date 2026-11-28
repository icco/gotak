// Package ai provides AI engines for playing the Tak board game.
package ai

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"

	taktician "github.com/nelhage/taktician/ai"
	"github.com/nelhage/taktician/ai/mcts"
	"github.com/nelhage/taktician/ptn"
	"github.com/nelhage/taktician/tak"
	"go.icco.me/gotak"
)

// Regular expressions for parsing PTN moves (copied from move.go)
var (
	placeRegex = regexp.MustCompile(`^([CSF])?([a-z]\d+)$`)
	moveRegex  = regexp.MustCompile(`^([1-9]*)([a-z]\d+)([<>+\-])(\d*)([CSF])?$`)
)

// Board sizes the engine supports. Taktician packs the board into 64-bit
// bitboards, so 9x9 is out of reach even though the rules allow it.
const (
	MinBoardSize = 3
	MaxBoardSize = 8
)

// DifficultyLevel represents AI strength.
type DifficultyLevel int

// Difficulty levels for the AI, from weakest to strongest.
const (
	Beginner DifficultyLevel = iota
	Intermediate
	Advanced
	Expert
)

// Style represents AI playing style.
type Style string

// Known AI playing styles.
const (
	Aggressive Style = "aggressive"
	Defensive  Style = "defensive"
	Balanced   Style = "balanced"
)

// AIConfig holds configuration for an AI player.
//
//revive:disable-next-line:exported // AIConfig is referenced widely across packages; renaming would be a breaking change.
type AIConfig struct {
	Level       DifficultyLevel
	Style       Style
	TimeLimit   time.Duration
	Personality string // Custom personality name
}

// Engine is the interface for AI move generation.
type Engine interface {
	GetMove(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, error)
	ExplainMove(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, error)
}

// TakticianEngine wraps the Taktician AI library
type TakticianEngine struct{}

// GetMove returns the engine's chosen move for the given game state, formatted as a PTN string.
func (e *TakticianEngine) GetMove(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, error) {
	if g.Board.Size < MinBoardSize || g.Board.Size > MaxBoardSize {
		return "", fmt.Errorf("invalid board size %d: must be between %d and %d", g.Board.Size, MinBoardSize, MaxBoardSize)
	}

	// Convert gotak.Game to tak.Position
	position, err := convertGameToPosition(g)
	if err != nil {
		return "", fmt.Errorf("failed to convert game state: %w", err)
	}

	// Safe conversion of int64 to int (already validated to be in range 3-9)
	boardSize := int(g.Board.Size)

	// Create appropriate AI based on configuration
	var ai taktician.TakPlayer
	switch cfg.Level {
	case Beginner:
		ai = taktician.NewRandom(42)
	case Intermediate:
		ai = taktician.NewMinimax(taktician.MinimaxConfig{
			Size:  boardSize,
			Depth: 3,
		})
	case Advanced:
		ai = taktician.NewMinimax(taktician.MinimaxConfig{
			Size:  boardSize,
			Depth: 5,
		})
	case Expert:
		ai = mcts.NewMonteCarlo(mcts.MCTSConfig{
			Size:  boardSize,
			Limit: cfg.TimeLimit,
			C:     1.4, // Exploration parameter
		})
	}

	// Get move from AI
	move := ai.GetMove(ctx, position)

	// Convert tak.Move to PTN string
	ptnMove, err := convertMoveToString(move, boardSize)
	if err != nil {
		return "", fmt.Errorf("failed to convert move: %w", err)
	}

	return ptnMove, nil
}

// ExplainMove returns a human-readable description of how the engine arrived at its move.
func (e *TakticianEngine) ExplainMove(_ context.Context, _ *gotak.Game, cfg AIConfig) (string, error) {
	return fmt.Sprintf("AI move generated using %v level with %v style", cfg.Level, cfg.Style), nil
}

// convertGameToPosition converts our gotak.Game to Taktician's tak.Position
func convertGameToPosition(g *gotak.Game) (*tak.Position, error) {
	if g == nil {
		return nil, fmt.Errorf("game cannot be nil")
	}
	if g.Board == nil {
		return nil, fmt.Errorf("game board cannot be nil")
	}

	// Validate board size and safe int64 -> int conversion
	if g.Board.Size < MinBoardSize || g.Board.Size > MaxBoardSize {
		return nil, fmt.Errorf("invalid board size %d: must be between %d and %d", g.Board.Size, MinBoardSize, MaxBoardSize)
	}
	if g.Board.Size > int64(math.MaxInt) || g.Board.Size < int64(math.MinInt) {
		return nil, fmt.Errorf("board size %d exceeds platform int size limits", g.Board.Size)
	}

	// Safe conversion of int64 to int (already validated to be within range)
	boardSize := int(g.Board.Size)

	// Create a new position with the same size
	config := tak.Config{
		Size: boardSize,
	}
	position := tak.New(config)

	// Apply moves from game history to build current position
	for _, turn := range g.Turns {
		// Apply each move in the turn
		moves := []*gotak.Move{}
		if turn.First != nil {
			moves = append(moves, turn.First)
		}
		if turn.Second != nil {
			moves = append(moves, turn.Second)
		}

		for _, move := range moves {
			if move == nil {
				continue
			}

			// Convert PTN move string to tak.Move
			takMove, err := convertStringToMove(move.Text, boardSize)
			if err != nil {
				return nil, fmt.Errorf("failed to convert move %s: %w", move.Text, err)
			}

			// Apply move to position
			newPosition, err := position.Move(takMove)
			if err != nil {
				return nil, fmt.Errorf("failed to apply move %s: %w", move.Text, err)
			}
			position = newPosition
		}
	}

	return position, nil
}

// convertMoveToString converts Taktician's tak.Move to PTN string notation
func convertMoveToString(move tak.Move, boardSize int) (string, error) {
	// Validate coordinates
	x := move.X
	y := move.Y
	if x < 0 || y < 0 || int(x) >= boardSize || int(y) >= boardSize {
		return "", fmt.Errorf("invalid move coordinates: %d,%d", x, y)
	}

	switch move.Type {
	case tak.PlaceFlat, tak.PlaceStanding, tak.PlaceCapstone:
	case tak.SlideLeft, tak.SlideRight, tak.SlideUp, tak.SlideDown:
		if move.Slides.Empty() {
			return "", fmt.Errorf("slide at %d,%d has no drops", x, y)
		}
	default:
		return "", fmt.Errorf("unsupported move type: %v", move.Type)
	}

	return ptn.FormatMove(move), nil
}

// convertStringToMove converts PTN move string to Taktician's tak.Move
func convertStringToMove(moveStr string, boardSize int) (tak.Move, error) {

	// Use regular expressions to parse the move
	if placeMatch := placeRegex.FindStringSubmatch(moveStr); placeMatch != nil {
		// Placement move
		stone := placeMatch[1]
		square := placeMatch[2]

		x, y, err := parseSquare(square, boardSize)
		if err != nil {
			return tak.Move{}, err
		}

		switch stone {
		case "":
			return tak.Move{X: x, Y: y, Type: tak.PlaceFlat}, nil
		case "S":
			return tak.Move{X: x, Y: y, Type: tak.PlaceStanding}, nil
		case "C":
			return tak.Move{X: x, Y: y, Type: tak.PlaceCapstone}, nil
		default:
			return tak.Move{}, fmt.Errorf("unknown stone type: %s", stone)
		}
	}

	if moveMatch := moveRegex.FindStringSubmatch(moveStr); moveMatch != nil {
		// Slide move
		countStr := moveMatch[1]
		square := moveMatch[2]
		direction := moveMatch[3]
		dropsStr := moveMatch[4]

		x, y, err := parseSquare(square, boardSize)
		if err != nil {
			return tak.Move{}, err
		}

		count := 1
		if countStr != "" {
			if c, err := strconv.Atoi(countStr); err == nil {
				count = c
			}
		}

		var moveType tak.MoveType
		switch direction {
		case "<":
			moveType = tak.SlideLeft
		case ">":
			moveType = tak.SlideRight
		case "+":
			moveType = tak.SlideUp
		case "-":
			moveType = tak.SlideDown
		default:
			return tak.Move{}, fmt.Errorf("unknown direction: %s", direction)
		}

		// PTN: without drop counts the whole carry lands on the first square.
		drops := []int{count}
		if dropsStr != "" {
			drops = drops[:0]
			total := 0
			for _, dropChar := range dropsStr {
				drop, err := strconv.Atoi(string(dropChar))
				if err != nil || drop < 1 {
					return tak.Move{}, fmt.Errorf("invalid drop count: %s", string(dropChar))
				}
				drops = append(drops, drop)
				total += drop
			}
			if total != count {
				return tak.Move{}, fmt.Errorf("drops %s do not add up to carry %d", dropsStr, count)
			}
		}
		if count < 1 || count > boardSize {
			return tak.Move{}, fmt.Errorf("carry %d is outside 1..%d", count, boardSize)
		}

		return tak.Move{X: x, Y: y, Type: moveType, Slides: tak.MkSlides(drops...)}, nil
	}

	return tak.Move{}, fmt.Errorf("invalid move format: %s", moveStr)
}

// parseSquare converts algebraic notation (a1) to coordinates
func parseSquare(square string, boardSize int) (int8, int8, error) {
	if len(square) < 2 {
		return 0, 0, fmt.Errorf("invalid square: %s", square)
	}

	col := square[0]
	rowStr := square[1:]

	if col < 'a' || col > 'z' {
		return 0, 0, fmt.Errorf("invalid column: %c", col)
	}

	x := int8(col - 'a')
	if int(x) >= boardSize {
		return 0, 0, fmt.Errorf("column out of bounds: %c", col)
	}

	row, err := strconv.Atoi(rowStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid row: %s", rowStr)
	}

	if row < 1 || row > boardSize {
		return 0, 0, fmt.Errorf("row out of bounds: %d", row)
	}
	y := int8(row - 1) // #nosec G115 -- bounded above

	return x, y, nil
}
