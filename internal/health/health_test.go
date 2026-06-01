package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(_ context.Context) error { return f.err }

// ── Liveness ──────────────────────────────────────────────────────────────────

func TestLiveness_OK(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	Liveness(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ── Readiness ─────────────────────────────────────────────────────────────────

func TestReadiness_OK(t *testing.T) {
	SetModuleFlagsSettingsLoadFailure(false)
	t.Cleanup(func() { SetModuleFlagsSettingsLoadFailure(false) })

	h := Readiness(fakePinger{})

	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	h(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestReadiness_DBDown(t *testing.T) {
	SetModuleFlagsSettingsLoadFailure(false)
	t.Cleanup(func() { SetModuleFlagsSettingsLoadFailure(false) })

	h := Readiness(fakePinger{err: errors.New("connection refused")})

	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	h(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestReadiness_ModuleFlagsDegraded(t *testing.T) {
	SetModuleFlagsSettingsLoadFailure(true)
	t.Cleanup(func() { SetModuleFlagsSettingsLoadFailure(false) })

	h := Readiness(fakePinger{})
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	h(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}
