package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/icco/gotak"
	"github.com/icco/gotak/cmd/server/docs"
	"github.com/icco/gutil/logging"
	"github.com/microcosm-cc/bluemonday"
	"github.com/swaggo/http-swagger"
	"github.com/unrolled/render"
	"github.com/unrolled/secure"
	"go.uber.org/zap"
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
		Funcs:                     []template.FuncMap{},
	})

	log       = logging.Must(logging.NewLogger(gotak.Service))
	ugcPolicy = bluemonday.StrictPolicy()
)

// @title GoTak API
// @version 1.0
// @description A Tak game server API with authentication
// @contact.name API Support
// @contact.url http://github.com/icco/gotak
// @license.name MIT
// @license.url https://github.com/icco/gotak/blob/main/LICENSE
// @host gotak.app
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description JWT token in format: Bearer {token}

func main() {
	port := "8080"
	if fromEnv := os.Getenv("PORT"); fromEnv != "" {
		port = fromEnv
	}
	log.Infow("Starting up", "host", "https://gotak.app")

	isDev := os.Getenv("NAT_ENV") != "production"

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(logging.Middleware(log.Desugar(), gotak.GCPProject))

	_, err := getDB()
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

		// Public routes
		r.Get("/", rootHandler)
		r.Get("/healthz", healthCheckHandler)
		r.Get("/swagger/*", httpSwagger.Handler(
			httpSwagger.URL("https://gotak.app/swagger/doc.json"),
		))

		// Auth endpoints (JWT + Google)
		r.Mount("/auth", AuthRoutes())

		// Public game viewing (no auth required)
		r.Get("/game/{slug}", getGameHandler)
		r.Get("/game/{slug}/{turn}", getTurnHandler)

		// Protected routes requiring authentication
		r.Group(func(r chi.Router) {
			r.Use(authMiddleware)
			r.Get("/game/new", newGameHandler)
			r.Post("/game/new", newGameHandler)
			r.Post("/game/{slug}/join", joinGameHandler)
			r.Post("/game/{slug}/move", newMoveHandler)
			r.Post("/game/{slug}/ai-move", PostAIMoveHandler)
		})
	})

	// GORM auto-migration is handled in getDB()

	server := &http.Server{
		Addr:           ":" + port,
		Handler:        r,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}
	log.Fatal(server.ListenAndServe())
}

// @Summary Get API information
// @Description Returns basic API information and available endpoints
// @Tags info
// @Accept json
// @Produce html
// @Success 200 {string} string "HTML page with API information"
// @Router / [get]
func rootHandler(w http.ResponseWriter, r *http.Request) {
	// Use embedded swagger.json data from docs package
	spec, err := docs.GetSwaggerSpec()
	if err != nil {
		log.Errorw("failed to parse swagger.json", zap.Error(err))
		// Fallback to static content
		writeStaticHomePage(w)
		return
	}

	// Generate HTML with endpoint information
	html := `
<html>
  <head>
    <title>GoTak API</title>
    <style>
      body { font-family: Arial, sans-serif; max-width: 800px; margin: 40px auto; padding: 20px; }
      h1 { color: #333; }
      .endpoint { margin: 20px 0; padding: 15px; border-left: 4px solid #007acc; background: #f8f9fa; }
      .method { font-weight: bold; color: #007acc; text-transform: uppercase; }
      .path { font-family: monospace; color: #333; margin: 5px 0; }
      .description { color: #666; margin: 5px 0; }
      .tag { background: #e1ecf4; color: #39739d; padding: 2px 6px; border-radius: 3px; font-size: 0.8em; margin-right: 5px; }
      a { color: #007acc; text-decoration: none; }
      a:hover { text-decoration: underline; }
    </style>
  </head>
  <body>
    <h1>GoTak API</h1>
    <p>A Tak game server API providing endpoints for game management and gameplay.</p>
    <p><a href="/swagger/">📚 View Swagger Documentation</a></p>
    
    <h2>Available Endpoints</h2>`

	// Sort endpoints by path for consistent display
	for path, methods := range spec.Paths {
		for method, info := range methods {
			html += fmt.Sprintf(`
    <div class="endpoint">
      <div class="method">%s</div>
      <div class="path">%s</div>
      <div class="description">%s</div>
      <div>`, method, path, info.Description)

			for _, tag := range info.Tags {
				html += fmt.Sprintf(`<span class="tag">%s</span>`, tag)
			}

			html += `</div>
    </div>`
		}
	}

	html += `
  </body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(html)); err != nil {
		log.Errorw("failed to write response", zap.Error(err))
	}
}

func writeStaticHomePage(w http.ResponseWriter) {
	html := `
<html>
  <head>
    <title>GoTak API</title>
    <style>
      body { font-family: Arial, sans-serif; max-width: 800px; margin: 40px auto; padding: 20px; }
    </style>
  </head>
  <body>
    <h1>GoTak API</h1>
    <p>A Tak game server API</p>
    <p><a href="/swagger/">📚 View Swagger Documentation</a></p>
    <ul>
      <li>GET /game/{slug} - Get game state</li>
      <li>GET /game/{slug}/{turn} - Get specific turn</li>
      <li>GET /game/new - Create a new game</li>
      <li>POST /game/new - Create a new game</li>
      <li>POST /game/{slug}/move - Make a move in a game</li>
      <li>GET /healthz - Health check</li>
    </ul>
  </body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(html)); err != nil {
		log.Errorw("failed to write response", zap.Error(err))
	}
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
// @Success 307 {string} string "Redirect to game URL"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /game/new [get]
// @Router /game/new [post]
func newGameHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		if err := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "bad connection to db"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Get current user from context (set by authMiddleware)
	user := getMustUserFromContext(r)
	userID := user.ID

	boardSize := 8

	var data CreateGameRequest
	if err := json.NewDecoder(r.Body).Decode(&data); err == nil && data.Size != "" {
		i, err := strconv.Atoi(data.Size)
		if err == nil && i > 0 {
			boardSize = i
		}
	}

	slug, err := createGame(db, boardSize, userID)
	if err != nil {
		log.Errorw("could not create game", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": err.Error()}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/game/%s", slug), http.StatusTemporaryRedirect)
}

// @Summary Join a waiting game
// @Description Join a game that is waiting for a second player (as black player)
// @Tags game
// @Accept json
// @Produce json
// @Param slug path string true "Game slug identifier"
// @Success 200 {object} MessageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /game/{slug}/join [post]
func joinGameHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		if err := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "bad connection to db"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	ctx := r.Context()
	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))

	// Get current user (must be authenticated to reach this handler)
	user := getMustUserFromContext(r)

	// Attempt to join the game
	err = joinGame(db, slug, user.ID)
	if err != nil {
		log.Errorw("could not join game", "slug", slug, "user_id", user.ID, zap.Error(err))

		// Determine appropriate status code based on error
		statusCode := 500
		if strings.Contains(err.Error(), "already") || strings.Contains(err.Error(), "full") {
			statusCode = 400
		} else if strings.Contains(err.Error(), "can only join") {
			statusCode = 400
		}

		if err := Renderer.JSON(w, statusCode, map[string]string{"error": err.Error()}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	log.Infow("user joined game", "slug", slug, "user_id", user.ID)

	if err := Renderer.JSON(w, 200, map[string]string{
		"message": "successfully joined game",
		"slug":    slug,
		"player":  "black",
	}); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /game/{slug}/move [post]
func newMoveHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		if err := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "bad connection to db"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	ctx := r.Context()

	// Get current user (must be authenticated to reach this handler)
	user := getMustUserFromContext(r)

	// Get DB Entry
	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))

	// Verify user is a participant in the game
	if err := verifyGameParticipation(db, slug, user.ID); err != nil {
		log.Errorw("game participation verification failed", "slug", slug, "user_id", user.ID, zap.Error(err))
		if err := Renderer.JSON(w, 403, map[string]string{"error": "access denied: you are not a participant in this game"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	game, err := getGame(db, slug)
	if err != nil {
		log.Errorw("could not get game", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": err.Error()}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	var data MoveRequest

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		log.Errorw("could not read body", zap.Error(err))
		if err := Renderer.JSON(w, 400, map[string]string{"error": err.Error()}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if data.Text == "" {
		log.Errorw("empty request", "data", data)
		if err := Renderer.JSON(w, 400, map[string]string{"error": "empty move text"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Validate player
	if data.Player != gotak.PlayerWhite && data.Player != gotak.PlayerBlack {
		log.Errorw("invalid player", "player", data.Player)
		if err := Renderer.JSON(w, 400, map[string]string{"error": "invalid player"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Verify the user is playing as the correct player number
	userPlayerNumber, err := getPlayerNumber(db, slug, user.ID)
	if err != nil {
		log.Errorw("could not get player number", "slug", slug, "user_id", user.ID, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "internal server error"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if data.Player != userPlayerNumber {
		log.Errorw("player mismatch", "requested_player", data.Player, "user_player", userPlayerNumber)
		if err := Renderer.JSON(w, 403, map[string]string{"error": "you can only make moves as your assigned player"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Get current game state from database to check if it's the player's turn
	var dbGame Game
	if err := db.Where("slug = ?", slug).First(&dbGame).Error; err != nil {
		log.Errorw("could not get game state", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not verify game state"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Check if it's the player's turn
	if dbGame.CurrentPlayer != data.Player {
		log.Errorw("not player's turn", "current_player", dbGame.CurrentPlayer, "requested_player", data.Player)
		if err := Renderer.JSON(w, 400, map[string]string{"error": "it's not your turn"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Check if game is already over
	winner, gameOver := game.GameOver()
	if gameOver {
		log.Errorw("game already over", "winner", winner)
		if err := Renderer.JSON(w, 400, map[string]string{"error": fmt.Sprintf("game is over, winner: %d", winner)}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Replay existing moves to get current board state
	err = replayMoves(game)
	if err != nil {
		log.Errorw("could not replay moves", zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not replay game state"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Validate and execute the move
	err = game.DoSingleMove(data.Text, data.Player)
	if err != nil {
		log.Errorw("invalid move", "move", data.Text, "player", data.Player, zap.Error(err))
		if err := Renderer.JSON(w, 400, map[string]string{"error": fmt.Sprintf("invalid move: %v", err)}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Store the move in database
	currentTurn := int64(len(game.Turns))
	if currentTurn == 0 {
		currentTurn = 1
	}

	if err := insertMove(db, game.ID, data.Player, data.Text, currentTurn); err != nil {
		log.Errorw("could not insert move", "data", data, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not save move"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	// Check if game is now over and update status
	winner, gameOver = game.GameOver()
	if gameOver {
		err = updateGameStatus(db, game.Slug, "finished", winner)
		if err != nil {
			log.Errorw("could not update game status", zap.Error(err))
		}
	}

	// Reload game to get updated state
	game, err = getGame(db, slug)
	if err != nil {
		log.Errorw("could not reload game", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": "could not reload game"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if err := Renderer.JSON(w, http.StatusOK, game); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}

// @Summary Get game state
// @Description Returns the current state of a game
// @Tags game
// @Accept json
// @Produce json
// @Param slug path string true "Game slug identifier"
// @Success 200 {object} gotak.Game
// @Failure 500 {object} ErrorResponse
// @Router /game/{slug} [get]
func getGameHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		if err := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "bad connection to db"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	ctx := r.Context()

	// Get DB Entry
	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))
	game, err := getGame(db, slug)
	if err != nil {
		log.Errorw("could not get game", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": err.Error()}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if err := Renderer.JSON(w, http.StatusOK, game); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}

// @Summary Get specific turn
// @Description Returns the state of a game at a specific turn
// @Tags game
// @Accept json
// @Produce json
// @Param slug path string true "Game slug identifier"
// @Param turn path int true "Turn number"
// @Success 200 {object} gotak.Turn
// @Failure 500 {object} ErrorResponse
// @Router /game/{slug}/{turn} [get]
func getTurnHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Errorw("could not get db", zap.Error(err))
		if err := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "bad connection to db"}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	ctx := r.Context()

	// Get DB Entry
	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))
	game, err := getGame(db, slug)
	if err != nil {
		log.Errorw("could not get game", "slug", slug, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": err.Error()}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	turnStr := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "turn"))
	turnNum, err := strconv.ParseInt(turnStr, 10, 0)
	if err != nil {
		log.Errorw("could not parse turn", "slug", slug, "turn", turnStr, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": err.Error()}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}
	turn, err := game.GetTurn(turnNum)
	if err != nil {
		log.Errorw("could not get turn", "slug", slug, "turn", turnNum, zap.Error(err))
		if err := Renderer.JSON(w, 500, map[string]string{"error": err.Error()}); err != nil {
			log.Errorw("failed to render JSON", zap.Error(err))
		}
		return
	}

	if err := Renderer.JSON(w, http.StatusOK, turn); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}

// @Summary Health check
// @Description Returns service health status
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /healthz [get]
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if err := Renderer.JSON(w, http.StatusOK, HealthResponse{
		Healthy:  "true",
		Revision: os.Getenv("GIT_REVISION"),
		Tag:      os.Getenv("GIT_TAG"),
		Branch:   os.Getenv("GIT_BRANCH"),
	}); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	if err := Renderer.JSON(w, http.StatusNotFound, ErrorResponse{
		Error: "404: This page could not be found",
	}); err != nil {
		log.Errorw("failed to render JSON", zap.Error(err))
	}
}
