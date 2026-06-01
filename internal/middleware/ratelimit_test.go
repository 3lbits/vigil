package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIPRateLimiter_Wrap(t *testing.T) {
	limiter := NewIPRateLimiter(2, time.Minute)
	handler := limiter.Wrap(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/users", nil)
	req.RemoteAddr = "192.0.2.11:1234"

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		handler(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("request %d: got %d, want %d", i+1, w.Code, http.StatusNoContent)
		}
	}

	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("third request: got %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

func TestIPRateLimiter_Wrap_WindowResetAllowsAgain(t *testing.T) {
	limiter := NewIPRateLimiter(1, time.Minute)
	handler := limiter.Wrap(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/users", nil)
	req.RemoteAddr = "192.0.2.12:1234"

	first := httptest.NewRecorder()
	handler(first, req)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first request: got %d, want %d", first.Code, http.StatusNoContent)
	}

	second := httptest.NewRecorder()
	handler(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want %d", second.Code, http.StatusTooManyRequests)
	}

	v, ok := limiter.entries.Load("192.0.2.12")
	if !ok {
		t.Fatal("expected limiter entry to exist")
	}
	entry, ok := v.(*ipEntry)
	if !ok {
		t.Fatalf("unexpected entry type %T", v)
	}
	entry.mu.Lock()
	entry.resetAt = time.Now().Add(-time.Second)
	entry.mu.Unlock()

	third := httptest.NewRecorder()
	handler(third, req)
	if third.Code != http.StatusNoContent {
		t.Fatalf("third request after reset: got %d, want %d", third.Code, http.StatusNoContent)
	}
}

func TestIPRateLimiter_Wrap_MalformedRemoteAddrFallback(t *testing.T) {
	limiter := NewIPRateLimiter(1, time.Minute)
	handler := limiter.Wrap(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/users", nil)
	req.RemoteAddr = "192.0.2.13" // no port; should fall back to raw RemoteAddr

	first := httptest.NewRecorder()
	handler(first, req)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first request: got %d, want %d", first.Code, http.StatusNoContent)
	}

	second := httptest.NewRecorder()
	handler(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want %d", second.Code, http.StatusTooManyRequests)
	}
}
