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

## Taktician gotchas

- Taktician packs boards into 64-bit bitboards, so the engine supports 3x3 to 8x8 only, while the rules (and `gotak.Board`) allow 9x9.
- Use `ptn.FormatMove` and `ptn.ParseMove` from `github.com/nelhage/taktician/ptn` for engine moves. In PTN a slide without drop counts lands the whole carry on the first square.
- `ptn.ParseTPS` panics on empty cells and bare `S`/`C` markers, so validate untrusted TPS with `gotak.ParseTPS` first.
- Turn 1 places the opponent's stone. `tak.Position` handles this itself, so replay moves in order without swapping colors.
