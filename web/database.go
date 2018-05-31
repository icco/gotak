package main

import (
	"database/sql"

	"github.com/golang-migrate/migrate"
	"github.com/golang-migrate/migrate/database/postgres"
	_ "github.com/golang-migrate/migrate/source/file"
	_ "github.com/lib/pq"
)

func updateDB() error {
	db, err := sql.Open("postgres", "postgres://localhost/gotak?sslmode=disable")
	if err != nil {
		return err
	}

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
