package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/ifo/sanic"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"moul.io/zapgorm2"

	"github.com/icco/gotak"
)

func getDB() (*gorm.DB, error) {
	dbURL := os.Getenv("DATABASE_URL")
	log.Debugw("Attempting database connection", "database_url_length", len(dbURL))
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is empty")
	}
	log.Debugw("DATABASE_URL is set, attempting connection")

	// Configure GORM to use zapgorm2 logger
	config := &gorm.Config{
		Logger: zapgorm2.New(log.Desugar()),
	}

	db, err := gorm.Open(postgres.Open(dbURL), config)
	if err != nil {
		return nil, err
	}

	// Auto-migrate the schema
	err = AutoMigrate(db)
	if err != nil {
		return nil, fmt.Errorf("failed to run auto-migration: %v", err)
	}

	return db, nil
}

func createGame(db *gorm.DB, size int, userID int64) (string, error) {
	if size < 4 {
		size = 6
	}

	if userID == 0 {
		return "", fmt.Errorf("user authentication required")
	}

	// Game Slug
	worker := sanic.NewWorker7()
	id := worker.NextID()
	slug := worker.IDString(id)

	game := Game{
		Slug:          slug,
		WhitePlayerID: userID,    // Creator becomes white player
		Status:        "waiting", // Waiting for second player
	}

	if err := db.Create(&game).Error; err != nil {
		return "", err
	}

	return slug, updateTag(db, slug, "Size", strconv.Itoa(size))
}

func updateTag(db *gorm.DB, slug, key, value string) error {
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		return err
	}

	tag := Tag{
		GameID: game.ID,
		Key:    key,
		Value:  value,
	}

	return db.Create(&tag).Error
}

func insertMove(db *gorm.DB, gameID int64, player int, text string, turnNumber int64) error {
	move := Move{
		GameID: gameID,
		Player: player,
		Text:   text,
		Turn:   turnNumber,
	}

	return db.Create(&move).Error
}

func getGameID(db *gorm.DB, slug string) (int64, error) {
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		return 0, err
	}
	return game.ID, nil
}

func getGame(db *gorm.DB, slug string) (*gotak.Game, error) {
	id, err := getGameID(db, slug)
	if err != nil {
		return nil, err
	}

	// Get Size
	var tag Tag
	if err := db.Where("game_id = ? AND key = ?", id, "Size").First(&tag).Error; err != nil {
		return nil, err
	}

	size, err := strconv.ParseInt(tag.Value, 10, 64)
	if err != nil {
		return nil, err
	}

	// Init game
	game, err := gotak.NewGame(size, id, slug)
	if err != nil {
		return game, err
	}

	err = getTurns(db, game)
	if err != nil {
		return game, err
	}

	err = getMeta(db, game)
	if err != nil {
		return game, err
	}

	// Replay moves to update board state
	err = replayMoves(game)
	if err != nil {
		return game, err
	}

	return game, nil
}

func getTurns(db *gorm.DB, game *gotak.Game) error {
	var moves []Move
	if err := db.Where("game_id = ?", game.ID).Order("turn, created_at").Find(&moves).Error; err != nil {
		return err
	}

	for _, move := range moves {
		currentTurn, err := game.GetTurn(move.Turn)
		if err != nil {
			return err
		}

		mv, err := gotak.NewMove(move.Text)
		if err != nil {
			return err
		}

		if move.Player == gotak.PlayerWhite {
			if move.Turn > 1 {
				currentTurn.First = mv
			} else {
				currentTurn.Second = mv
			}
		}

		if move.Player == gotak.PlayerBlack {
			if move.Turn > 1 {
				currentTurn.Second = mv
			} else {
				currentTurn.First = mv
			}
		}

		game.UpdateTurn(currentTurn)
	}

	return nil
}

func getMeta(db *gorm.DB, game *gotak.Game) error {
	var tags []Tag
	if err := db.Where("game_id = ?", game.ID).Order("created_at").Find(&tags).Error; err != nil {
		return err
	}

	for _, tag := range tags {
		err := game.UpdateMeta(tag.Key, tag.Value)
		if err != nil {
			return err
		}
	}

	return nil
}

// replayMoves replays all moves in a game to restore board state
func replayMoves(game *gotak.Game) error {
	// Reset board
	err := game.Board.Init()
	if err != nil {
		return err
	}

	// Replay all moves in order
	for _, turn := range game.Turns {
		if turn.First != nil {
			if turn.Number == 1 {
				// First turn: white places black stone
				err = game.Board.DoMove(turn.First, gotak.PlayerBlack)
			} else {
				err = game.Board.DoMove(turn.First, gotak.PlayerWhite)
			}
			if err != nil {
				return fmt.Errorf("error replaying turn %d first move: %v", turn.Number, err)
			}
		}

		if turn.Second != nil {
			if turn.Number == 1 {
				// First turn: black places white stone
				err = game.Board.DoMove(turn.Second, gotak.PlayerWhite)
			} else {
				err = game.Board.DoMove(turn.Second, gotak.PlayerBlack)
			}
			if err != nil {
				return fmt.Errorf("error replaying turn %d second move: %v", turn.Number, err)
			}
		}
	}

	return nil
}

// updateGameStatus updates the game status in the database
func updateGameStatus(db *gorm.DB, slug, status string, winner int) error {
	result := db.Model(&Game{}).Where("slug = ?", slug).Updates(Game{
		Status: status,
		Winner: winner,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// verifyGameParticipation checks if the user is a participant in the specified game
func verifyGameParticipation(db *gorm.DB, slug string, userID int64) error {
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		return err
	}

	// Check if user is white player
	if game.WhitePlayerID == userID {
		return nil
	}

	// Check if user is black player
	if game.BlackPlayerID != nil && *game.BlackPlayerID == userID {
		return nil
	}

	return fmt.Errorf("unauthorized: user is not a participant in this game")
}

// joinGame allows a user to join a waiting game as the black player
func joinGame(db *gorm.DB, slug string, userID int64) error {
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		return err
	}

	// Can't join if already a participant
	if game.WhitePlayerID == userID {
		return fmt.Errorf("user is already the white player in this game")
	}

	if game.BlackPlayerID != nil {
		if *game.BlackPlayerID == userID {
			return fmt.Errorf("user is already the black player in this game")
		}
		return fmt.Errorf("game is full - both players already assigned")
	}

	// Can only join games that are waiting
	if game.Status != "waiting" {
		return fmt.Errorf("can only join games with 'waiting' status")
	}

	// Update game with black player and change status to active
	updates := Game{
		BlackPlayerID: &userID,
		Status:        "active",
	}

	result := db.Model(&game).Where("slug = ?", slug).Updates(updates)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("failed to join game - no rows affected")
	}

	return nil
}

// getPlayerNumber returns the player number (1 for white, 2 for black) for a user in a game
func getPlayerNumber(db *gorm.DB, slug string, userID int64) (int, error) {
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		return 0, err
	}

	if game.WhitePlayerID == userID {
		return 1, nil // PlayerWhite
	}

	if game.BlackPlayerID != nil && *game.BlackPlayerID == userID {
		return 2, nil // PlayerBlack
	}

	return 0, fmt.Errorf("user is not a participant in this game")
}
