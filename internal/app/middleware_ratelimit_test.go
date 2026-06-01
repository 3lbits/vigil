package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/3lbits/vigil/internal/config"
)

func TestGlobalRateLimitMiddleware_ExemptRoutesBypassLimit(t *testing.T) {
	cfg := &config.Config{
		GlobalRateLimitPerWindow: 1,
		GlobalRateLimitWindow:    time.Minute,
		MetricsPath:              "/metrics",
	}
	handler := globalRateLimitMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.RemoteAddr = "192.0.2.20:5555"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("request %d to healthz: got %d, want %d", i+1, w.Code, http.StatusNoContent)
		}
	}
}

func TestGlobalRateLimitMiddleware_PrivateRouteLimited(t *testing.T) {
	cfg := &config.Config{
		GlobalRateLimitPerWindow: 1,
		GlobalRateLimitWindow:    time.Minute,
	}
	handler := globalRateLimitMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/measures", nil)
	req.RemoteAddr = "192.0.2.21:1234"

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, req)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first request: got %d, want %d", first.Code, http.StatusNoContent)
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want %d", second.Code, http.StatusTooManyRequests)
	}
}

func TestGlobalRateLimitMiddleware_DisabledPassesThrough(t *testing.T) {
	cfg := &config.Config{
		GlobalRateLimitPerWindow: 0,
		GlobalRateLimitWindow:    time.Minute,
	}
	handler := globalRateLimitMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/measures", nil)
		req.RemoteAddr = "192.0.2.22:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("request %d: got %d, want %d", i+1, w.Code, http.StatusNoContent)
		}
	}
}
