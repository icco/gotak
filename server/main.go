package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/icco/gotak"
	"github.com/icco/gutil/logging"
	"github.com/microcosm-cc/bluemonday"
	"github.com/swaggo/http-swagger"
	"github.com/unrolled/render"
	"github.com/unrolled/secure"
	"go.uber.org/zap"

	_ "github.com/icco/gotak/server/docs"
)

var (
	// Renderer is a renderer for all occasions. These are our preferred default options.
	// See:
	//  - https://github.com/unrolled/render/blob/v1/README.md
	//  - https://godoc.org/gopkg.in/unrolled/render.v1
	Renderer = render.New(render.Options{
		Charset:                   "UTF-8",
		Directory:                 "views",
		DisableHTTPErrorRendering: false,
		Extensions:                []string{".tmpl", ".html"},
		IndentJSON:                false,
		IndentXML:                 true,
		Layout:                    "layout",
		RequirePartials:           true,
		Funcs:                     []template.FuncMap{template.FuncMap{}},
	})

	log       = logging.Must(logging.NewLogger(gotak.Service))
	ugcPolicy = bluemonday.StrictPolicy()
)

// @title GoTak API
// @version 1.0
// @description A Tak game server API
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.url http://github.com/icco/gotak
// @license.name MIT
// @license.url https://github.com/icco/gotak/blob/main/LICENSE
// @host localhost:8080
// @BasePath /

func main() {
	port := "8080"
	if fromEnv := os.Getenv("PORT"); fromEnv != "" {
		port = fromEnv
	}
	log.Infow("Starting up", "host", fmt.Sprintf("http://localhost:%s", port))

	isDev := os.Getenv("NAT_ENV") != "production"

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(logging.Middleware(log.Desugar(), gotak.GCPProject))

	db, err := getDB()
	if err != nil {
		log.Panicw("could not get db", zap.Error(err))
		return
	}

	r.Use(cors.New(cors.Options{
		AllowCredentials:   true,
		OptionsPassthrough: true,
		AllowedOrigins:     []string{"*"},
		AllowedMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:     []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:     []string{"Link"},
		MaxAge:             300, // Maximum value not ignored by any of major browsers
	}).Handler)

	r.NotFound(notFoundHandler)

	// Stuff that does not ssl redirect
	r.Group(func(r chi.Router) {
		r.Use(secure.New(secure.Options{
			BrowserXssFilter:   true,
			ContentTypeNosniff: true,
			FrameDeny:          true,
			HostsProxyHeaders:  []string{"X-Forwarded-Host"},
			IsDevelopment:      isDev,
			SSLProxyHeaders:    map[string]string{"X-Forwarded-Proto": "https"},
		}).Handler)

		r.Get("/healthz", healthCheckHandler)
	})

	// Everything that does SSL only
	r.Group(func(r chi.Router) {
		r.Use(secure.New(secure.Options{
			BrowserXssFilter:     true,
			ContentTypeNosniff:   true,
			FrameDeny:            true,
			HostsProxyHeaders:    []string{"X-Forwarded-Host"},
			IsDevelopment:        isDev,
			SSLProxyHeaders:      map[string]string{"X-Forwarded-Proto": "https"},
			SSLRedirect:          !isDev,
			STSIncludeSubdomains: true,
			STSPreload:           true,
			STSSeconds:           315360000,
		}).Handler)

		r.Get("/", rootHandler)
		r.Get("/swagger/*", httpSwagger.Handler(
			httpSwagger.URL("http://localhost:8080/swagger/doc.json"),
		))
		r.HandleFunc("/game/{slug}", getGameHandler)
		r.Get("/game/{slug}/{turn}", getTurnHandler)
		r.Get("/game/new", newGameHandler)
		r.Post("/game/new", newGameHandler)
		r.Post("/game/{slug}/move", newMoveHandler)
	})

	if err := updateDB(db); err != nil {
		log.Panic(err)
		return
	}

	log.Fatal(http.ListenAndServe(":"+port, r))
}

// @Summary Get API information
// @Description Returns basic API information and available endpoints
// @Tags info
// @Accept json
// @Produce html
// @Success 200 {string} string "HTML page with API information"
// @Router / [get]
func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`
<html>
  <head>
    <title>GoTak</title>
  </head>
  <body>
    <h1>GoTak</h1>
    <ul>
      <li>Get "/game/{slug}"</li>
      <li>Get "/game/{slug}/{turn}"</li>
      <li>Get "/game/new"</li>
      <li>Post "/game/new"</li>
      <li>Post "/game/{slug}/move"</li>
    </ul>
  </body>
</html>
  `))
}

// CreateGameRequest represents the request body for creating a new game
type CreateGameRequest struct {
	Size string `json:"size" example:"8" description:"Board size (4-9)"`
}

// @Summary Create a new game
// @Description Creates a new Tak game with the specified board size
// @Tags game
// @Accept json
// @Produce json
// @Param game body CreateGameRequest false "Game configuration"
// @Success 307 {object} map[string]string "Redirect to game URL"
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /game/new [get]
// @Router /game/new [post]
func newGameHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		Renderer.JSON(w, 500, map[string]string{"error": "bad connection to db"})
		return
	}

	boardSize := 8

	var data CreateGameRequest
	if err := json.NewDecoder(r.Body).Decode(&data); err == nil && data.Size != "" {
		i, err := strconv.Atoi(data.Size)
		if err == nil && i > 0 {
			boardSize = i
		}
	}

	slug, err := createGame(db, boardSize)
	if err != nil {
		log.Errorw("could not create game", zap.Error(err))
		Renderer.JSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/game/%s", slug), http.StatusTemporaryRedirect)
}

// MoveRequest represents the request body for making a move
type MoveRequest struct {
	Player int    `json:"player" example:"1" description:"Player number (1 or 2)"`
	Text   string `json:"move" example:"c3" description:"Move in PTN notation"`
	Turn   int64  `json:"turn" example:"1" description:"Turn number"`
}

// @Summary Make a move in a game
// @Description Submit a move for a specific game
// @Tags game
// @Accept json
// @Produce json
// @Param slug path string true "Game slug identifier"
// @Param move body MoveRequest true "Move details"
// @Success 200 {object} gotak.Game
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /game/{slug}/move [post]
func newMoveHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		Renderer.JSON(w, 500, map[string]string{"error": "bad connection to db"})
		return
	}

	ctx := r.Context()

	// Get DB Entry
	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))
	id, err := getGameID(db, slug)
	if err != nil {
		log.Errorw("could not get game", "slug", slug, zap.Error(err))
		Renderer.JSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	var data MoveRequest

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		log.Errorw("could not read body", zap.Error(err))
		Renderer.JSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	if data.Text == "" {
		log.Errorw("empty request", "data", data)
		Renderer.JSON(w, 400, map[string]string{"error": "empty request"})
		return
	}

	if err := insertMove(db, id, data.Player, data.Text, data.Turn); err != nil {
		log.Errorw("bad insert", "data", data, zap.Error(err))
		return
	}

	game, err := getGame(db, slug)
	if err != nil {
		log.Errorw("bad get game", "slug", slug, zap.Error(err))
		return
	}

	Renderer.JSON(w, http.StatusOK, game)
}

// @Summary Get game state
// @Description Returns the current state of a game
// @Tags game
// @Accept json
// @Produce json
// @Param slug path string true "Game slug identifier"
// @Success 200 {object} gotak.Game
// @Failure 500 {object} map[string]string
// @Router /game/{slug} [get]
func getGameHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		Renderer.JSON(w, 500, map[string]string{"error": "bad connection to db"})
		return
	}

	ctx := r.Context()

	// Get DB Entry
	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))
	game, err := getGame(db, slug)
	if err != nil {
		log.Errorw("could not get game", "slug", slug, zap.Error(err))
		Renderer.JSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	Renderer.JSON(w, http.StatusOK, game)
}

// @Summary Get specific turn
// @Description Returns the state of a game at a specific turn
// @Tags game
// @Accept json
// @Produce json
// @Param slug path string true "Game slug identifier"
// @Param turn path int true "Turn number"
// @Success 200 {object} gotak.Turn
// @Failure 500 {object} map[string]string
// @Router /game/{slug}/{turn} [get]
func getTurnHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		Renderer.JSON(w, 500, map[string]string{"error": "bad connection to db"})
		return
	}

	ctx := r.Context()

	// Get DB Entry
	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))
	game, err := getGame(db, slug)
	if err != nil {
		log.Errorw("could not get game", "slug", slug, zap.Error(err))
		Renderer.JSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	turnStr := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "turn"))
	turnNum, err := strconv.ParseInt(turnStr, 10, 0)
	if err != nil {
		log.Errorw("could not parse turn", "slug", slug, "turn", turnStr, zap.Error(err))
		Renderer.JSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	turn, err := game.GetTurn(turnNum)
	if err != nil {
		log.Errorw("could not get turn", "slug", slug, "turn", turnNum, zap.Error(err))
		Renderer.JSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	Renderer.JSON(w, http.StatusOK, turn)
}

// @Summary Health check
// @Description Returns service health status
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Router /healthz [get]
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	Renderer.JSON(w, http.StatusOK, map[string]string{
		"healthy":  "true",
		"revision": os.Getenv("GIT_REVISION"),
		"tag":      os.Getenv("GIT_TAG"),
		"branch":   os.Getenv("GIT_BRANCH"),
	})
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	Renderer.JSON(w, http.StatusNotFound, map[string]string{
		"error": "404: This page could not be found",
	})
}
