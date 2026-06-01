package httputil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/3lbits/vigil/internal/middleware"
)

func TestError_FullPage(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/missing", nil)
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-ID", "req-123")

	NotFound(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d", http.StatusNotFound, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "HTTP 404") {
		t.Fatalf("expected full page to include HTTP status, body=%q", body)
	}
	if !strings.Contains(body, "req-123") {
		t.Fatalf("expected request id in body, body=%q", body)
	}
}

func TestError_HTMXInline(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/restricted", nil)
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()

	Forbidden(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "HTTP 403") {
		t.Fatalf("expected inline fragment status, body=%q", body)
	}
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Fatalf("expected inline fragment for HTMX, got full page")
	}
}

func TestError_FullPage_IncludesTopNav(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/missing", nil)
	w := httptest.NewRecorder()

	NotFound(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d", http.StatusNotFound, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Error navigation") {
		t.Fatalf("expected top nav to be present, body=%q", body)
	}
	if !strings.Contains(body, "HTTP 404") {
		t.Fatalf("expected full page to include HTTP status, body=%q", body)
	}
}

func TestError_FullPage_WithUserContext_UsesAppLayout(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/missing", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID:    "u1",
		Name:  "Dev User",
		Role:  "admin",
		Email: "dev@example.com",
	}))
	w := httptest.NewRecorder()

	NotFound(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d", http.StatusNotFound, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Main navigation") {
		t.Fatalf("expected app layout navigation, body=%q", body)
	}
	if strings.Contains(body, "Error navigation") {
		t.Fatalf("expected app layout instead of fallback error nav")
	}
}
