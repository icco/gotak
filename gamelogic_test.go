package gotak

import (
	"testing"
)

// Test GameOver function with road wins
func TestGameOverRoadWin(t *testing.T) {
	// Test horizontal road win
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Place stones to create a horizontal road for white
	moves := []struct {
		square string
		player int
	}{
		{"a1", PlayerWhite},
		{"b1", PlayerWhite},
		{"c1", PlayerWhite},
		{"d1", PlayerWhite},
		{"e1", PlayerWhite},
	}

	for _, move := range moves {
		stone := &Stone{
			Player: move.player,
			Type:   StoneFlat,
		}
		game.Board.Squares[move.square] = []*Stone{stone}
	}

	winner, gameOver := game.GameOver()
	if !gameOver {
		t.Errorf("Expected game to be over, but it wasn't")
	}
	if winner != PlayerWhite {
		t.Errorf("Expected PlayerWhite (%d) to win, got %d", PlayerWhite, winner)
	}

	// Test vertical road win
	game2, err := NewGame(5, 2, "test2")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Place stones to create a vertical road for black
	moves2 := []struct {
		square string
		player int
	}{
		{"a1", PlayerBlack},
		{"a2", PlayerBlack},
		{"a3", PlayerBlack},
		{"a4", PlayerBlack},
		{"a5", PlayerBlack},
	}

	for _, move := range moves2 {
		stone := &Stone{
			Player: move.player,
			Type:   StoneFlat,
		}
		game2.Board.Squares[move.square] = []*Stone{stone}
	}

	winner2, gameOver2 := game2.GameOver()
	if !gameOver2 {
		t.Errorf("Expected game to be over, but it wasn't")
	}
	if winner2 != PlayerBlack {
		t.Errorf("Expected PlayerBlack (%d) to win, got %d", PlayerBlack, winner2)
	}
}

// Test GameOver function with flat wins
func TestGameOverFlatWin(t *testing.T) {
	game, err := NewGame(4, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Fill the board with more white flat stones than black
	whiteSquares := []string{"a1", "a2", "a3", "a4", "b1", "b2", "b3", "b4", "c1"}
	blackSquares := []string{"c2", "c3", "c4", "d1", "d2", "d3", "d4"}

	for _, square := range whiteSquares {
		stone := &Stone{
			Player: PlayerWhite,
			Type:   StoneFlat,
		}
		game.Board.Squares[square] = []*Stone{stone}
	}

	for _, square := range blackSquares {
		stone := &Stone{
			Player: PlayerBlack,
			Type:   StoneFlat,
		}
		game.Board.Squares[square] = []*Stone{stone}
	}

	winner, gameOver := game.GameOver()
	if !gameOver {
		t.Errorf("Expected game to be over (board full), but it wasn't")
	}
	if winner != PlayerWhite {
		t.Errorf("Expected PlayerWhite (%d) to win with more flats, got %d", PlayerWhite, winner)
	}
}

// Test first turn rule with DoTurn
func TestFirstTurnRule(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Test first turn using DoTurn (both players place opponent's stones)
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Errorf("First turn failed: %v", err)
	}

	// Check that white placed a black stone at a1
	topStone := game.Board.TopStone("a1")
	if topStone == nil || topStone.Player != PlayerBlack {
		t.Errorf("Expected black stone at a1 (placed by white), got %v", topStone)
	}

	// Check that black placed a white stone at b1
	topStone = game.Board.TopStone("b1")
	if topStone == nil || topStone.Player != PlayerWhite {
		t.Errorf("Expected white stone at b1 (placed by black), got %v", topStone)
	}

	// Test second turn (normal turn - players place their own stones)
	err = game.DoTurn("c1", "d1")
	if err != nil {
		t.Errorf("Second turn failed: %v", err)
	}

	// Check that white placed a white stone at c1
	topStone = game.Board.TopStone("c1")
	if topStone == nil || topStone.Player != PlayerWhite {
		t.Errorf("Expected white stone at c1, got %v", topStone)
	}

	// Check that black placed a black stone at d1
	topStone = game.Board.TopStone("d1")
	if topStone == nil || topStone.Player != PlayerBlack {
		t.Errorf("Expected black stone at d1, got %v", topStone)
	}
}

// Test carry limit enforcement
func TestCarryLimitEnforcement(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Create a stack higher than carry limit
	stones := []*Stone{}
	for i := 0; i < 7; i++ { // More than board size (5)
		stones = append(stones, &Stone{Player: PlayerWhite, Type: StoneFlat})
	}
	game.Board.Squares["c3"] = stones

	// Try to move more stones than carry limit allows
	move, err := NewMove("7c3+7")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move, PlayerWhite)
	if err == nil {
		t.Errorf("Expected carry limit error, but move succeeded")
	}
}

// Test capstone flattening
func TestCapstoneFlattening(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Place a standing stone
	game.Board.Squares["b3"] = []*Stone{{Player: PlayerBlack, Type: StoneStanding}}

	// Place a capstone that can flatten it
	game.Board.Squares["a3"] = []*Stone{{Player: PlayerWhite, Type: StoneCap}}

	// Move capstone to flatten the standing stone
	move, err := NewMove("a3>")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move, PlayerWhite)
	if err != nil {
		t.Errorf("Capstone flattening failed: %v", err)
	}

	// Check that the standing stone was flattened
	stones := game.Board.Squares["b3"]
	if len(stones) < 2 {
		t.Fatalf("Expected at least 2 stones at b3, got %d", len(stones))
	}

	// Bottom stone should be flattened
	if stones[0].Type != StoneFlat {
		t.Errorf("Expected flattened stone, got %s", stones[0].Type)
	}

	// Top stone should be capstone
	if stones[1].Type != StoneCap {
		t.Errorf("Expected capstone on top, got %s", stones[1].Type)
	}
}

// Test placement validation
func TestPlacementValidation(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Place a standing stone
	game.Board.Squares["c3"] = []*Stone{{Player: PlayerWhite, Type: StoneStanding}}

	// Try to place a stone on the standing stone
	move, err := NewMove("c3")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move, PlayerBlack)
	if err == nil {
		t.Errorf("Expected error placing stone on standing stone, but move succeeded")
	}

	// Place a capstone
	game.Board.Squares["d3"] = []*Stone{{Player: PlayerWhite, Type: StoneCap}}

	// Try to place a stone on the capstone
	move2, err := NewMove("d3")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move2, PlayerBlack)
	if err == nil {
		t.Errorf("Expected error placing stone on capstone, but move succeeded")
	}
}

// Test piece count limits
func TestPieceCountLimits(t *testing.T) {
	testCases := []struct {
		size      int64
		maxPieces int64
		capstones int64
	}{
		{4, 15, 0},
		{5, 21, 1},
		{6, 30, 1},
		{8, 50, 2},
	}

	for _, tc := range testCases {
		game, err := NewGame(tc.size, 1, "test")
		if err != nil {
			t.Fatalf("Failed to create game of size %d: %v", tc.size, err)
		}

		maxStones := game.GetMaxStonesForBoardSize()
		if maxStones != tc.maxPieces {
			t.Errorf("Size %d: expected %d max stones, got %d", tc.size, tc.maxPieces, maxStones)
		}

		capstones := game.GetCapstoneCount()
		if capstones != tc.capstones {
			t.Errorf("Size %d: expected %d capstones, got %d", tc.size, tc.capstones, capstones)
		}
	}
}

// Test road detection doesn't work with standing stones
func TestRoadDetectionStandingStones(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Create a potential road but with a standing stone in the middle
	moves := []struct {
		square string
		player int
		stone  string
	}{
		{"a1", PlayerWhite, StoneFlat},
		{"b1", PlayerWhite, StoneFlat},
		{"c1", PlayerWhite, StoneStanding}, // Standing stone breaks the road
		{"d1", PlayerWhite, StoneFlat},
		{"e1", PlayerWhite, StoneFlat},
	}

	for _, move := range moves {
		stone := &Stone{
			Player: move.player,
			Type:   move.stone,
		}
		game.Board.Squares[move.square] = []*Stone{stone}
	}

	winner, gameOver := game.GameOver()
	if gameOver {
		t.Errorf("Expected game to continue (standing stone should break road), but game ended with winner %d", winner)
	}
}
