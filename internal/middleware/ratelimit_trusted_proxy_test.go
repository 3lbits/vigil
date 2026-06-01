package middleware_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/obs"
)

func TestIPRateLimiter_TrustedProxy_UsesXFFClientIP(t *testing.T) {
	_, trusted, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatalf("parse trusted cidr: %v", err)
	}

	limiter := middleware.NewIPRateLimiterWithKey(1, time.Minute, func(r *http.Request) string {
		return obs.SourceIPFromContext(r.Context())
	})
	base := limiter.Wrap(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := obs.SourceIPMiddlewareWithTrustedCIDRs([]*net.IPNet{trusted})(base)

	reqA1 := httptest.NewRequest(http.MethodPost, "/auth/github", nil)
	reqA1.RemoteAddr = "10.1.2.3:443"
	reqA1.Header.Set("X-Forwarded-For", "198.51.100.10")

	reqB1 := httptest.NewRequest(http.MethodPost, "/auth/github", nil)
	reqB1.RemoteAddr = "10.1.2.3:443"
	reqB1.Header.Set("X-Forwarded-For", "198.51.100.11")

	reqA2 := httptest.NewRequest(http.MethodPost, "/auth/github", nil)
	reqA2.RemoteAddr = "10.1.2.3:443"
	reqA2.Header.Set("X-Forwarded-For", "198.51.100.10")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, reqA1)
	if w.Code != http.StatusNoContent {
		t.Fatalf("client A first request: got %d, want %d", w.Code, http.StatusNoContent)
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, reqB1)
	if w.Code != http.StatusNoContent {
		t.Fatalf("client B first request: got %d, want %d", w.Code, http.StatusNoContent)
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, reqA2)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("client A second request: got %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

func TestIPRateLimiter_UntrustedProxy_IgnoresSpoofedXFF(t *testing.T) {
	_, trusted, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatalf("parse trusted cidr: %v", err)
	}

	limiter := middleware.NewIPRateLimiterWithKey(1, time.Minute, func(r *http.Request) string {
		return obs.SourceIPFromContext(r.Context())
	})
	base := limiter.Wrap(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := obs.SourceIPMiddlewareWithTrustedCIDRs([]*net.IPNet{trusted})(base)

	req1 := httptest.NewRequest(http.MethodPost, "/admin/users", nil)
	req1.RemoteAddr = "203.0.113.9:443" // not in trusted CIDR
	req1.Header.Set("X-Forwarded-For", "198.51.100.10")

	req2 := httptest.NewRequest(http.MethodPost, "/admin/users", nil)
	req2.RemoteAddr = "203.0.113.9:443" // same untrusted source
	req2.Header.Set("X-Forwarded-For", "198.51.100.11")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req1)
	if w.Code != http.StatusNoContent {
		t.Fatalf("first request: got %d, want %d", w.Code, http.StatusNoContent)
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req2)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}
