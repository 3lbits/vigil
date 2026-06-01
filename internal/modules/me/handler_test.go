package me

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/modregistry"
	"github.com/3lbits/vigil/internal/testutil"
)

type meQ struct {
	testutil.StubQuerier
	user        db.User
	userErr     error
	acts        []db.ListOwnedActivitiesRow
	actsErr     error
	measures    []db.ListOwnedMeasuresRow
	measuresErr error
	risks       []db.ListOwnedRisksRow
	risksErr    error
	org         db.Organization
	orgErr      error
}

func (q *meQ) GetUserByID(_ context.Context, _ uuid.UUID) (db.User, error) {
	return q.user, q.userErr
}
func (q *meQ) ListOwnedActivities(_ context.Context, _ uuid.NullUUID) ([]db.ListOwnedActivitiesRow, error) {
	return q.acts, q.actsErr
}
func (q *meQ) ListOwnedMeasures(_ context.Context, _ uuid.NullUUID) ([]db.ListOwnedMeasuresRow, error) {
	return q.measures, q.measuresErr
}
func (q *meQ) ListOwnedRisks(_ context.Context, _ uuid.NullUUID) ([]db.ListOwnedRisksRow, error) {
	return q.risks, q.risksErr
}
func (q *meQ) GetOrganization(_ context.Context, _ uuid.UUID) (db.Organization, error) {
	return q.org, q.orgErr
}

func meCtx(r *http.Request, id string) *http.Request {
	return r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: id, Name: "Alice", Email: "alice@example.com", Role: "editor",
	}))
}

func TestShow_MissingUserContext(t *testing.T) {
	h := NewHandler(&meQ{})
	r := httptest.NewRequest(http.MethodGet, "/me", nil)
	w := httptest.NewRecorder()

	h.Show(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestShow_InvalidUserID_RendersProfileOnly(t *testing.T) {
	h := NewHandler(&meQ{})
	r := meCtx(httptest.NewRequest(http.MethodGet, "/me", nil), "not-a-uuid")
	w := httptest.NewRecorder()

	h.Show(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestShow_ListOwnedActivitiesError(t *testing.T) {
	h := NewHandler(&meQ{actsErr: errors.New("db down")})
	r := meCtx(httptest.NewRequest(http.MethodGet, "/me", nil), uuid.NewString())
	w := httptest.NewRecorder()

	h.Show(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestShow_OK(t *testing.T) {
	orgID := uuid.New()
	h := NewHandler(&meQ{
		user:     db.User{Provider: "entra_id", OrgID: uuid.NullUUID{UUID: orgID, Valid: true}},
		org:      db.Organization{Name: "Acme"},
		acts:     []db.ListOwnedActivitiesRow{},
		measures: []db.ListOwnedMeasuresRow{},
		risks:    []db.ListOwnedRisksRow{},
	})
	r := meCtx(httptest.NewRequest(http.MethodGet, "/me", nil), uuid.NewString())
	w := httptest.NewRecorder()

	h.Show(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestModuleContract(t *testing.T) {
	m := New()
	if got := m.Name(); got != "me" {
		t.Fatalf("module name = %q, want %q", got, "me")
	}
	mux := http.NewServeMux()
	r := modregistry.NewRegistrar(mux, nil)
	if err := m.Register(modregistry.Dependencies{}, r); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	meta := r.Meta()
	if len(meta) != 1 || meta[0].Pattern != "GET /me" {
		t.Fatalf("unexpected route meta: %#v", meta)
	}
}
