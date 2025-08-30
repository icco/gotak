package ai

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"
	"math"

	"github.com/icco/gotak"
	taktician "github.com/nelhage/taktician/ai"
	"github.com/nelhage/taktician/ai/mcts"
	"github.com/nelhage/taktician/tak"
)

// Regular expressions for parsing PTN moves (copied from move.go)
var (
	placeRegex = regexp.MustCompile(`^([CSF])?([a-z]\d+)$`)
	moveRegex  = regexp.MustCompile(`^([1-9]*)([a-z]\d+)([<>+\-])(\d*)([CSF])?$`)
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

func (e *TakticianEngine) GetMove(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, error) {
	// Convert gotak.Game to tak.Position
	position, err := convertGameToPosition(g)
	if err != nil {
		return "", fmt.Errorf("failed to convert game state: %w", err)
	}

	// Ensure board size is within expected bounds (Tak standard: 3 - 9)
	if g.Board.Size < 3 || g.Board.Size > 9 {
		return "", fmt.Errorf("invalid board size %d: must be between 3 and 9", g.Board.Size)
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

func (e *TakticianEngine) ExplainMove(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, error) {
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
	if g.Board.Size < 3 || g.Board.Size > 9 {
		return nil, fmt.Errorf("invalid board size %d: must be between 3 and 9", g.Board.Size)
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

	// Convert coordinates to algebraic notation (a1, b2, etc.)
	col := string(rune('a' + x))
	row := fmt.Sprintf("%d", y+1)
	square := col + row

	switch move.Type {
	case tak.PlaceFlat:
		return square, nil
	case tak.PlaceStanding:
		return "S" + square, nil
	case tak.PlaceCapstone:
		return "C" + square, nil
	case tak.SlideLeft, tak.SlideRight, tak.SlideUp, tak.SlideDown:
		// Handle slide moves
		direction := ""
		switch move.Type {
		case tak.SlideLeft:
			direction = "<"
		case tak.SlideRight:
			direction = ">"
		case tak.SlideUp:
			direction = "+"
		case tak.SlideDown:
			direction = "-"
		}

		// Build move string: (count)(square)(direction)(drops)
		count := move.Slides.Len()
		moveStr := fmt.Sprintf("%d%s%s", count, square, direction)

		// Add drop counts if needed
		if !move.Slides.Empty() {
			drops := ""
			it := move.Slides.Iterator()
			for it.Ok() {
				drops += fmt.Sprintf("%d", it.Elem())
				it = it.Next()
			}
			moveStr += drops
		}

		return moveStr, nil
	default:
		return "", fmt.Errorf("unsupported move type: %v", move.Type)
	}
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

		// Parse drop counts
		var slides tak.Slides
		if dropsStr != "" {
			drops := []int{}
			for _, dropChar := range dropsStr {
				drop, err := strconv.Atoi(string(dropChar))
				if err != nil {
					return tak.Move{}, fmt.Errorf("invalid drop count: %s", string(dropChar))
				}
				drops = append(drops, drop)
			}
			slides = tak.MkSlides(drops...)
		} else {
			// Default: drop all stones one by one
			drops := make([]int, count)
			for i := 0; i < count; i++ {
				drops[i] = 1
			}
			slides = tak.MkSlides(drops...)
		}

		return tak.Move{X: x, Y: y, Type: moveType, Slides: slides}, nil
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

	y := int8(row - 1) // Convert to 0-based
	if y < 0 || int(y) >= boardSize {
		return 0, 0, fmt.Errorf("row out of bounds: %d", row)
	}

	return x, y, nil
}
