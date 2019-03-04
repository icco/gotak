package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"

	"contrib.go.opencensus.io/exporter/stackdriver"
	"contrib.go.opencensus.io/exporter/stackdriver/monitoredresource"
	"contrib.go.opencensus.io/exporter/stackdriver/propagation"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/icco/gotak"
	"github.com/unrolled/render"
	"github.com/unrolled/secure"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
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

	log = gotak.InitLogging()
)

func main() {
	port := "8080"
	if fromEnv := os.Getenv("PORT"); fromEnv != "" {
		port = fromEnv
	}
	log.Printf("Starting up on %s", port)

	labels := &stackdriver.Labels{}
	labels.Set("app", "gotak", "The name of the app.")
	sd, err := stackdriver.NewExporter(stackdriver.Options{
		ProjectID:               "icco-cloud",
		MonitoredResource:       monitoredresource.Autodetect(),
		DefaultMonitoringLabels: labels,
		DefaultTraceAttributes:  map[string]interface{}{"/http/host": "gotak.app"},
	})

	if err != nil {
		log.WithError(err).Fatalf("Failed to create the Stackdriver exporter: %v", err)
	}
	defer sd.Flush()

	view.RegisterExporter(sd)
	trace.RegisterExporter(sd)
	trace.ApplyConfig(trace.Config{
		DefaultSampler: trace.AlwaysSample(),
	})

	isDev := os.Getenv("NAT_ENV") != "production"

	r := chi.NewRouter()

	db, err := getDB()
	if err != nil {
		log.Panic(err)
		return
	}

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(gotak.LoggingMiddleware())

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
		r.HandleFunc("/game/{slug}", getGameHandler)
		r.Get("/game/{slug}/{turn}", getTurnHandler)
		r.Get("/game/new", newGameHandler)
		r.Post("/game/new", newGameHandler)
		r.Post("/game/{slug}/move", newMoveHandler)
	})

	err = updateDB(db)
	if err != nil {
		log.Panic(err)
		return
	}

	h := &ochttp.Handler{
		Handler:     r,
		Propagation: &propagation.HTTPFormat{},
	}
	if err := view.Register(ochttp.DefaultServerViews...); err != nil {
		log.Fatal("Failed to register ochttp.DefaultServerViews")
	}

	log.Fatal(http.ListenAndServe(":"+port, h))
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
}

func newGameHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Panic(err)
		return
	}

	boardSize := 8

	var data map[string]string
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&data)

	if err == nil && data != nil && data["size"] != "" {
		i, err := strconv.Atoi(data["size"])
		if err == nil && i > 0 {
			boardSize = i
		}
	}

	slug, err := createGame(db, boardSize)
	if err != nil {
		log.Error(err)
		Renderer.JSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/game/%s", slug), http.StatusTemporaryRedirect)
}

func newMoveHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Panic(err)
		return
	}

	ctx := r.Context()

	// Get DB Entry
	slug := chi.URLParamFromCtx(ctx, "slug")
	id, err := getGameID(db, slug)
	if err != nil {
		log.Printf("%+v : %+v", slug, err)
		return
	}

	decoder := json.NewDecoder(r.Body)

	var data struct {
		Player int    `json:"player"`
		Text   string `json:"move"`
		Turn   int64  `json:"turn"`
	}

	err = decoder.Decode(&data)
	if err != nil {
		log.Printf("%+v : %+v", r.Body, err)
		return
	}

	if data.Text == "" {
		log.Printf("empty data")
		return
	}

	err = insertMove(db, id, data.Player, data.Text, data.Turn)
	if err != nil {
		log.Printf("insert: %+v", err)
		return
	}

	game, err := getGame(db, slug)
	if err != nil {
		log.Printf("%+v : %+v", slug, err)
		return
	}

	Renderer.JSON(w, http.StatusOK, game)
}

func getGameHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Panic(err)
		return
	}

	ctx := r.Context()

	// Get DB Entry
	slug := chi.URLParamFromCtx(ctx, "slug")
	game, err := getGame(db, slug)
	if err != nil {
		log.Printf("%+v : %+v", slug, err)
		return
	}

	Renderer.JSON(w, http.StatusOK, game)
}

func getTurnHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Panic(err)
		return
	}

	ctx := r.Context()

	// Get DB Entry
	slug := chi.URLParamFromCtx(ctx, "slug")
	game, err := getGame(db, slug)
	if err != nil {
		log.Printf("%+v : %+v", slug, err)
		return
	}

	turnNum, err := strconv.ParseInt(chi.URLParamFromCtx(ctx, "turn"), 10, 0)
	if err != nil {
		log.Printf("parsing turn: %+v", err)
		return
	}
	turn, err := game.GetTurn(turnNum)
	if err != nil {
		log.Printf("%+v : %+v", slug, err)
		return
	}

	Renderer.JSON(w, http.StatusOK, turn)
}

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
