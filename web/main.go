package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/unrolled/render.v1"
	"gopkg.in/unrolled/secure.v1"
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

	// SecureMiddlewareOptions is a set of defaults for securing web apps.
	// SSLRedirect is handeled by a different middleware because it does not
	// support whitelisting paths.
	SecureMiddlewareOptions = secure.Options{
		HostsProxyHeaders:    []string{"X-Forwarded-Host"},
		SSLRedirect:          false, // No way to whitelist for healthcheck :/
		SSLProxyHeaders:      map[string]string{"X-Forwarded-Proto": "https"},
		STSSeconds:           315360000,
		STSIncludeSubdomains: true,
		STSPreload:           true,
		FrameDeny:            true,
		ContentTypeNosniff:   true,
		BrowserXssFilter:     true,
		IsDevelopment:        os.Getenv("TAK_ENV") == "local",
	}
)

// SSLMiddleware redirects for all paths besides /healthz and /metrics. This is
// a slight modification of the code in
// https://github.com/unrolled/secure/blob/v1/secure.go
func SSLMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" && r.URL.Path != "/metrics" {
			ssl := strings.EqualFold(r.URL.Scheme, "https") || r.TLS != nil
			if !ssl {
				for k, v := range SecureMiddlewareOptions.SSLProxyHeaders {
					if r.Header.Get(k) == v {
						ssl = true
						break
					}
				}
			}

			if !ssl && !SecureMiddlewareOptions.IsDevelopment {
				url := r.URL
				url.Scheme = "https"
				url.Host = r.Host

				http.Redirect(w, r, url.String(), http.StatusMovedPermanently)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	port := "8080"
	if fromEnv := os.Getenv("PORT"); fromEnv != "" {
		port = fromEnv
	}
	log.Printf("Starting up on %s", port)

	secureMiddleware := secure.New(SecureMiddlewareOptions)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Turnstile and security when not local
	if !SecureMiddlewareOptions.IsDevelopment {
		r.Use(secureMiddleware.Handler)
		r.Use(SSLMiddleware)
	}

	db, err := getDB()
	if err != nil {
		log.Panic(err)
		return
	}

	// Metrics
	r.Get("/healthz", healthCheckHandler)
	r.Mount("/metrics", promhttp.Handler())

	r.Get("/", rootHandler)
	r.HandleFunc("/game/{slug}", getGameHandler)
	r.Get("/game/{slug}/{turn}", getTurnHandler)
	r.Post("/game/new", newGameHandler)
	r.Post("/game/{slug}/move", newMoveHandler)

	err = updateDB(db)
	if err != nil {
		log.Panic(err)
		return
	}

	log.Fatal(http.ListenAndServe(":"+port, r))
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	return
}

func newGameHandler(w http.ResponseWriter, r *http.Request) {
	db, err := getDB()
	if err != nil {
		log.Panic(err)
		return
	}

	var data map[string]int
	decoder := json.NewDecoder(r.Body)
	decoder.Decode(&data)

	slug, err := createGame(db, data["size"])
	if err != nil {
		log.Panic(err)
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
	return
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
	return
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
	return
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	Renderer.JSON(w, http.StatusOK, map[string]string{
		"healthy":  "true",
		"revision": os.Getenv("GIT_REVISION"),
		"tag":      os.Getenv("GIT_TAG"),
		"branch":   os.Getenv("GIT_BRANCH"),
	})
}
