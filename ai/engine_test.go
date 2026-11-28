package ai

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nelhage/taktician/tak"
	"go.icco.me/gotak"
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
			expected: "3b2>12",
		},
		{
			name:     "slide up",
			move:     tak.Move{X: 0, Y: 0, Type: tak.SlideUp, Slides: tak.MkSlides(3)},
			size:     5,
			expected: "3a1+",
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
			moveStr:  "3b2>12",
			size:     5,
			expected: tak.Move{X: 1, Y: 1, Type: tak.SlideRight, Slides: tak.MkSlides(1, 2)},
		},
		{
			name:     "slide up simple",
			moveStr:  "3a1+",
			size:     5,
			expected: tak.Move{X: 0, Y: 0, Type: tak.SlideUp, Slides: tak.MkSlides(3)},
		},
		{
			name:     "single stone slide",
			moveStr:  "a1>",
			size:     5,
			expected: tak.Move{X: 0, Y: 0, Type: tak.SlideRight, Slides: tak.MkSlides(1)},
		},
		{
			name:    "drops do not match carry",
			moveStr: "2b2>12",
			size:    5,
			wantErr: true,
		},
		{
			name:    "carry larger than board",
			moveStr: "6a1>",
			size:    5,
			wantErr: true,
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
	tests := []struct {
		in   string
		want string
	}{
		{"a1", "a1"},
		{"Sa1", "Sa1"},
		{"Ca1", "Ca1"},
		{"b2", "b2"},
		{"a1<", "a1<"},
		{"1a1<1", "a1<"},
		{"3b2-", "3b2-"},
		{"3b2-3", "3b2-"},
		{"2c3>11", "2c3>11"},
		{"3d4+12", "3d4+12"},
		{"5a1>212", "5a1>212"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			takMove, err := convertStringToMove(tt.in, 5)
			if err != nil {
				t.Fatalf("convertStringToMove(%q) error = %v", tt.in, err)
			}
			got, err := convertMoveToString(takMove, 5)
			if err != nil {
				t.Fatalf("convertMoveToString() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("round trip %q = %q, want %q", tt.in, got, tt.want)
			}
			if _, err := gotak.NewMove(got); err != nil {
				t.Errorf("gotak rejects engine output %q: %v", got, err)
			}
		})
	}
}

func TestTakticianEngineRejectsUnsupportedSize(t *testing.T) {
	game, err := gotak.NewGame(9, 1, "test-nine")
	if err != nil {
		t.Fatalf("NewGame(9): %v", err)
	}
	_, err = (&TakticianEngine{}).GetMove(context.Background(), game, AIConfig{Level: Intermediate})
	if err == nil {
		t.Fatal("GetMove on a 9x9 board should return an error, not a move")
	}
}

func TestTakticianEngineSlidesStayInSync(t *testing.T) {
	game, err := gotak.NewGame(5, 1, "test-slide")
	if err != nil {
		t.Fatalf("NewGame: %v", err)
	}
	if err := game.DoTurn("a1", "e5"); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if err := game.DoTurn("b1", "a2"); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if err := game.DoTurn("b1<", "a2-"); err != nil {
		t.Fatalf("turn 3: %v", err)
	}
	if err := game.DoTurn("c3", "3a1>"); err != nil {
		t.Fatalf("turn 4: %v", err)
	}

	pos, err := convertGameToPosition(game)
	if err != nil {
		t.Fatalf("convertGameToPosition: %v", err)
	}
	if h := pos.At(1, 0); len(h) != 3 {
		t.Errorf("b1 height in engine = %d, want 3", len(h))
	}
	if h := pos.At(0, 0); len(h) != 0 {
		t.Errorf("a1 height in engine = %d, want 0", len(h))
	}
}

func TestTakticianEngineConcurrentMoves(t *testing.T) {
	game, err := gotak.NewGame(5, 1, "alice-vs-bob")
	if err != nil {
		t.Fatalf("NewGame: %v", err)
	}
	if err := game.DoTurn("a1", "e5"); err != nil {
		t.Fatalf("DoTurn: %v", err)
	}
	engine := &TakticianEngine{}
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			mv, err := engine.GetMove(context.Background(), game, AIConfig{Level: Intermediate})
			if err == nil && mv == "" {
				err = fmt.Errorf("empty move")
			}
			errs <- err
		}()
	}
	for range 8 {
		if err := <-errs; err != nil {
			t.Errorf("GetMove: %v", err)
		}
	}
}

func TestExpertLimit(t *testing.T) {
	cases := []struct{ in, want time.Duration }{
		{0, time.Second},
		{-time.Second, time.Second},
		{300 * time.Millisecond, time.Second},
		{2400 * time.Millisecond, 2 * time.Second},
		{10 * time.Second, 10 * time.Second},
		{time.Hour, 30 * time.Second},
	}
	for _, c := range cases {
		if got := expertLimit(c.in); got != c.want {
			t.Errorf("expertLimit(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
