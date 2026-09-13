# AGENTS.md

Guidance for coding agents working on gotak.
See [CLAUDE.md](CLAUDE.md) for Claude Code entrypoint (`@AGENTS.md`).

## Project Overview

Tak board game implementation in Go, featuring a CLI AI opponent and a web server backed by PostgreSQL.

## Commands

```sh
# Testing & Linting
go test -v -cover ./...
go vet ./...
staticcheck ./...
go fmt ./...

# Building & Running
go build -o gotak-cli ./cmd/gotak    # Build CLI
go run ./cmd/server                  # Run server (requires DATABASE_URL)
```

## Architecture & Layout

- `cmd/gotak` — CLI interface with AI opponent.
- `cmd/server` — HTTP/web server with PostgreSQL persistence.
- `tak` / `lib` — Core game engine, move generation, and board evaluation rules.

## Conventions

- PR titles and commits must follow Conventional Commits with a lowercase subject.
- Run `go fmt ./...` and ensure `go test` and linters pass before committing.
