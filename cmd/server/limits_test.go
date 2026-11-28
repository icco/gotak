package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func TestAcquireSlot(t *testing.T) {
	slots := make(chan struct{}, 1)

	release, err := acquireSlot(t.Context(), slots, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := acquireSlot(t.Context(), slots, 50*time.Millisecond); !errors.Is(err, errEngineBusy) {
		t.Errorf("second acquire err = %v, want errEngineBusy", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := acquireSlot(ctx, slots, time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled acquire err = %v, want context.Canceled", err)
	}

	release()
	again, err := acquireSlot(t.Context(), slots, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("slot was not released: %v", err)
	}
	again()
}

func TestWriteEngineBusy(t *testing.T) {
	rec := httptest.NewRecorder()
	writeEngineBusy(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil), errEngineBusy)
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
		t.Errorf("busy = %d with Retry-After %q, want 503 with Retry-After", rec.Code, rec.Header().Get("Retry-After"))
	}

	gone := httptest.NewRecorder()
	writeEngineBusy(gone, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil), context.Canceled)
	if gone.Body.Len() != 0 {
		t.Errorf("canceled client got a body: %q", gone.Body.String())
	}
}

func TestLimitByClientIP(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := middleware.ClientIPFromXFF(trustedProxies...)(limitByClientIP(2, time.Minute)(ok))

	send := func(remote, xff string) int {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/analyze/position", nil)
		req.RemoteAddr = remote
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for i, xff := range []string{"", "198.51.100.1", "198.51.100.2"} {
		want := http.StatusOK
		if i == 2 {
			want = http.StatusTooManyRequests
		}
		if got := send("203.0.113.9:4000", xff); got != want {
			t.Errorf("direct request %d (spoofed XFF %q) = %d, want %d", i, xff, got, want)
		}
	}

	for i := range 2 {
		if got := send("172.18.0.2:5000", "198.51.100.7"); got != http.StatusOK {
			t.Errorf("proxied alice request %d = %d, want 200", i, got)
		}
	}
	if got := send("172.18.0.2:5000", "198.51.100.7"); got != http.StatusTooManyRequests {
		t.Errorf("third proxied alice request = %d, want 429", got)
	}
	if got := send("172.18.0.2:5000", "198.51.100.8"); got != http.StatusOK {
		t.Errorf("bob behind the same proxy = %d, want 200", got)
	}
}
