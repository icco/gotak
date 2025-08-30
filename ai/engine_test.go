package ai

import (
	"context"
	"testing"
	"time"

	"github.com/icco/gotak"
	"github.com/nelhage/taktician/tak"
)

func TestConvertMoveToString(t *testing.T) {
	tests := []struct {
		name     string
		move     tak.Move
		size     int
		expected string
		wantErr  bool
	}{
		{
			name:     "place flat stone",
			move:     tak.Move{X: 0, Y: 0, Type: tak.PlaceFlat},
			size:     5,
			expected: "a1",
		},
		{
			name:     "place standing stone",
			move:     tak.Move{X: 2, Y: 3, Type: tak.PlaceStanding},
			size:     5,
			expected: "Sc4",
		},
		{
			name:     "place capstone",
			move:     tak.Move{X: 4, Y: 4, Type: tak.PlaceCapstone},
			size:     5,
			expected: "Ce5",
		},
		{
			name:     "slide right",
			move:     tak.Move{X: 1, Y: 1, Type: tak.SlideRight, Slides: tak.MkSlides(1, 2)},
			size:     5,
			expected: "2b2>12",
		},
		{
			name:     "slide up",
			move:     tak.Move{X: 0, Y: 0, Type: tak.SlideUp, Slides: tak.MkSlides(3)},
			size:     5,
			expected: "1a1+3",
		},
		{
			name:     "invalid coordinates - out of bounds",
			move:     tak.Move{X: 5, Y: 0, Type: tak.PlaceFlat},
			size:     5,
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertMoveToString(tt.move, tt.size)
			if tt.wantErr {
				if err == nil {
					t.Errorf("convertMoveToString() expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("convertMoveToString() error = %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("convertMoveToString() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConvertStringToMove(t *testing.T) {
	tests := []struct {
		name     string
		moveStr  string
		size     int
		expected tak.Move
		wantErr  bool
	}{
		{
			name:     "place flat stone",
			moveStr:  "a1",
			size:     5,
			expected: tak.Move{X: 0, Y: 0, Type: tak.PlaceFlat},
		},
		{
			name:     "place standing stone",
			moveStr:  "Sc4",
			size:     5,
			expected: tak.Move{X: 2, Y: 3, Type: tak.PlaceStanding},
		},
		{
			name:     "place capstone",
			moveStr:  "Ce5",
			size:     5,
			expected: tak.Move{X: 4, Y: 4, Type: tak.PlaceCapstone},
		},
		{
			name:     "slide right with drops",
			moveStr:  "2b2>12",
			size:     5,
			expected: tak.Move{X: 1, Y: 1, Type: tak.SlideRight, Slides: tak.MkSlides(1, 2)},
		},
		{
			name:     "slide up simple",
			moveStr:  "3a1+",
			size:     5,
			expected: tak.Move{X: 0, Y: 0, Type: tak.SlideUp, Slides: tak.MkSlides(1, 1, 1)},
		},
		{
			name:    "invalid move format",
			moveStr: "invalid",
			size:    5,
			wantErr: true,
		},
		{
			name:    "out of bounds square",
			moveStr: "z9",
			size:    5,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertStringToMove(tt.moveStr, tt.size)
			if tt.wantErr {
				if err == nil {
					t.Errorf("convertStringToMove() expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("convertStringToMove() error = %v", err)
				return
			}
			if result.X != tt.expected.X || result.Y != tt.expected.Y || result.Type != tt.expected.Type {
				t.Errorf("convertStringToMove() = %+v, want %+v", result, tt.expected)
			}
			// For slide moves, check slides match
			if result.Type >= tak.SlideLeft && !result.Slides.Empty() {
				if result.Slides != tt.expected.Slides {
					t.Errorf("convertStringToMove() slides = %v, want %v", result.Slides, tt.expected.Slides)
				}
			}
		})
	}
}

func TestParseSquare(t *testing.T) {
	tests := []struct {
		name      string
		square    string
		boardSize int
		expectedX int8
		expectedY int8
		wantErr   bool
	}{
		{"a1", "a1", 5, 0, 0, false},
		{"c3", "c3", 5, 2, 2, false},
		{"e5", "e5", 5, 4, 4, false},
		{"invalid short", "a", 5, 0, 0, true},
		{"invalid column", "z1", 5, 0, 0, true},
		{"invalid row", "a0", 5, 0, 0, true},
		{"out of bounds", "f1", 5, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, err := parseSquare(tt.square, tt.boardSize)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseSquare() expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("parseSquare() error = %v", err)
				return
			}
			if x != tt.expectedX || y != tt.expectedY {
				t.Errorf("parseSquare() = (%v, %v), want (%v, %v)", x, y, tt.expectedX, tt.expectedY)
			}
		})
	}
}

func TestTakticianEngineGetMove(t *testing.T) {
	// Create a simple 5x5 game
	game, err := gotak.NewGame(5, 1, "test-ai")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	engine := &TakticianEngine{}
	ctx := context.Background()

	// Test different difficulty levels
	levels := []struct {
		name  string
		level DifficultyLevel
	}{
		{"Beginner", Beginner},
		{"Intermediate", Intermediate},
		{"Advanced", Advanced},
		{"Expert", Expert},
	}

	for _, tt := range levels {
		t.Run(tt.name, func(t *testing.T) {
			cfg := AIConfig{
				Level:     tt.level,
				Style:     Balanced,
				TimeLimit: 2 * time.Second,
			}

			move, err := engine.GetMove(ctx, game, cfg)
			if err != nil {
				t.Errorf("GetMove() error = %v", err)
				return
			}

			// Basic validation - should be a valid PTN move
			if len(move) < 2 {
				t.Errorf("GetMove() returned invalid move length: %s", move)
			}

			// Should be a valid square format (basic check)
			if move[len(move)-1] < '1' || move[len(move)-1] > '8' {
				t.Errorf("GetMove() returned move with invalid row: %s", move)
			}
			if move[0] < 'a' || move[0] > 'h' {
				t.Errorf("GetMove() returned move with invalid column: %s", move)
			}

			t.Logf("AI Level %s generated move: %s", tt.name, move)
		})
	}
}

func TestTakticianEngineExplainMove(t *testing.T) {
	game, err := gotak.NewGame(5, 1, "test-explain")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	engine := &TakticianEngine{}
	cfg := AIConfig{
		Level:     Intermediate,
		Style:     Balanced,
		TimeLimit: 1 * time.Second,
	}

	ctx := context.Background()
	explanation, err := engine.ExplainMove(ctx, game, cfg)
	if err != nil {
		t.Errorf("ExplainMove() error = %v", err)
		return
	}

	if explanation == "" {
		t.Errorf("ExplainMove() returned empty explanation")
	}

	if len(explanation) < 10 {
		t.Errorf("ExplainMove() returned very short explanation: %s", explanation)
	}

	t.Logf("AI explanation: %s", explanation)
}

func TestConvertGameToPosition(t *testing.T) {
	// Test with empty game
	game, err := gotak.NewGame(5, 1, "test-convert")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	position, err := convertGameToPosition(game)
	if err != nil {
		t.Errorf("convertGameToPosition() error = %v", err)
		return
	}

	if position.Size() != 5 {
		t.Errorf("convertGameToPosition() size = %v, want 5", position.Size())
	}

	// Test with game that has some moves
	// Add a simple move to the game
	move1, err := gotak.NewMove("a1")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	turn := &gotak.Turn{
		Number: 1,
		First:  move1,
	}
	game.Turns = append(game.Turns, turn)

	position2, err := convertGameToPosition(game)
	if err != nil {
		t.Errorf("convertGameToPosition() with moves error = %v", err)
		return
	}

	if position2.Size() != 5 {
		t.Errorf("convertGameToPosition() with moves size = %v, want 5", position2.Size())
	}

	// The position should have advanced by one move
	if position2.MoveNumber() != 1 {
		t.Errorf("convertGameToPosition() move number = %v, want 1", position2.MoveNumber())
	}
}

func TestAIConfigurationOptions(t *testing.T) {
	game, err := gotak.NewGame(6, 1, "test-config")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	engine := &TakticianEngine{}
	ctx := context.Background()

	// Test different styles
	styles := []Style{Aggressive, Defensive, Balanced}
	for _, style := range styles {
		cfg := AIConfig{
			Level:       Beginner, // Use beginner for faster tests
			Style:       style,
			TimeLimit:   1 * time.Second,
			Personality: "test-" + string(style),
		}

		move, err := engine.GetMove(ctx, game, cfg)
		if err != nil {
			t.Errorf("GetMove() with style %v error = %v", style, err)
			continue
		}

		if move == "" {
			t.Errorf("GetMove() with style %v returned empty move", style)
		}

		t.Logf("Style %v generated move: %s", style, move)
	}
}

func TestRoundTripConversion(t *testing.T) {
	// Test that converting a move to string and back gives the same result
	testMoves := []string{
		"a1",
		"Sa1",
		"Ca1",
		"b2",
		"2c3>11",
		"3d4+111",
		"1a1<1",
	}

	for _, moveStr := range testMoves {
		t.Run(moveStr, func(t *testing.T) {
			// Convert string to move
			takMove, err := convertStringToMove(moveStr, 5)
			if err != nil {
				t.Errorf("convertStringToMove() error = %v", err)
				return
			}

			// Convert back to string
			resultStr, err := convertMoveToString(takMove, 5)
			if err != nil {
				t.Errorf("convertMoveToString() error = %v", err)
				return
			}

			// For simple placement moves, they should be identical
			// For slide moves, the format might be normalized but should be equivalent
			if takMove.Type == tak.PlaceFlat || takMove.Type == tak.PlaceStanding || takMove.Type == tak.PlaceCapstone {
				if resultStr != moveStr {
					t.Errorf("Round trip conversion failed: %s -> %s", moveStr, resultStr)
				}
			} else {
				// For slide moves, at least verify the coordinates match
				takMove2, err := convertStringToMove(resultStr, 5)
				if err != nil {
					t.Errorf("Second convertStringToMove() error = %v", err)
					return
				}
				if takMove.X != takMove2.X || takMove.Y != takMove2.Y || takMove.Type != takMove2.Type {
					t.Errorf("Round trip slide move conversion failed: coordinates or type mismatch")
				}
			}
		})
	}
}
