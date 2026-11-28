package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func TestLimitConcurrency_busyReturns503(t *testing.T) {
	slots := make(chan struct{}, 1)
	release := make(chan struct{})
	entered := make(chan struct{})
	var once sync.Once
	h := limitConcurrency(slots, 50*time.Millisecond, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		once.Do(func() { close(entered) })
		<-release
		w.WriteHeader(http.StatusOK)
	}))

	first := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(first, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil))
		close(done)
	}()
	<-entered

	second := httptest.NewRecorder()
	h.ServeHTTP(second, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil))
	if second.Code != http.StatusServiceUnavailable {
		t.Errorf("second request = %d, want 503", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Error("503 should carry Retry-After")
	}

	close(release)
	<-done
	if first.Code != http.StatusOK {
		t.Errorf("first request = %d, want 200", first.Code)
	}

	third := httptest.NewRecorder()
	h.ServeHTTP(third, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil))
	if third.Code != http.StatusOK {
		t.Errorf("slot was not released: third request = %d", third.Code)
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
