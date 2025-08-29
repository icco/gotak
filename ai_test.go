package gotak

import (
	"testing"
)

func TestTakticianEngine(t *testing.T) {
	// Create a simple 5x5 game for testing
	game, err := NewGame(5, 1, "test-ai")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Test basic AI functionality
	// For now, just test that we can create a game
	if game.Board.Size != 5 {
		t.Errorf("Expected board size 5, got %d", game.Board.Size)
	}

	t.Logf("AI integration test placeholder - game created successfully")
}

// TODO: Add proper AI integration tests when import cycle is resolved