package main

import (
	"context"
	"testing"
	"time"

	"github.com/icco/gotak"
	"github.com/icco/gotak/ai"
)

// TestAIIntegration tests the full integration of AI with the game system
func TestAIIntegration(t *testing.T) {
	// Create a 5x5 game
	game, err := gotak.NewGame(5, 1, "integration-test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	engine := &ai.TakticianEngine{}
	ctx := context.Background()

	// Test playing a few moves against the AI
	configs := []ai.AIConfig{
		{Level: ai.Beginner, Style: ai.Balanced, TimeLimit: 2 * time.Second},
		{Level: ai.Intermediate, Style: ai.Balanced, TimeLimit: 5 * time.Second},
	}

	for i, cfg := range configs {
		t.Run(difficultyLevelName(cfg.Level), func(t *testing.T) {
			// Get AI move for empty board
			move, err := engine.GetMove(ctx, game, cfg)
			if err != nil {
				t.Errorf("GetMove() failed: %v", err)
				return
			}

			// Validate the move is reasonable for an empty board
			// Should be a placement move (a1-e5 format)
			if len(move) < 2 {
				t.Errorf("AI generated invalid move: %s", move)
				return
			}

			// Check it's a valid square
			col := move[0]
			if col < 'a' || col > 'e' {
				t.Errorf("AI generated move with invalid column: %s", move)
				return
			}

			// Get explanation
			explanation, err := engine.ExplainMove(ctx, game, cfg)
			if err != nil {
				t.Errorf("ExplainMove() failed: %v", err)
				return
			}

			if explanation == "" {
				t.Errorf("AI provided empty explanation")
			}

			t.Logf("Test %d: AI move = %s, explanation = %s", i, move, explanation)
		})
	}
}

// TestAIPerformance tests that AI responds within reasonable time limits
func TestAIPerformance(t *testing.T) {
	game, err := gotak.NewGame(5, 1, "performance-test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	engine := &ai.TakticianEngine{}

	// Test with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cfg := ai.AIConfig{
		Level:     ai.Beginner, // Use beginner for fast response
		Style:     ai.Balanced,
		TimeLimit: 1 * time.Second,
	}

	start := time.Now()
	move, err := engine.GetMove(ctx, game, cfg)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("AI failed to respond within timeout: %v", err)
		return
	}

	if move == "" {
		t.Errorf("AI returned empty move")
		return
	}

	t.Logf("AI response time: %v, move: %s", elapsed, move)

	// Should respond reasonably quickly for beginner level
	if elapsed > 5*time.Second {
		t.Errorf("AI took too long to respond: %v", elapsed)
	}
}

// TestAIDifferentBoardSizes tests AI works with different board sizes
func TestAIDifferentBoardSizes(t *testing.T) {
	sizes := []int64{4, 5, 6, 7, 8}
	engine := &ai.TakticianEngine{}
	ctx := context.Background()

	for _, size := range sizes {
		t.Run(string(rune('0'+size)), func(t *testing.T) {
			game, err := gotak.NewGame(size, 1, "size-test")
			if err != nil {
				t.Fatalf("Failed to create %dx%d game: %v", size, size, err)
			}

			cfg := ai.AIConfig{
				Level:     ai.Beginner,
				Style:     ai.Balanced,
				TimeLimit: 2 * time.Second,
			}

			move, err := engine.GetMove(ctx, game, cfg)
			if err != nil {
				t.Errorf("AI failed on %dx%d board: %v", size, size, err)
				return
			}

			// Validate move is within board bounds
			if len(move) < 2 {
				t.Errorf("Invalid move format for %dx%d board: %s", size, size, move)
				return
			}

			col := move[0]
			maxCol := 'a' + byte(size-1)
			if col < 'a' || col > maxCol {
				t.Errorf("Move column out of bounds for %dx%d board: %s", size, size, move)
				return
			}

			t.Logf("%dx%d board - AI move: %s", size, size, move)
		})
	}
}

// TestAIGameProgression tests AI can handle games with multiple moves
func TestAIGameProgression(t *testing.T) {
	game, err := gotak.NewGame(5, 1, "progression-test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	engine := &ai.TakticianEngine{}
	ctx := context.Background()
	cfg := ai.AIConfig{
		Level:     ai.Beginner,
		Style:     ai.Balanced,
		TimeLimit: 3 * time.Second,
	}

	// Simulate a few moves in the game
	moves := []string{"a1", "b2", "c3"}

	for i, moveStr := range moves {
		// Add move to game history
		move, err := gotak.NewMove(moveStr)
		if err != nil {
			t.Fatalf("Failed to create move %s: %v", moveStr, err)
		}

		turn := &gotak.Turn{
			Number: int64(i + 1),
			First:  move,
		}
		game.Turns = append(game.Turns, turn)

		// Get AI move for current position
		aiMove, err := engine.GetMove(ctx, game, cfg)
		if err != nil {
			t.Errorf("AI failed after move %d (%s): %v", i+1, moveStr, err)
			return
		}

		if aiMove == "" {
			t.Errorf("AI returned empty move after move %d", i+1)
			return
		}

		// Validate AI doesn't repeat the same square (basic check)
		if aiMove == moveStr {
			t.Errorf("AI repeated the same move: %s", aiMove)
		}

		t.Logf("After move %d (%s), AI suggests: %s", i+1, moveStr, aiMove)
	}
}

// TestAIErrorHandling tests AI handles invalid game states gracefully
func TestAIErrorHandling(t *testing.T) {
	engine := &ai.TakticianEngine{}
	ctx := context.Background()
	cfg := ai.AIConfig{
		Level:     ai.Beginner,
		Style:     ai.Balanced,
		TimeLimit: 2 * time.Second,
	}

	// Test with nil game (should handle gracefully)
	_, err := engine.GetMove(ctx, nil, cfg)
	if err == nil {
		t.Errorf("Expected error with nil game, but got none")
	}

	// Test with very short timeout
	shortCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	cancel() // Cancel immediately

	game, err := gotak.NewGame(5, 1, "error-test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	_, err = engine.GetMove(shortCtx, game, cfg)
	// This should either succeed quickly or return a timeout error
	// We don't require it to fail since beginner AI is very fast
	t.Logf("Short timeout result: %v", err)
}

// Helper function to convert difficulty level to string for test names
func difficultyLevelName(d ai.DifficultyLevel) string {
	switch d {
	case ai.Beginner:
		return "Beginner"
	case ai.Intermediate:
		return "Intermediate"
	case ai.Advanced:
		return "Advanced"
	case ai.Expert:
		return "Expert"
	default:
		return "Unknown"
	}
}
