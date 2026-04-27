package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func TestHealthCheckHandler(t *testing.T) {
	twoHundreds := map[string]http.HandlerFunc{
		"/healthz": healthCheckHandler,
		"/":        rootHandler,
	}

	for route, handler := range twoHundreds {
		t.Run(route, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, route, http.NoBody)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			http.HandlerFunc(handler).ServeHTTP(rr, req)

			if status := rr.Code; status != http.StatusOK {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, http.StatusOK)
			}
		})
	}
}

// TestMetricsEndpoint asserts otelhttp's HTTP server histogram lands
// in /metrics tagged with the chi route pattern.
func TestMetricsEndpoint(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "test-secret")

	reg := prometheus.NewRegistry()
	exporter, err := otelprom.New(otelprom.WithRegisterer(reg))
	if err != nil {
		t.Fatalf("otelprom.New: %v", err)
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() {
		if err := mp.Shutdown(context.Background()); err != nil {
			t.Logf("meter provider shutdown: %v", err)
		}
	})

	h := buildRouter(routerOptions{
		IsDev:          true,
		MetricsHandler: promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/healthz") //nolint:noctx // test
	if err != nil {
		t.Fatalf("get healthz: %v", err)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Logf("drain healthz body: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Logf("close healthz body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", resp.StatusCode)
	}

	metricsResp, err := http.Get(srv.URL + "/metrics") //nolint:noctx // test
	if err != nil {
		t.Fatalf("get metrics: %v", err)
	}
	defer func() {
		if err := metricsResp.Body.Close(); err != nil {
			t.Logf("close metrics body: %v", err)
		}
	}()
	if metricsResp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", metricsResp.StatusCode)
	}
	raw, err := io.ReadAll(metricsResp.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	text := string(raw)

	for _, want := range []string{
		"http_server_request_duration_seconds",
		`http_route="/healthz"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("metrics body missing %q\nbody:\n%s", want, text)
		}
	}
}
