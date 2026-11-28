package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"go.icco.me/gutil/logging"
	"go.uber.org/zap"
)

// trustedProxies are the private ranges a reverse proxy in front of the
// server sits in. X-Forwarded-For hops from anywhere else are the client.
var trustedProxies = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"127.0.0.0/8",
	"::1/128",
	"fc00::/7",
}

// Per-client request budgets for the analysis endpoints. Engine endpoints
// can each burn seconds of CPU; the read endpoints scan every stored move.
const (
	engineRequestsPerMinute = 10
	readRequestsPerMinute   = 60
)

// engineSlotWait is how long a request queues for a free engine slot
// before giving up with 503.
const engineSlotWait = 5 * time.Second

// engineSlots bounds concurrent engine searches to the CPUs available, so
// a burst of analysis cannot starve game play.
var engineSlots = make(chan struct{}, max(1, runtime.GOMAXPROCS(0)))

// clientIPKey keys rate limits on the IP chi resolved from trusted proxies,
// falling back to the TCP peer when the chain could not be read.
func clientIPKey(r *http.Request) (string, error) {
	ip := middleware.GetClientIP(r.Context())
	if ip == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip = host
	}
	return httprate.CanonicalizeIP(ip), nil
}

func limitByClientIP(requests int, window time.Duration) func(http.Handler) http.Handler {
	return httprate.LimitBy(requests, window, clientIPKey,
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			writeLimitError(w, r, http.StatusTooManyRequests, "too many analysis requests, try again later")
		}))
}

// errEngineBusy means no engine slot freed up within engineSlotWait.
var errEngineBusy = errors.New("analysis engine is busy, try again later")

// acquireEngineSlot waits up to engineSlotWait for a free engine slot.
// Handlers call it after reading and validating the request, right before
// searching, so a slow client cannot hold a slot while sending its body.
func acquireEngineSlot(ctx context.Context) (func(), error) {
	return acquireSlot(ctx, engineSlots, engineSlotWait)
}

func acquireSlot(ctx context.Context, slots chan struct{}, wait time.Duration) (func(), error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-timer.C:
		return nil, errEngineBusy
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// writeEngineBusy answers a request that could not get an engine slot.
func writeEngineBusy(w http.ResponseWriter, r *http.Request, err error) {
	if !errors.Is(err, errEngineBusy) {
		return
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(engineSlotWait.Seconds())))
	writeLimitError(w, r, http.StatusServiceUnavailable, err.Error())
}

func writeLimitError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	l := logging.FromContext(r.Context())
	l.Warnw("analysis request limited", "status", status, "path", r.URL.Path)
	if err := Renderer.JSON(w, status, ErrorResponse{Error: msg}); err != nil {
		l.Errorw("failed to render JSON", zap.Error(err))
	}
}
