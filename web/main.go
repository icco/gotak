package main

import (
	"net/http"

	"github.com/go-chi/chi"
)

func main() {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("welcome"))
	})

	r.Get("/game/{id}/?", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("welcome")) })
	r.Get("/game/{id}/{move}/?", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("welcome")) })
	r.Post("/game/{id}/move/?", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("welcome")) })
	r.Post("/game/new", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("welcome")) })

	http.ListenAndServe(":3000", r)
}
