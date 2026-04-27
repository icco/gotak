# gotak

[![Tests](https://github.com/icco/gotak/actions/workflows/test.yml/badge.svg)](https://github.com/icco/gotak/actions/workflows/test.yml)
[![CodeQL](https://github.com/icco/gotak/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/icco/gotak/actions/workflows/codeql-analysis.yml)
[![GoDoc](https://godoc.org/github.com/icco/gotak?status.svg)](https://godoc.org/github.com/icco/gotak)
[![Go Report Card](https://goreportcard.com/badge/github.com/icco/gotak)](https://goreportcard.com/report/github.com/icco/gotak)

A server, CLI, and core library for the board game [Tak](https://en.wikipedia.org/wiki/Tak_(game)),
including PTN parsing, full rules (roads, piece limits, win conditions), and a chi-based HTTP API
with PostgreSQL persistence.

## Binaries

| Path                  | Description                                                          |
|-----------------------|----------------------------------------------------------------------|
| `./cmd/server`        | HTTP API (chi + GORM + Swagger) backed by PostgreSQL.                |
| `./cmd/gotak`         | Bubble Tea TUI client for playing against humans or the local AI.    |
| `./cmd/parse-ptn`     | One-shot PTN parser/validator.                                       |

## API

| Method | Path                  | Description                                                                                                                |
|--------|-----------------------|----------------------------------------------------------------------------------------------------------------------------|
| `GET`  | `/`                   | HTML index generated from the Swagger spec.                                                                                |
| `GET`  | `/healthz`            | Liveness probe.                                                                                                            |
| `GET`  | `/swagger/*`          | Swagger UI for the OpenAPI spec.                                                                                           |
| `GET`  | `/game/{slug}`        | Game state. Public.                                                                                                        |
| `GET`  | `/game/{slug}/{turn}` | Game state at a specific turn. Public.                                                                                     |
| `POST` | `/game/new`           | Create a game (auth required). Body: `{"size": "8"}`.                                                                      |
| `POST` | `/game/{slug}/join`   | Join a waiting game as black (auth required).                                                                              |
| `POST` | `/game/{slug}/move`   | Submit a move (auth required). Body: `{"player": 1, "move": "c3", "turn": 1}`.                                             |
| `POST` | `/game/{slug}/ai-move`| Request an AI move (auth required).                                                                                        |
| `GET`  | `/auth/*`             | JWT + Google OAuth via `go-pkgz/auth`.                                                                                     |
| `GET`  | `/metrics`            | OTel HTTP semconv metrics (e.g. `http_server_request_duration_seconds`) in Prometheus exposition format.                   |

## Environment variables

| Variable               | Required | Default     | Description                                                       |
|------------------------|----------|-------------|-------------------------------------------------------------------|
| `PORT`                 | no       | `8080`      | HTTP listen port.                                                 |
| `DATABASE_URL`         | yes      | _(empty)_   | Postgres DSN. SQLite fallback in tests.                           |
| `AUTH_JWT_SECRET`      | yes      | _(empty)_   | HMAC secret for JWTs.                                             |
| `GOOGLE_CLIENT_ID`     | no       | _(empty)_   | Enables Google OAuth provider.                                    |
| `GOOGLE_CLIENT_SECRET` | no       | _(empty)_   | Pairs with `GOOGLE_CLIENT_ID`.                                    |
| `NAT_ENV`              | no       | _(empty)_   | Set to `production` to enable SSL redirect / strict headers.      |

## Running

```bash
export DATABASE_URL="postgres://user:password@localhost/gotak?sslmode=disable"
export AUTH_JWT_SECRET="$(openssl rand -hex 32)"
go run ./cmd/server
```

```bash
docker build -t gotak .
docker run --rm -p 8080:8080 \
  -e DATABASE_URL=... \
  -e AUTH_JWT_SECRET=... \
  gotak
```

```bash
go run ./cmd/gotak                       # TUI against https://gotak.app
go run ./cmd/gotak -- --local            # TUI against http://localhost:8080
go run ./cmd/parse-ptn -f test_games/foo.ptn
```

## Development

```bash
go build ./... && go vet ./... && go test ./...
golangci-lint run -E bodyclose,misspell,gosec,goconst,errorlint
swag init -g cmd/server/main.go -o cmd/server/docs   # regenerate OpenAPI
```

## Inspirations

- [PTN-Ninja](https://github.com/gruppler/PTN-Ninja) — PTN notation and analysis
- [PlayTak.org](http://playtak.org) — Online Tak platform
- [Taktician](https://github.com/nelhage/taktician) — Tak AI
- [Tak Subreddit Wiki](https://www.reddit.com/r/Tak/wiki/) — Rules and notation
- [Official Rules of Tak](https://ustak.org/play-beautiful-game-tak/)
