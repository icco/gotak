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

## Features

### ✅ Implemented

- **Complete Game Logic**
  - Full Tak rules implementation (board sizes 4x4 to 9x9)
  - Road detection using flood-fill algorithm
  - Piece limits and carry restrictions
  - Win condition detection (road wins, flat stone wins)
  - First-turn rule enforcement (place opponent's stone)

- **PTN (Portable Tak Notation) Support**
  - Parse PTN files and notation strings
  - Generate PTN from game state
  - Support for drops: `a1`, `Ca1` (capstone), `Sa1` (standing stone)
  - Support for moves: `3a3+3`, `4a4>121` (with drop counts)
  - Game metadata and comments

- **REST API**
  - `GET /` - Dynamic home page with endpoint documentation
  - `GET /game/{slug}` - Get current game state
  - `GET /game/{slug}/{turn}` - Get game state at specific turn
  - `GET /game/new` - Create new game (also supports POST)
  - `POST /game/{slug}/move` - Submit move
  - `GET /healthz` - Health check endpoint
  - `GET /swagger/*` - Interactive API documentation

- **Database Storage**
  - PostgreSQL with automatic migrations
  - Game persistence with slugs
  - Move history tracking
  - Game metadata storage

- **Web Interface**
  - Styled home page with endpoint summaries
  - Swagger UI integration
  - Responsive design with proper CSS styling

- **Developer Experience**
  - Comprehensive test coverage (69.3%)
  - Real PTN game files for testing
  - Automated CI/CD with GitHub Actions
  - Security analysis with CodeQL
  - Input sanitization and security headers

### Applications

The project includes three main applications:

1. **Web Server** (`./server`) - Production HTTP API server
2. **CLI Tool** (`./cmd/gotak`) - Command-line game demonstration
3. **PTN Parser** (`./cmd/parse-ptn`) - Parse and validate PTN files

## Quick Start

### Prerequisites

- Go 1.23+
- PostgreSQL (for the server)
- Optional: Docker for containerized deployment

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

### Make a Move

```bash
curl -X POST http://localhost:8080/game/{slug}/move \
  -H "Content-Type: application/json" \
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

