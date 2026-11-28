package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverPanics_returns500(t *testing.T) {
	h := recoverPanics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("index out of range")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error == "" {
		t.Error("want an error message in the body")
	}
}

func TestRecoverPanics_passesThrough(t *testing.T) {
	h := recoverPanics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want 418", rec.Code)
	}
}

func TestRecoverPanics_keepsAbortHandler(t *testing.T) {
	h := recoverPanics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		rec := recover()
		err, ok := rec.(error)
		if !ok || !errors.Is(err, http.ErrAbortHandler) {
			t.Errorf("recovered %v, want http.ErrAbortHandler re-panicked", rec)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
}

func TestRecoverPanics_abortsCommittedResponse(t *testing.T) {
	cases := map[string]func(http.ResponseWriter){
		"after write": func(w http.ResponseWriter) {
			if _, err := w.Write([]byte("partial")); err != nil {
				t.Fatal(err)
			}
		},
		"after flush": func(w http.ResponseWriter) {
			w.(http.Flusher).Flush()
		},
	}
	for name, start := range cases {
		t.Run(name, func(t *testing.T) {
			h := recoverPanics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				start(w)
				panic("boom")
			}))
			rec := httptest.NewRecorder()
			defer func() {
				r := recover()
				err, ok := r.(error)
				if !ok || !errors.Is(err, http.ErrAbortHandler) {
					t.Errorf("recovered %v, want http.ErrAbortHandler", r)
				}
				if strings.Contains(rec.Body.String(), "internal server error") {
					t.Errorf("error JSON appended to a committed response: %q", rec.Body.String())
				}
			}()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		})
	}
}
