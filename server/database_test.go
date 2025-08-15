package main

import (
	"testing"

	"github.com/icco/gotak"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	// Use in-memory SQLite for testing with silent logger to avoid test output pollution
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Run auto-migration
	err = AutoMigrate(db)
	if err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	return db
}

func TestCreateGame(t *testing.T) {
	db := setupTestDB(t)

	// Test creating a game with default size
	slug1, err := createGame(db, 3) // Should default to 6
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	if slug1 == "" {
		t.Error("Expected non-empty slug")
	}

	// Test creating a game with valid size (use separate test DB to avoid duplicate slugs)
	db2 := setupTestDB(t)
	slug2, err := createGame(db2, 8)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	if slug2 == "" {
		t.Error("Expected non-empty slug")
	}

	// Verify game exists in database
	var game Game
	err = db2.Where("slug = ?", slug2).First(&game).Error
	if err != nil {
		t.Fatalf("Game not found in database: %v", err)
	}

	if game.Slug != slug2 {
		t.Errorf("Expected slug %s, got %s", slug2, game.Slug)
	}
}

func TestUpdateTag(t *testing.T) {
	db := setupTestDB(t)

	// Create a game first
	slug, err := createGame(db, 6)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Test updating a tag
	err = updateTag(db, slug, "TestKey", "TestValue")
	if err != nil {
		t.Fatalf("Failed to update tag: %v", err)
	}

	// Verify tag exists in database
	var tag Tag
	err = db.Where("key = ? AND value = ?", "TestKey", "TestValue").First(&tag).Error
	if err != nil {
		t.Fatalf("Tag not found in database: %v", err)
	}

	// Test updating non-existent game
	err = updateTag(db, "nonexistent", "Key", "Value")
	if err == nil {
		t.Error("Expected error when updating tag for non-existent game")
	}
}

func TestGetGameID(t *testing.T) {
	db := setupTestDB(t)

	// Create a game first
	slug, err := createGame(db, 6)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Test getting game ID
	id, err := getGameID(db, slug)
	if err != nil {
		t.Fatalf("Failed to get game ID: %v", err)
	}

	if id <= 0 {
		t.Error("Expected positive game ID")
	}

	// Test getting non-existent game ID
	_, err = getGameID(db, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting ID for non-existent game")
	}
}

func TestInsertMove(t *testing.T) {
	db := setupTestDB(t)

	// Create a game first
	slug, err := createGame(db, 6)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	gameID, err := getGameID(db, slug)
	if err != nil {
		t.Fatalf("Failed to get game ID: %v", err)
	}

	// Test inserting a move
	err = insertMove(db, gameID, gotak.PlayerWhite, "a1", 1)
	if err != nil {
		t.Fatalf("Failed to insert move: %v", err)
	}

	// Verify move exists in database
	var move Move
	err = db.Where("game_id = ? AND player = ? AND text = ?", gameID, gotak.PlayerWhite, "a1").First(&move).Error
	if err != nil {
		t.Fatalf("Move not found in database: %v", err)
	}

	if move.Turn != 1 {
		t.Errorf("Expected turn 1, got %d", move.Turn)
	}
}

func TestGetGame(t *testing.T) {
	db := setupTestDB(t)

	// Create a game first
	slug, err := createGame(db, 8)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Get the game ID for inserting moves
	gameID, err := getGameID(db, slug)
	if err != nil {
		t.Fatalf("Failed to get game ID: %v", err)
	}

	// Insert some moves
	err = insertMove(db, gameID, gotak.PlayerWhite, "a1", 1)
	if err != nil {
		t.Fatalf("Failed to insert move: %v", err)
	}

	err = insertMove(db, gameID, gotak.PlayerBlack, "b2", 1)
	if err != nil {
		t.Fatalf("Failed to insert move: %v", err)
	}

	// Test getting the game
	game, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("Failed to get game: %v", err)
	}

	if game.Slug != slug {
		t.Errorf("Expected slug %s, got %s", slug, game.Slug)
	}

	if game.Board.Size != 8 {
		t.Errorf("Expected board size 8, got %d", game.Board.Size)
	}

	// Verify moves were loaded
	if len(game.Turns) == 0 {
		t.Error("Expected at least one turn with moves")
	}

	// Test getting non-existent game
	_, err = getGame(db, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting non-existent game")
	}
}

func TestUpdateGameStatus(t *testing.T) {
	db := setupTestDB(t)

	// Create a game first
	slug, err := createGame(db, 6)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Test updating game status
	err = updateGameStatus(db, slug, "finished", gotak.PlayerWhite)
	if err != nil {
		t.Fatalf("Failed to update game status: %v", err)
	}

	// Verify status was updated
	var game Game
	err = db.Where("slug = ?", slug).First(&game).Error
	if err != nil {
		t.Fatalf("Game not found: %v", err)
	}

	if game.Status != "finished" {
		t.Errorf("Expected status 'finished', got %s", game.Status)
	}

	if game.Winner != gotak.PlayerWhite {
		t.Errorf("Expected winner %d, got %d", gotak.PlayerWhite, game.Winner)
	}

	// Test updating non-existent game
	err = updateGameStatus(db, "nonexistent", "finished", 1)
	if err == nil {
		t.Error("Expected error when updating status for non-existent game")
	}
}

func TestGameWorkflow(t *testing.T) {
	db := setupTestDB(t)

	// Test complete game workflow
	// 1. Create game
	slug, err := createGame(db, 6)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// 2. Add some metadata
	err = updateTag(db, slug, "Player1", "Alice")
	if err != nil {
		t.Fatalf("Failed to add player tag: %v", err)
	}

	err = updateTag(db, slug, "Player2", "Bob")
	if err != nil {
		t.Fatalf("Failed to add player tag: %v", err)
	}

	// 3. Get game ID
	gameID, err := getGameID(db, slug)
	if err != nil {
		t.Fatalf("Failed to get game ID: %v", err)
	}

	// 4. Play some moves
	moves := []struct {
		player int
		text   string
		turn   int64
	}{
		{gotak.PlayerWhite, "b2", 1}, // White places black's stone on first turn
		{gotak.PlayerBlack, "a1", 1}, // Black places white's stone on first turn
		{gotak.PlayerWhite, "c3", 2},
		{gotak.PlayerBlack, "d4", 2},
	}

	for _, move := range moves {
		err = insertMove(db, gameID, move.player, move.text, move.turn)
		if err != nil {
			t.Fatalf("Failed to insert move %s: %v", move.text, err)
		}
	}

	// 5. Retrieve and verify complete game
	game, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("Failed to get complete game: %v", err)
	}

	// Verify game properties
	if game.Board.Size != 6 {
		t.Errorf("Expected board size 6, got %d", game.Board.Size)
	}

	// Verify metadata
	sizeFound := false
	player1Found := false
	player2Found := false
	for _, tag := range game.Meta {
		switch tag.Key {
		case "Size":
			if tag.Value != "6" {
				t.Errorf("Expected Size '6', got %s", tag.Value)
			}
			sizeFound = true
		case "Player1":
			if tag.Value != "Alice" {
				t.Errorf("Expected Player1 'Alice', got %s", tag.Value)
			}
			player1Found = true
		case "Player2":
			if tag.Value != "Bob" {
				t.Errorf("Expected Player2 'Bob', got %s", tag.Value)
			}
			player2Found = true
		}
	}

	if !sizeFound {
		t.Error("Size tag not found in game metadata")
	}
	if !player1Found {
		t.Error("Player1 tag not found in game metadata")
	}
	if !player2Found {
		t.Error("Player2 tag not found in game metadata")
	}

	// Verify turns and moves
	if len(game.Turns) < 2 {
		t.Errorf("Expected at least 2 turns, got %d", len(game.Turns))
	}

	// 6. Update game status
	err = updateGameStatus(db, slug, "finished", gotak.PlayerWhite)
	if err != nil {
		t.Fatalf("Failed to update game status: %v", err)
	}

	// 7. Verify final state
	finalGame, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("Failed to get final game state: %v", err)
	}

	// Note: The Game struct from gotak package doesn't have Status/Winner fields
	// So we verify by checking the database model directly
	var dbGame Game
	err = db.Where("slug = ?", slug).First(&dbGame).Error
	if err != nil {
		t.Fatalf("Failed to get game from database: %v", err)
	}

	if dbGame.Status != "finished" {
		t.Errorf("Expected final status 'finished', got %s", dbGame.Status)
	}

	if dbGame.Winner != gotak.PlayerWhite {
		t.Errorf("Expected winner %d, got %d", gotak.PlayerWhite, dbGame.Winner)
	}

	// Ensure we can still load the game after status update
	if finalGame.Slug != slug {
		t.Errorf("Expected slug %s, got %s", slug, finalGame.Slug)
	}
}

func TestAutoMigrate(t *testing.T) {
	// Test that AutoMigrate works correctly
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	err = AutoMigrate(db)
	if err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// Verify tables exist by trying to query them
	var count int64
	
	err = db.Model(&Game{}).Count(&count).Error
	if err != nil {
		t.Errorf("Games table not created properly: %v", err)
	}

	err = db.Model(&Tag{}).Count(&count).Error
	if err != nil {
		t.Errorf("Tags table not created properly: %v", err)
	}

	err = db.Model(&Move{}).Count(&count).Error
	if err != nil {
		t.Errorf("Moves table not created properly: %v", err)
	}
}

func TestGetGameWithNoMoves(t *testing.T) {
	db := setupTestDB(t)

	// Create a game with no moves
	slug, err := createGame(db, 5)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	// Test getting game with no moves
	game, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("Failed to get game with no moves: %v", err)
	}

	if game.Board.Size != 5 {
		t.Errorf("Expected board size 5, got %d", game.Board.Size)
	}

	if len(game.Turns) != 0 {
		t.Errorf("Expected 0 turns, got %d", len(game.Turns))
	}
}

func TestDatabaseTypesConsistency(t *testing.T) {
	db := setupTestDB(t)

	// Create a game and verify ID types are consistent
	slug, err := createGame(db, 6)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}

	gameID, err := getGameID(db, slug)
	if err != nil {
		t.Fatalf("Failed to get game ID: %v", err)
	}

	// Insert a move and verify types
	err = insertMove(db, gameID, gotak.PlayerWhite, "a1", 1)
	if err != nil {
		t.Fatalf("Failed to insert move: %v", err)
	}

	// Retrieve the game and verify the ID types match
	game, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("Failed to get game: %v", err)
	}

	if game.ID != gameID {
		t.Errorf("Game ID mismatch: expected %d, got %d", gameID, game.ID)
	}

	// Verify the ID is int64 as expected by gotak.Game
	var _ int64 = game.ID
}

func TestGetGameBoardState(t *testing.T) {
	db := setupTestDB(t)

	// Create a new game
	slug, err := createGame(db, 5)
	if err != nil {
		t.Fatalf("could not create game: %v", err)
	}

	// Get the game to get its ID
	game, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("could not get game: %v", err)
	}

	// Insert some moves
	moves := []struct {
		player int
		text   string
		turn   int64
	}{
		{gotak.PlayerWhite, "a1", 1}, // First turn: white places black stone
		{gotak.PlayerBlack, "e5", 1}, // First turn: black places white stone
		{gotak.PlayerWhite, "b2", 2}, // Second turn: white places white stone
		{gotak.PlayerBlack, "d4", 2}, // Second turn: black places black stone
	}

	for _, move := range moves {
		if err := insertMove(db, game.ID, move.player, move.text, move.turn); err != nil {
			t.Fatalf("could not insert move: %v", err)
		}
	}

	// Get the game from database
	retrievedGame, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("could not get game: %v", err)
	}

	// Verify board state is updated
	if retrievedGame.Board.Squares["a1"] == nil || len(retrievedGame.Board.Squares["a1"]) == 0 {
		t.Error("board state not updated: a1 should have a stone")
	}
	if retrievedGame.Board.Squares["e5"] == nil || len(retrievedGame.Board.Squares["e5"]) == 0 {
		t.Error("board state not updated: e5 should have a stone")
	}
	if retrievedGame.Board.Squares["b2"] == nil || len(retrievedGame.Board.Squares["b2"]) == 0 {
		t.Error("board state not updated: b2 should have a stone")
	}
	if retrievedGame.Board.Squares["d4"] == nil || len(retrievedGame.Board.Squares["d4"]) == 0 {
		t.Error("board state not updated: d4 should have a stone")
	}

	// Verify stone colors are correct
	if retrievedGame.Board.Color("a1") != gotak.PlayerWhite {
		t.Errorf("a1 should be white, got %d", retrievedGame.Board.Color("a1"))
	}
	if retrievedGame.Board.Color("e5") != gotak.PlayerBlack {
		t.Errorf("e5 should be black, got %d", retrievedGame.Board.Color("e5"))
	}
	if retrievedGame.Board.Color("b2") != gotak.PlayerWhite {
		t.Errorf("b2 should be white, got %d", retrievedGame.Board.Color("b2"))
	}
	if retrievedGame.Board.Color("d4") != gotak.PlayerBlack {
		t.Errorf("d4 should be black, got %d", retrievedGame.Board.Color("d4"))
	}
}