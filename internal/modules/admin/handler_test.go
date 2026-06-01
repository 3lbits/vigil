package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/testutil"
	"github.com/google/uuid"
)

// ── test doubles ──────────────────────────────────────────────────────────────

type adminQ struct {
	testutil.StubQuerier
	users    []db.User
	listErr  error
	roleErr  error
	roleUser db.User
	sessions []db.ListActiveSessionsByUserRow
	sessErr  error
}

func (q *adminQ) ListUsers(_ context.Context) ([]db.User, error) {
	return q.users, q.listErr
}
func (q *adminQ) SetUserRole(_ context.Context, _ db.SetUserRoleParams) (db.User, error) {
	return q.roleUser, q.roleErr
}
func (q *adminQ) ListActiveSessionsByUser(_ context.Context) ([]db.ListActiveSessionsByUserRow, error) {
	return q.sessions, q.sessErr
}

type noopPinger struct{}

func (noopPinger) Ping(_ context.Context) error { return nil }

func newTestHandler(q *adminQ) *Handler {
	return NewHandler(q, noopPinger{}, time.Now(), "test")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func adminCtx(r *http.Request) *http.Request {
	ctx := middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "00000000-0000-0000-0000-000000000001", Name: "Alice", Role: "admin",
	})
	return r.WithContext(ctx)
}

// ── AdminPage ─────────────────────────────────────────────────────────────────

func TestAdminPage_OK(t *testing.T) {
	q := &adminQ{users: []db.User{{ID: uuid.New(), Name: "Bob", Role: "viewer"}}}
	h := newTestHandler(q)

	r := httptest.NewRequest(http.MethodGet, "/admin", nil)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.AdminPage(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAdminPage_DBError(t *testing.T) {
	q := &adminQ{listErr: errors.New("db down")}
	h := newTestHandler(q)

	r := httptest.NewRequest(http.MethodGet, "/admin", nil)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.AdminPage(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── SetRole ───────────────────────────────────────────────────────────────────

func TestSetRole_OK(t *testing.T) {
	uid := uuid.New()
	q := &adminQ{}
	h := newTestHandler(q)

	form := url.Values{"role": {"editor"}}
	r := httptest.NewRequest(http.MethodPost, "/admin/users/"+uid.String()+"/role", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", uid.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.SetRole(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "success") {
		t.Errorf("expected success flash in redirect, got %q", loc)
	}
}

func TestSetRole_InvalidRole(t *testing.T) {
	uid := uuid.New()
	h := newTestHandler(&adminQ{})

	form := url.Values{"role": {"superuser"}}
	r := httptest.NewRequest(http.MethodPost, "/admin/users/"+uid.String()+"/role", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", uid.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.SetRole(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSetRole_InvalidID(t *testing.T) {
	h := newTestHandler(&adminQ{})

	form := url.Values{"role": {"editor"}}
	r := httptest.NewRequest(http.MethodPost, "/admin/users/not-a-uuid/role", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", "not-a-uuid")
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.SetRole(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminPage_SessionsDBError(t *testing.T) {
	q := &adminQ{
		users:   []db.User{{ID: uuid.New(), Name: "Bob", Role: "viewer"}},
		sessErr: errors.New("sessions db down"),
	}
	h := newTestHandler(q)

	r := httptest.NewRequest(http.MethodGet, "/admin", nil)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.AdminPage(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("sessions DB error: expected 500, got %d", w.Code)
	}
}

func TestSetRole_DBError(t *testing.T) {
	uid := uuid.New()
	q := &adminQ{roleErr: errors.New("db down")}
	h := newTestHandler(q)

	form := url.Values{"role": {"editor"}}
	r := httptest.NewRequest(http.MethodPost, "/admin/users/"+uid.String()+"/role", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", uid.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.SetRole(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect on error, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "error") {
		t.Errorf("expected error flash in redirect, got %q", loc)
	}
}
