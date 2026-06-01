package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/testutil"
	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
)

type userMWQ struct {
	testutil.StubQuerier
	user db.User
	err  error
}

func (q *userMWQ) GetUserByID(_ context.Context, _ uuid.UUID) (db.User, error) {
	return q.user, q.err
}

func withSessionCtx(t *testing.T, sm *scs.SessionManager, r *http.Request, userID string) *http.Request {
	t.Helper()
	ctx, err := sm.Load(r.Context(), "")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if userID != "" {
		sm.Put(ctx, "userID", userID)
	}
	return r.WithContext(ctx)
}

func TestUserMiddleware_NoSessionUser(t *testing.T) {
	sm := scs.New()
	q := &userMWQ{}
	var hadUser bool

	h := UserMiddleware(sm, q)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, hadUser = middleware.FromContext(r.Context())
	}))
	r := withSessionCtx(t, sm, httptest.NewRequest(http.MethodGet, "/", nil), "")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if hadUser {
		t.Fatal("expected no user in context")
	}
}

func TestUserMiddleware_InvalidUUID(t *testing.T) {
	sm := scs.New()
	q := &userMWQ{}
	var hadUser bool

	h := UserMiddleware(sm, q)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, hadUser = middleware.FromContext(r.Context())
	}))
	r := withSessionCtx(t, sm, httptest.NewRequest(http.MethodGet, "/", nil), "not-a-uuid")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if hadUser {
		t.Fatal("expected no user in context")
	}
}

func TestUserMiddleware_DBLookupError(t *testing.T) {
	sm := scs.New()
	q := &userMWQ{err: errors.New("db down")}
	var hadUser bool
	uid := uuid.New().String()

	h := UserMiddleware(sm, q)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, hadUser = middleware.FromContext(r.Context())
	}))
	r := withSessionCtx(t, sm, httptest.NewRequest(http.MethodGet, "/", nil), uid)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if hadUser {
		t.Fatal("expected no user in context on DB lookup failure")
	}
}

func TestUserMiddleware_ValidUserInjected(t *testing.T) {
	sm := scs.New()
	userID := uuid.New()
	q := &userMWQ{
		user: db.User{
			ID:    userID,
			Name:  "Alice",
			Email: "alice@example.com",
			Role:  "admin",
		},
	}
	var got middleware.SessionUser
	var ok bool

	h := UserMiddleware(sm, q)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, ok = middleware.FromContext(r.Context())
	}))
	r := withSessionCtx(t, sm, httptest.NewRequest(http.MethodGet, "/", nil), userID.String())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if !ok {
		t.Fatal("expected user in context")
	}
	if got.ID != userID.String() || got.Email != "alice@example.com" || got.Role != "admin" {
		t.Fatalf("unexpected session user: %+v", got)
	}
}

func TestSessionIdleTimeout_ExpiresSessionAfterInactivity(t *testing.T) {
	sm := scs.New()
	sm.IdleTimeout = 20 * time.Millisecond

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx, err := sm.Load(r.Context(), "")
	if err != nil {
		t.Fatalf("load fresh session: %v", err)
	}
	sm.Put(ctx, "userID", uuid.New().String())
	token, _, err := sm.Commit(ctx)
	if err != nil {
		t.Fatalf("commit session: %v", err)
	}

	time.Sleep(60 * time.Millisecond)

	expiredCtx, err := sm.Load(r.Context(), token)
	if err != nil {
		t.Fatalf("load expired session: %v", err)
	}
	if got := sm.GetString(expiredCtx, "userID"); got != "" {
		t.Fatalf("expected expired session userID to be empty, got %q", got)
	}
}
