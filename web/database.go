package main

import (
	"database/sql"

	"github.com/golang-migrate/migrate"
	"github.com/golang-migrate/migrate/database/postgres"
	_ "github.com/golang-migrate/migrate/source/file"
	"github.com/icco/gotak"
	"github.com/ifo/sanic"
	_ "github.com/lib/pq"
)

func getDB() (*sql.DB, error) {
	return sql.Open("postgres", "postgres://localhost/gotak?sslmode=disable")
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

func createGame(db *sql.DB) (string, error) {

	// Game Slug
	worker := sanic.NewWorker7()
	id := worker.NextID()
	slug := worker.IDString(id)

	query := `INSERT INTO games (slug) VALUES ($1)`
	_, err := db.Exec(query, slug)
	if err != nil {
		return "", err
	}

	return slug, nil
}

func getGame(db *sql.DB, slug string) (*gotak.Game, error) {
	query := `SELECT id FROM games WHERE slug = $1`
	stmt, err := db.Prepare(query)
	if err != nil {
		return nil, err
	}

	var id int64
	err = stmt.QueryRow(slug).Scan(&id)
	if err != nil {
		return nil, err
	}

	game := &gotak.Game{
		ID:   id,
		Slug: slug,
	}

	// TODO: Get Turns and Meta

	return game, nil
}
