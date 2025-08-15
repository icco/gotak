package gotak

import (
	"fmt"
	"testing"
)

// TestBoardSizeValidation tests that only valid board sizes (4-8) are accepted
func TestBoardSizeValidation(t *testing.T) {
	testCases := []struct {
		size    int64
		isValid bool
	}{
		{3, false},  // Too small
		{4, true},   // Minimum valid size
		{5, true},   // Standard size
		{6, true},   // Standard size
		{7, true},   // Standard size
		{8, true},   // Standard size
		{9, true},   // Extended size (supported by implementation)
		{10, false}, // Too large
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Size%d", tc.size), func(t *testing.T) {
			board := &Board{Size: tc.size}
			err := board.Init()
			
			if tc.isValid && err != nil {
				t.Errorf("Expected valid board size %d, got error: %v", tc.size, err)
			}
			if !tc.isValid && err == nil {
				t.Errorf("Expected invalid board size %d, but no error returned", tc.size)
			}
		})
	}
}

// TestStoneCountsByBoardSize tests the correct stone counts for each board size
func TestStoneCountsByBoardSize(t *testing.T) {
	testCases := []struct {
		size           int64
		expectedStones int64
		expectedCaps   int64
	}{
		{4, 15, 0},  // 4x4: 15 stones, 0 capstones
		{5, 21, 1},  // 5x5: 21 stones, 1 capstone
		{6, 30, 1},  // 6x6: 30 stones, 1 capstone
		{7, 40, 1},  // 7x7: 40 stones, 1 capstone (assuming 1 based on rules)
		{8, 50, 2},  // 8x8: 50 stones, 2 capstones
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Size%d", tc.size), func(t *testing.T) {
			game, err := NewGame(tc.size, 1, "test")
			if err != nil {
				t.Fatalf("Failed to create game: %v", err)
			}

			stones := game.GetMaxStonesForBoardSize()
			if stones != tc.expectedStones {
				t.Errorf("Expected %d stones for size %d, got %d", tc.expectedStones, tc.size, stones)
			}

			caps := game.GetCapstoneCount()
			if caps != tc.expectedCaps {
				t.Errorf("Expected %d capstones for size %d, got %d", tc.expectedCaps, tc.size, caps)
			}
		})
	}
}

// TestFirstTurnRuleComprehensive tests the special first turn rule where players place opponent's stones
func TestFirstTurnRuleComprehensive(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Test that first turn must be flat stone placement only
	err = game.DoSingleMove("a1", PlayerWhite)
	if err != nil {
		t.Errorf("First move should be valid: %v", err)
	}

	// Check that white placed a black stone (opponent's color)
	topStone := game.Board.TopStone("a1")
	if topStone == nil || topStone.Player != PlayerBlack {
		t.Errorf("Expected black stone at a1 (placed by white), got %v", topStone)
	}

	// Test that non-flat stones are rejected on first turn
	game2, err := NewGame(5, 2, "test2")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	err = game2.DoSingleMove("Sa1", PlayerWhite)
	if err == nil {
		t.Errorf("Expected error for standing stone on first turn, but move succeeded")
	}

	err = game2.DoSingleMove("Ca1", PlayerWhite)
	if err == nil {
		t.Errorf("Expected error for capstone on first turn, but move succeeded")
	}
}

// TestNormalTurnRule tests that after first turn, players place their own stones
func TestNormalTurnRule(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Test second turn - players place their own stones
	err = game.DoTurn("c1", "d1")
	if err != nil {
		t.Errorf("Second turn failed: %v", err)
	}

	// Check that white placed a white stone
	topStone := game.Board.TopStone("c1")
	if topStone == nil || topStone.Player != PlayerWhite {
		t.Errorf("Expected white stone at c1, got %v", topStone)
	}

	// Check that black placed a black stone
	topStone = game.Board.TopStone("d1")
	if topStone == nil || topStone.Player != PlayerBlack {
		t.Errorf("Expected black stone at d1, got %v", topStone)
	}
}

// TestStonePlacementRules tests the three types of stone placement
func TestStonePlacementRules(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Test flat stone placement (default) and standing stone placement
	err = game.DoTurn("c1", "Sc2")
	if err != nil {
		t.Errorf("Second turn failed: %v", err)
	}

	topStone := game.Board.TopStone("c1")
	if topStone.Type != StoneFlat {
		t.Errorf("Expected flat stone, got %s", topStone.Type)
	}

	topStone = game.Board.TopStone("c2")
	if topStone.Type != StoneStanding {
		t.Errorf("Expected standing stone, got %s", topStone.Type)
	}

	// Test capstone placement
	err = game.DoTurn("Cc3", "d3")
	if err != nil {
		t.Errorf("Third turn failed: %v", err)
	}

	topStone = game.Board.TopStone("c3")
	if topStone.Type != StoneCap {
		t.Errorf("Expected capstone, got %s", topStone.Type)
	}
}

// TestPlacementOnStandingStones tests that only capstones can be placed on standing stones
func TestPlacementOnStandingStones(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Place a standing stone
	err = game.DoSingleMove("Sc3", PlayerWhite)
	if err != nil {
		t.Fatalf("Failed to place standing stone: %v", err)
	}

	// Try to place flat stone on standing stone - should fail
	err = game.DoSingleMove("c3", PlayerBlack)
	if err == nil {
		t.Errorf("Expected error placing flat stone on standing stone, but move succeeded")
	}

	// Try to place standing stone on standing stone - should fail
	err = game.DoSingleMove("Sc3", PlayerBlack)
	if err == nil {
		t.Errorf("Expected error placing standing stone on standing stone, but move succeeded")
	}

	// Place capstone on standing stone - should succeed and flatten it
	err = game.DoSingleMove("Cc3", PlayerBlack)
	if err != nil {
		t.Errorf("Capstone should be able to flatten standing stone: %v", err)
	}

	// Check that the standing stone was flattened
	stones := game.Board.Squares["c3"]
	if len(stones) < 2 {
		t.Fatalf("Expected at least 2 stones at c3, got %d", len(stones))
	}

	if stones[0].Type != StoneFlat {
		t.Errorf("Expected flattened stone, got %s", stones[0].Type)
	}

	if stones[1].Type != StoneCap {
		t.Errorf("Expected capstone on top, got %s", stones[1].Type)
	}
}

// TestPlacementOnCapstones tests that nothing can be placed on capstones
func TestPlacementOnCapstones(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Place a capstone
	err = game.DoSingleMove("Cc3", PlayerWhite)
	if err != nil {
		t.Fatalf("Failed to place capstone: %v", err)
	}

	// Try to place flat stone on capstone - should fail
	err = game.DoSingleMove("c3", PlayerBlack)
	if err == nil {
		t.Errorf("Expected error placing flat stone on capstone, but move succeeded")
	}

	// Try to place standing stone on capstone - should fail
	err = game.DoSingleMove("Sc3", PlayerBlack)
	if err == nil {
		t.Errorf("Expected error placing standing stone on capstone, but move succeeded")
	}

	// Try to place capstone on capstone - should fail
	err = game.DoSingleMove("Cc3", PlayerBlack)
	if err == nil {
		t.Errorf("Expected error placing capstone on capstone, but move succeeded")
	}
}

// TestStackControl tests that the top stone determines stack control
func TestStackControl(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// White places a stone
	err = game.DoSingleMove("c3", PlayerWhite)
	if err != nil {
		t.Fatalf("Failed to place white stone: %v", err)
	}

	// Black places a stone on top
	err = game.DoSingleMove("c3", PlayerBlack)
	if err != nil {
		t.Fatalf("Failed to place black stone on top: %v", err)
	}

	// Check that black controls the stack
	control := game.Board.Color("c3")
	if control != PlayerBlack {
		t.Errorf("Expected black to control stack, got player %d", control)
	}

	// Try to move stack with white - should fail
	move, err := NewMove("c3+")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move, PlayerWhite)
	if err == nil {
		t.Errorf("Expected error moving opponent's stack, but move succeeded")
	}

	// Move stack with black - should succeed
	err = game.Board.DoMove(move, PlayerBlack)
	if err != nil {
		t.Errorf("Failed to move own stack: %v", err)
	}
}

// TestCarryLimit tests that players cannot carry more stones than the board size
func TestCarryLimit(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Create a stack of 6 stones (more than board size 5)
	for i := 0; i < 6; i++ {
		stone := &Stone{Player: PlayerWhite, Type: StoneFlat}
		game.Board.Squares["c3"] = append(game.Board.Squares["c3"], stone)
	}

	// Try to move 6 stones - should fail
	move, err := NewMove("6c3+6")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move, PlayerWhite)
	if err == nil {
		t.Errorf("Expected carry limit error, but move succeeded")
	}

	// Move 5 stones - should succeed
	move, err = NewMove("5c3+5")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move, PlayerWhite)
	if err != nil {
		t.Errorf("Failed to move within carry limit: %v", err)
	}
}

// TestOrthogonalMovement tests that stones can only move orthogonally
func TestOrthogonalMovement(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Place a stone at c3
	err = game.DoSingleMove("c3", PlayerWhite)
	if err != nil {
		t.Fatalf("Failed to place stone: %v", err)
	}

	// Test all four orthogonal directions
	directions := []string{"+", "-", "<", ">"}
	for _, dir := range directions {
		t.Run(fmt.Sprintf("Direction%s", dir), func(t *testing.T) {
			move, err := NewMove(fmt.Sprintf("c3%s", dir))
			if err != nil {
				t.Fatalf("Failed to create move: %v", err)
			}

			err = game.Board.DoMove(move, PlayerWhite)
			if err != nil {
				t.Errorf("Failed to move in direction %s: %v", dir, err)
			}
		})
	}
}

// TestStackMovement tests that stacks can be moved as a unit
func TestStackMovement(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Create a stack of 3 stones
	for i := 0; i < 3; i++ {
		stone := &Stone{Player: PlayerWhite, Type: StoneFlat}
		game.Board.Squares["c3"] = append(game.Board.Squares["c3"], stone)
	}

	// Move the entire stack
	move, err := NewMove("3c3+3")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move, PlayerWhite)
	if err != nil {
		t.Errorf("Failed to move stack: %v", err)
	}

	// Check that all stones moved
	if len(game.Board.Squares["c3"]) != 0 {
		t.Errorf("Expected empty c3, got %d stones", len(game.Board.Squares["c3"]))
	}

	if len(game.Board.Squares["c4"]) != 3 {
		t.Errorf("Expected 3 stones at c4, got %d", len(game.Board.Squares["c4"]))
	}
}

// TestStackBreaking tests that stacks can be broken during movement
func TestStackBreaking(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Create a stack of 4 stones
	for i := 0; i < 4; i++ {
		stone := &Stone{Player: PlayerWhite, Type: StoneFlat}
		game.Board.Squares["c3"] = append(game.Board.Squares["c3"], stone)
	}

	// Move stack and break it: 1 stone at c4, 2 stones at c5, 1 stone at c6
	move, err := NewMove("4c3+121")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move, PlayerWhite)
	if err != nil {
		t.Errorf("Failed to break stack: %v", err)
	}

	// Check distribution
	if len(game.Board.Squares["c3"]) != 0 {
		t.Errorf("Expected empty c3, got %d stones", len(game.Board.Squares["c3"]))
	}

	if len(game.Board.Squares["c4"]) != 1 {
		t.Errorf("Expected 1 stone at c4, got %d", len(game.Board.Squares["c4"]))
	}

	if len(game.Board.Squares["c5"]) != 2 {
		t.Errorf("Expected 2 stones at c5, got %d", len(game.Board.Squares["c5"]))
	}

	if len(game.Board.Squares["c6"]) != 1 {
		t.Errorf("Expected 1 stone at c6, got %d", len(game.Board.Squares["c6"]))
	}
}

// TestRoadWinHorizontal tests horizontal road wins
func TestRoadWinHorizontal(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Create horizontal road for white
	roadSquares := []string{"a1", "b1", "c1", "d1", "e1"}
	for _, square := range roadSquares {
		stone := &Stone{Player: PlayerWhite, Type: StoneFlat}
		game.Board.Squares[square] = []*Stone{stone}
	}

	winner, gameOver := game.GameOver()
	if !gameOver {
		t.Errorf("Expected game to be over with road win, but it wasn't")
	}
	if winner != PlayerWhite {
		t.Errorf("Expected white to win, got player %d", winner)
	}
}

// TestRoadWinVertical tests vertical road wins
func TestRoadWinVertical(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Create vertical road for black
	roadSquares := []string{"a1", "a2", "a3", "a4", "a5"}
	for _, square := range roadSquares {
		stone := &Stone{Player: PlayerBlack, Type: StoneFlat}
		game.Board.Squares[square] = []*Stone{stone}
	}

	winner, gameOver := game.GameOver()
	if !gameOver {
		t.Errorf("Expected game to be over with road win, but it wasn't")
	}
	if winner != PlayerBlack {
		t.Errorf("Expected black to win, got player %d", winner)
	}
}

// TestRoadWithCapstones tests that capstones count toward roads
func TestRoadWithCapstones(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Create road with capstone for white
	roadSquares := []struct {
		square string
		stone  string
	}{
		{"a1", StoneFlat},
		{"b1", StoneFlat},
		{"c1", StoneCap}, // Capstone counts toward road
		{"d1", StoneFlat},
		{"e1", StoneFlat},
	}

	for _, road := range roadSquares {
		stone := &Stone{Player: PlayerWhite, Type: road.stone}
		game.Board.Squares[road.square] = []*Stone{stone}
	}

	winner, gameOver := game.GameOver()
	if !gameOver {
		t.Errorf("Expected game to be over with road win, but it wasn't")
	}
	if winner != PlayerWhite {
		t.Errorf("Expected white to win, got player %d", winner)
	}
}

// TestStandingStonesDontCountForRoads tests that standing stones don't count toward roads
func TestStandingStonesDontCountForRoads(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Create potential road but with standing stone in middle
	roadSquares := []struct {
		square string
		stone  string
	}{
		{"a1", StoneFlat},
		{"b1", StoneFlat},
		{"c1", StoneStanding}, // Standing stone breaks the road
		{"d1", StoneFlat},
		{"e1", StoneFlat},
	}

	for _, road := range roadSquares {
		stone := &Stone{Player: PlayerWhite, Type: road.stone}
		game.Board.Squares[road.square] = []*Stone{stone}
	}

	winner, gameOver := game.GameOver()
	if gameOver {
		t.Errorf("Expected game to continue (standing stone should break road), but game ended with winner %d", winner)
	}
}

// TestFlatWin tests flat win conditions
func TestFlatWin(t *testing.T) {
	game, err := NewGame(4, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Fill board with more white flat stones than black
	whiteSquares := []string{"a2", "a3", "a4", "b2", "b3", "b4", "c1", "c2", "c3"}
	blackSquares := []string{"c4", "d1", "d2", "d3", "d4"}

	for _, square := range whiteSquares {
		stone := &Stone{Player: PlayerWhite, Type: StoneFlat}
		game.Board.Squares[square] = []*Stone{stone}
	}

	for _, square := range blackSquares {
		stone := &Stone{Player: PlayerBlack, Type: StoneFlat}
		game.Board.Squares[square] = []*Stone{stone}
	}

	winner, gameOver := game.GameOver()
	if !gameOver {
		t.Errorf("Expected game to be over (board full), but it wasn't")
	}
	if winner != PlayerWhite {
		t.Errorf("Expected white to win with more flats, got player %d", winner)
	}
}

// TestFlatWinCaptivesDontCount tests that captured stones don't count toward flat win
func TestFlatWinCaptivesDontCount(t *testing.T) {
	game, err := NewGame(4, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Create a stack with white on top, black underneath
	game.Board.Squares["c3"] = []*Stone{
		{Player: PlayerBlack, Type: StoneFlat}, // Captive
		{Player: PlayerWhite, Type: StoneFlat}, // Controller
	}

	// Fill rest of board with equal flats
	remainingSquares := []string{"a2", "a3", "a4", "b2", "b3", "b4", "c1", "c2", "c4", "d1", "d2", "d3", "d4"}
	for i, square := range remainingSquares {
		player := PlayerWhite
		if i%2 == 0 {
			player = PlayerBlack
		}
		stone := &Stone{Player: player, Type: StoneFlat}
		game.Board.Squares[square] = []*Stone{stone}
	}

	winner, gameOver := game.GameOver()
	if !gameOver {
		t.Errorf("Expected game to be over (board full), but it wasn't")
	}
	// Should be a tie since captive doesn't count
	if winner != 0 {
		t.Errorf("Expected tie game, got winner %d", winner)
	}
}

// TestOutOfStonesWin tests win condition when a player runs out of stones
func TestOutOfStonesWin(t *testing.T) {
	game, err := NewGame(4, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Place exactly the maximum number of stones for white
	maxStones := game.GetMaxStonesForBoardSize()
	for i := int64(0); i < maxStones; i++ {
		row := i / 4
		col := i % 4
		square := fmt.Sprintf("%c%d", 'a'+col, row+1)
		stone := &Stone{Player: PlayerWhite, Type: StoneFlat}
		game.Board.Squares[square] = []*Stone{stone}
	}

	winner, gameOver := game.GameOver()
	if !gameOver {
		t.Errorf("Expected game to be over (white out of stones), but it wasn't")
	}
	if winner != PlayerWhite {
		t.Errorf("Expected white to win (more flats), got player %d", winner)
	}
}

// TestTieGame tests tie game conditions
func TestTieGame(t *testing.T) {
	game, err := NewGame(4, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Fill board with equal number of flat stones
	whiteSquares := []string{"a2", "a3", "a4", "b2", "b3", "b4", "c1", "c2"}
	blackSquares := []string{"c3", "c4", "d1", "d2", "d3", "d4"}

	for _, square := range whiteSquares {
		stone := &Stone{Player: PlayerWhite, Type: StoneFlat}
		game.Board.Squares[square] = []*Stone{stone}
	}

	for _, square := range blackSquares {
		stone := &Stone{Player: PlayerBlack, Type: StoneFlat}
		game.Board.Squares[square] = []*Stone{stone}
	}

	winner, gameOver := game.GameOver()
	if !gameOver {
		t.Errorf("Expected game to be over (board full), but it wasn't")
	}
	if winner != 0 {
		t.Errorf("Expected tie game, got winner %d", winner)
	}
}

// TestNoPassingRule tests that players cannot pass their turn
func TestNoPassingRule(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Try to do a turn with empty moves - should fail
	err = game.DoTurn("", "")
	if err == nil {
		t.Errorf("Expected error for empty moves, but turn succeeded")
	}
}

// TestInvalidMoveValidation tests various invalid move scenarios
func TestInvalidMoveValidation(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Test moving from empty square
	move, err := NewMove("c3+")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move, PlayerWhite)
	if err == nil {
		t.Errorf("Expected error moving from empty square, but move succeeded")
	}

	// Test moving opponent's piece
	err = game.DoSingleMove("c3", PlayerWhite)
	if err != nil {
		t.Fatalf("Failed to place stone: %v", err)
	}

	move, err = NewMove("c3+")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move, PlayerBlack)
	if err == nil {
		t.Errorf("Expected error moving opponent's piece, but move succeeded")
	}

	// Test moving off board
	err = game.DoSingleMove("a1", PlayerWhite)
	if err != nil {
		t.Fatalf("Failed to place stone: %v", err)
	}

	move, err = NewMove("a1<")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move, PlayerWhite)
	if err == nil {
		t.Errorf("Expected error moving off board, but move succeeded")
	}
}

// TestCapstoneFlatteningRules tests the specific rules for capstone flattening
func TestCapstoneFlatteningRules(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Place standing stone
	err = game.DoSingleMove("Sc3", PlayerBlack)
	if err != nil {
		t.Fatalf("Failed to place standing stone: %v", err)
	}

	// Place capstone
	err = game.DoSingleMove("Cc2", PlayerWhite)
	if err != nil {
		t.Fatalf("Failed to place capstone: %v", err)
	}

	// Test that capstone must move by itself to flatten
	move, err := NewMove("2c2>2")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move, PlayerWhite)
	if err == nil {
		t.Errorf("Expected error for capstone moving with other stones to flatten, but move succeeded")
	}

	// Test that only capstone can flatten standing stones
	err = game.DoSingleMove("c4", PlayerWhite)
	if err != nil {
		t.Fatalf("Failed to place flat stone: %v", err)
	}

	move, err = NewMove("c4<")
	if err != nil {
		t.Fatalf("Failed to create move: %v", err)
	}

	err = game.Board.DoMove(move, PlayerWhite)
	if err == nil {
		t.Errorf("Expected error for flat stone trying to flatten standing stone, but move succeeded")
	}
}

// TestRoadDetectionComplex tests complex road detection scenarios
func TestRoadDetectionComplex(t *testing.T) {
	game, err := NewGame(6, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Create a complex road with multiple paths
	roadSquares := []struct {
		square string
		player int
		stone  string
	}{
		{"a1", PlayerWhite, StoneFlat},
		{"b1", PlayerWhite, StoneFlat},
		{"c1", PlayerWhite, StoneFlat},
		{"d1", PlayerWhite, StoneFlat},
		{"e1", PlayerWhite, StoneFlat},
		{"f1", PlayerWhite, StoneFlat},
		// Add some blocking pieces
		{"c2", PlayerBlack, StoneStanding},
		{"d2", PlayerBlack, StoneStanding},
	}

	for _, road := range roadSquares {
		stone := &Stone{Player: road.player, Type: road.stone}
		game.Board.Squares[road.square] = []*Stone{stone}
	}

	winner, gameOver := game.GameOver()
	if !gameOver {
		t.Errorf("Expected game to be over with road win, but it wasn't")
	}
	if winner != PlayerWhite {
		t.Errorf("Expected white to win, got player %d", winner)
	}
}

// TestBoardEdgeDetection tests that road wins work from any edge to opposite edge
func TestBoardEdgeDetection(t *testing.T) {
	game, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Complete first turn
	err = game.DoTurn("a1", "b1")
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Test that any edge square can be part of a road win
	edgeTests := []struct {
		startEdge []string
		endEdge   []string
		roadPath  []string
	}{
		{
			startEdge: []string{"a1", "a2", "a3", "a4", "a5"},
			endEdge:   []string{"e1", "e2", "e3", "e4", "e5"},
			roadPath:  []string{"a3", "b3", "c3", "d3", "e3"},
		},
		{
			startEdge: []string{"a1", "b1", "c1", "d1", "e1"},
			endEdge:   []string{"a5", "b5", "c5", "d5", "e5"},
			roadPath:  []string{"c1", "c2", "c3", "c4", "c5"},
		},
	}

	for i, test := range edgeTests {
		t.Run(fmt.Sprintf("EdgeTest%d", i), func(t *testing.T) {
			// Create a fresh game for each test
			game, err := NewGame(5, int64(i+1), fmt.Sprintf("test%d", i))
			if err != nil {
				t.Fatalf("Failed to create game: %v", err)
			}

			// Complete first turn
			err = game.DoTurn("a1", "b1")
			if err != nil {
				t.Fatalf("First turn failed: %v", err)
			}

			// Place stones along the road path
			for _, square := range test.roadPath {
				stone := &Stone{Player: PlayerWhite, Type: StoneFlat}
				game.Board.Squares[square] = []*Stone{stone}
			}

			winner, gameOver := game.GameOver()
			if !gameOver {
				t.Errorf("Expected game to be over with road win, but it wasn't")
			}
			if winner != PlayerWhite {
				t.Errorf("Expected white to win, got player %d", winner)
			}
		})
	}
}
