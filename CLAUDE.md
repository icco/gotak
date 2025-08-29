# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Testing
```bash
go test -v -cover ./...
```

### Linting and Code Quality
```bash
go vet ./...
# Note: staticcheck may have version compatibility issues with Go 1.24+
# staticcheck -go 1.17 ./...
go run github.com/fzipp/gocyclo/cmd/gocyclo -avg .
```

### Building and Running
```bash
# Build the main CLI application
go build -o gotak ./cmd/gotak

# Run the PTN parser
go run ./cmd/parse-ptn

# Start the web server
go run ./server

# Generate Swagger documentation (after making API changes)
swag init -g server/main.go -o server/docs

# Install swag tool globally (if not already installed)
go install github.com/swaggo/swag/cmd/swag@latest
```

### Database Operations
The server requires a PostgreSQL database. Set `DATABASE_URL` environment variable and run migrations with:
```bash
# Migrations are in db/migrations/
# Server automatically runs migrations on startup
```

## Architecture Overview

This is a Tak game server implementation with the following key components:

### Core Game Logic (`*.go` files in root)
- **Game (`game.go`)**: Main game state management, PTN parsing, and game rules
- **Board (`board.go`)**: Board state, move validation, and road detection using flood fill algorithm
- **Move (`move.go`)**: PTN move notation parsing and validation
- **Stone (`stone.go`)**: Stone types (Flat, Standing, Capstone) and player constants
- **Turn (`turn.go`)**: Individual turn representation with moves and comments

### Applications
- **CLI Tool (`cmd/gotak/`)**: Simple game demonstration and testing
- **PTN Parser (`cmd/parse-ptn/`)**: Parses PTN (Portable Tak Notation) files
- **Web Server (`server/`)**: HTTP API for multiplayer games with PostgreSQL storage

### Server Architecture
- **REST API** with endpoints:
  - `GET /` - Dynamic home page with endpoint summaries from swagger.json
  - `GET /healthz` - Health check endpoint
  - `GET /swagger/*` - Swagger UI documentation
  - `GET /game/{slug}` - Get current game state
  - `GET /game/{slug}/{turn}` - Get game state at specific turn
  - `GET /game/new` - Create new game (redirects after creation)
  - `POST /game/new` - Create new game (accepts JSON body with size)
  - `POST /game/{slug}/move` - Submit move
- **Database layer** with PostgreSQL for game persistence
- **Middleware stack** includes CORS, security headers, logging, and request validation
- **Dynamic UI**: Home page reads swagger.json to display endpoint information with fallback
- **CI/CD**: Automated Swagger documentation updates via GitHub Actions

### Key Technical Details
- Uses PTN (Portable Tak Notation) for move representation
- Board sizes from 4x4 to 9x9 (configurable)
- Implements complete Tak rules including road detection
- Thread-safe game state management
- Database migrations handled automatically on server startup
- Security: input sanitization with bluemonday policy
- Dynamic home page that reads swagger.json for endpoint documentation
- Styled web interface with fallback when swagger.json unavailable
- Go 1.23+ with toolchain 1.24.6

### Game Flow
1. Create game with specified board size
2. Players alternate placing stones (first turn places opponent's stone)
3. Moves parsed from PTN notation: placement `(stone)(square)` or movement `(count)(square)(direction)(drops)(stone)`
4. Win condition: continuous road from one edge to opposite edge
5. Game history stored as turns with individual moves

### Testing
- Comprehensive test coverage (69.3% overall coverage)
- Test games in `test_games/` directory with real PTN files from actual games
- Unit tests for move parsing, board state, and game rules
- Server tests for HTTP endpoints with database mocking
- Game logic validation with stress tests

### CI/CD and Documentation
- **GitHub Actions**:
  - CodeQL security analysis on push/PR to main
  - Automatic Swagger documentation updates on API changes
  - Test suite runs on all PRs and pushes
- **Swagger Documentation**: Auto-generated API docs served at `/swagger/`
- **Workflow triggers**: Documentation updates when Go files in `server/` or core game files change
- **Home Page**: Dynamically reads swagger.json to display endpoint documentation

### Recent Improvements
- **Dynamic Home Page**: Home page now reads from swagger.json to display endpoint summaries with styling
- **Enhanced UI**: Added CSS styling for professional appearance with endpoint tags and descriptions
- **Robust Fallback**: Graceful degradation when swagger.json is unavailable
- **Database Schema**: Added game status tracking with migrations
- **Security**: Comprehensive input sanitization and security headers

### Database Schema
- `games`: Store game metadata with ID, slug, and creation timestamp
- `moves`: Track all game moves with player, turn, and PTN text
- `tags`: Store game metadata as key-value pairs
- Automatic migrations on server startup

### Environment Variables
- `DATABASE_URL`: PostgreSQL connection string (required for server)
- `PORT`: Server port (defaults to 8080)
- `NAT_ENV`: Set to "production" to enable SSL redirects

### Performance Notes
- Game state reconstruction happens on-demand by replaying moves
- Database queries optimized for game retrieval and move insertion
- Concurrent request handling with Chi router
- Memory-efficient board representation using maps

# Development Guidelines
- Simple commands are always best, prefer them to long command strings with && or ||
- Always run tests before committing changes
- Update swagger documentation after API changes
- Follow existing code patterns and conventions

- Remember to commit and push often while you work