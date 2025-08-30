# GoTak

[![Tests](https://github.com/icco/gotak/actions/workflows/test.yml/badge.svg)](https://github.com/icco/gotak/actions/workflows/test.yml) [![CodeQL](https://github.com/icco/gotak/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/icco/gotak/actions/workflows/codeql-analysis.yml) [![GoDoc](https://godoc.org/github.com/icco/gotak?status.svg)](https://godoc.org/github.com/icco/gotak) [![Go Report Card](https://goreportcard.com/badge/github.com/icco/gotak)](https://goreportcard.com/report/github.com/icco/gotak)

A complete Tak game server implementation in Go with REST API, database persistence, and comprehensive game logic.

## Overview

GoTak is a production-ready server for the board game [Tak](https://en.wikipedia.org/wiki/Tak_(game)), providing:

- **Complete Tak Game Logic**: Full implementation of official Tak rules including road detection, piece limits, and win conditions
- **PTN Support**: Parse and generate Portable Tak Notation (PTN) for game recording and replay
- **REST API**: HTTP endpoints for game creation, move submission, and state retrieval
- **Database Persistence**: PostgreSQL storage with automatic migrations
- **Web Interface**: Dynamic home page with endpoint documentation
- **Developer Tools**: Comprehensive testing, Swagger documentation, and CI/CD integration

### Applications

The project includes three main applications:

1. **Web Server** (`./cmd/server`) - Production HTTP API server
2. **CLI Tool** (`./cmd/gotak`) - Command-line game demonstration
3. **PTN Parser** (`./cmd/parse-ptn`) - Parse and validate PTN files

## Quick Start

### Running the Server

```bash
# Set database connection
export DATABASE_URL="postgres://user:password@localhost/gotak?sslmode=disable"

# Start the server
go run ./server

# Or build and run
go build -o gotak-server ./server
./gotak-server
```

The server will start on port 8080 and automatically run database migrations.

### Running Tests

```bash
# Run all tests with coverage
go test -v -cover ./...

# Run specific package tests
go test -v ./server
```

### Building Applications

```bash
# Build the main CLI tool
go build -o gotak ./cmd/gotak

# Build the PTN parser
go build -o parse-ptn ./cmd/parse-ptn

# Build the server
go build -o gotak-server ./server
```

## API Usage

### Create a New Game

```bash
curl -X POST http://localhost:8080/game/new \
  -H "Content-Type: application/json" \
  -d '{"size": "8"}'
```

### Join a Game

```bash
curl -X POST http://localhost:8080/game/{slug}/join \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Make a Move

```bash
curl -X POST http://localhost:8080/game/{slug}/move \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"player": 1, "move": "a1", "turn": 1}'
```

### Get Game State

```bash
curl http://localhost:8080/game/{slug}
```

## Architecture

- **Core Logic**: Game rules, board state, and PTN parsing in root `*.go` files
- **Server**: HTTP API with Chi router, middleware, and PostgreSQL storage
- **Database**: Schema with games, moves, and tags tables
- **Security**: Input sanitization, CORS, security headers
- **Documentation**: Auto-generated Swagger docs with GitHub Actions integration

## Development

See [CLAUDE.md](./CLAUDE.md) for detailed development commands and architecture information.

## Inspirations

- [PTN-Ninja](https://github.com/gruppler/PTN-Ninja) - PTN notation and game analysis
- [PlayTak.org](http://playtak.org) - Online Tak platform
- [Taktician](https://github.com/nelhage/taktician) - Tak AI implementation
- [Tak Subreddit Wiki](https://www.reddit.com/r/Tak/wiki/) - Rules and notation references
- [Official Rules of Tak](https://ustak.org/play-beautiful-game-tak/)

