package dashboard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/testutil"
)

// ── test doubles ──────────────────────────────────────────────────────────────

type dashboardQ struct {
	testutil.StubQuerier
	stats      db.GetDashboardStatsRow
	statsErr   error
	frameworks []db.Framework
	fwErr      error
}

func (q *dashboardQ) GetDashboardStats(_ context.Context) (db.GetDashboardStatsRow, error) {
	return q.stats, q.statsErr
}
func (q *dashboardQ) ListFrameworks(_ context.Context) ([]db.Framework, error) {
	return q.frameworks, q.fwErr
}
func (q *dashboardQ) CountRequirementsByFramework(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}
func (q *dashboardQ) CountCoveredRequirementsByFramework(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}
func (q *dashboardQ) ListRecentActivities(_ context.Context) ([]db.ListRecentActivitiesRow, error) {
	return nil, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func adminCtx(r *http.Request) *http.Request {
	ctx := middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "00000000-0000-0000-0000-000000000001", Name: "Alice", Role: "admin",
	})
	return r.WithContext(ctx)
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestDashboard_OK(t *testing.T) {
	q := &dashboardQ{
		stats: db.GetDashboardStatsRow{
			FrameworksCount:   2,
			RequirementsCount: 10,
			MeasuresCount:     5,
			ImplementedCount:  3,
		},
		frameworks: []db.Framework{
			{ID: uuid.New(), Name: "ISO 27001", ShortName: "ISO", Version: "2022"},
		},
	}
	h := NewHandler(q)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Dashboard(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestDashboard_StatsError(t *testing.T) {
	q := &dashboardQ{statsErr: errors.New("db down")}
	h := NewHandler(q)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Dashboard(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestDashboard_FrameworksError(t *testing.T) {
	q := &dashboardQ{fwErr: errors.New("db down")}
	h := NewHandler(q)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Dashboard(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestDashboard_WrongPath(t *testing.T) {
	h := NewHandler(&dashboardQ{})

	r := httptest.NewRequest(http.MethodGet, "/not-root", nil)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Dashboard(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
