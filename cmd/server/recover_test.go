package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
