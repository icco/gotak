package main

import (
	"database/sql"

	"github.com/golang-migrate/migrate"
	"github.com/golang-migrate/migrate/database/postgres"
	_ "github.com/golang-migrate/migrate/source/file"
	"github.com/ifo/sanic"
	_ "github.com/lib/pq"
)

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

func createGame(db sql.DB) error {

	// Game Slug
	worker := sanic.NewWorker7()
	id := worker.NextID()
	slug := worker.IDString(id)

	query := `INSERT INTO games (slug) VALUES ($1)`
	_, err := db.Exec(query, slug)
	if err != nil {
		return err
	}

	return nil
}
