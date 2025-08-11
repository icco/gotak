package main

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate"
	"github.com/golang-migrate/migrate/database/postgres"
	"github.com/icco/gotak"
	"github.com/ifo/sanic"

	_ "github.com/golang-migrate/migrate/source/file"
	_ "github.com/lib/pq"
)

func getDB() (*sql.DB, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is empty")
	}

	return sql.Open("postgres", dbURL)
}

func updateDB(db *sql.DB) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://./db/migrations",
		"postgres", driver)
	if err != nil {
		return err
	}

	// TODO: Return err if it's not the "no change" error
	m.Up()

	return nil
}

func createGame(db *sql.DB, size int) (string, error) {
	if size < 4 {
		size = 6
	}

	// Game Slug
	worker := sanic.NewWorker7()
	id := worker.NextID()
	slug := worker.IDString(id)

	query := `INSERT INTO games (slug) VALUES ($1)`
	_, err := db.Exec(query, slug)
	if err != nil {
		return "", err
	}

	return slug, updateTag(db, slug, "Size", strconv.Itoa(size))
}

func updateTag(db *sql.DB, slug, key, value string) error {
	id, err := getGameID(db, slug)
	if err != nil {
		return err
	}

	query := `INSERT INTO tags(game_id, key, value) VALUES ($1, $2, $3)`
	_, err = db.Exec(query, id, key, value)

	return err
}

func insertMove(db *sql.DB, gameID int64, player int, text string, turnNumber int64) error {
	query := `INSERT INTO moves (game_id, player, text, turn) VALUES ($1, $2, $3, $4)`
	_, err := db.Exec(query, gameID, player, text, turnNumber)

	return err
}

func getGameID(db *sql.DB, slug string) (int64, error) {
	query := `SELECT id FROM games WHERE slug = $1`
	stmt, err := db.Prepare(query)
	if err != nil {
		return 0, err
	}

	var id int64
	err = stmt.QueryRow(slug).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func getGame(db *sql.DB, slug string) (*gotak.Game, error) {
	id, err := getGameID(db, slug)
	if err != nil {
		return nil, err
	}

	// Get Size
	var size int64
	query := `SELECT value FROM tags WHERE game_id = $1 and key = 'Size' ORDER BY created_at LIMIT 1`
	row := db.QueryRow(query, id)
	err = row.Scan(&size)
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

	return game, nil
}

func getTurns(db *sql.DB, game *gotak.Game) error {
	query := `SELECT player, turn, text FROM moves WHERE game_id = $1 ORDER BY turn, created_at`
	rows, err := db.Query(query, game.ID)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var player int
		var turnNumber int64
		var text string
		err = rows.Scan(&player, &turnNumber, &text)
		if err != nil {
			return err
		}

		currentTurn, err := game.GetTurn(turnNumber)
		if err != nil {
			return err
		}

		mv, err := gotak.NewMove(text)
		if err != nil {
			return err
		}

		if player == gotak.PlayerWhite {
			if turnNumber > 1 {
				currentTurn.First = mv
			} else {
				currentTurn.Second = mv
			}
		}

		if player == gotak.PlayerBlack {
			if turnNumber > 1 {
				currentTurn.Second = mv
			} else {
				currentTurn.First = mv
			}
		}

		game.UpdateTurn(currentTurn)
	}

	return nil
}

func getMeta(db *sql.DB, game *gotak.Game) error {
	query := `SELECT key, value FROM tags WHERE game_id = $1 ORDER BY created_at`
	rows, err := db.Query(query, game.ID)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		var value string
		err = rows.Scan(&key, &value)
		if err != nil {
			return err
		}

		err = game.UpdateMeta(key, value)
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
func updateGameStatus(db *sql.DB, slug, status string, winner int) error {
	query := `UPDATE games SET status = $1, winner = $2 WHERE slug = $3`
	_, err := db.Exec(query, status, winner, slug)
	return err
}
