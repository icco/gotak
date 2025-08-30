# CLAUDE.md

## Development Commands

```bash
# Testing & Linting
go test -v -cover ./...
go vet ./...
staticcheck ./...
yq -iP '.' file.yml

# Building & Running
go build -o gotak-cli ./cmd/gotak
go run ./cmd/server
```

## Architecture

**Tak game server with CLI, web API, and AI**

- CLI: `./gotak` (AI opponent)
- Server: `cmd/server/` (PostgreSQL, set `DATABASE_URL`)
