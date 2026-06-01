package activities

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/testutil"
)

type guardQ struct {
	testutil.StubQuerier
	activity       db.GetActivityRow
	getErr         error
	isParticipant  bool
	participantErr error
}

func (q *guardQ) GetActivity(_ context.Context, _ uuid.UUID) (db.GetActivityRow, error) {
	return q.activity, q.getErr
}

func (q *guardQ) IsParticipant(_ context.Context, _ db.IsParticipantParams) (bool, error) {
	return q.isParticipant, q.participantErr
}

func testEngineForOwnUpdate(t *testing.T) *authz.Engine {
	t.Helper()
	e, err := authz.New(context.Background(), `
package authz
import rego.v1
default allow := false
allow if {
	input.user.role == "contributor"
	input.action == "update_own"
	input.resource == "activities"
	input.is_participant == true
}
`)
	if err != nil {
		t.Fatalf("compile authz policy: %v", err)
	}
	return e
}

func contributorReq(t *testing.T, pathID string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/activities/"+pathID, nil)
	r.SetPathValue("id", pathID)
	return r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID:   "00000000-0000-0000-0000-000000000123",
		Role: "contributor",
	}))
}

func TestRequireActivityUpdateOwn_AllowsParticipantAndInjectsActivity(t *testing.T) {
	id := uuid.New()
	q := &guardQ{
		activity:      db.GetActivityRow{ID: id, Title: "A"},
		isParticipant: true,
	}
	guard := RequireActivityUpdateOwn(q, testEngineForOwnUpdate(t))
	next := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := activityFromContext(r.Context())
		if !ok {
			t.Fatal("expected activity in context")
		}
		if a.ID != id {
			t.Fatalf("activity ID: got %s want %s", a.ID, id)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, contributorReq(t, id.String()))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusNoContent)
	}
}

func TestRequireActivityUpdateOwn_DeniesNonParticipant(t *testing.T) {
	id := uuid.New()
	q := &guardQ{
		activity:      db.GetActivityRow{ID: id},
		isParticipant: false,
	}
	guard := RequireActivityUpdateOwn(q, testEngineForOwnUpdate(t))
	next := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, contributorReq(t, id.String()))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireActivityUpdateOwn_BadIDNotFound(t *testing.T) {
	q := &guardQ{}
	guard := RequireActivityUpdateOwn(q, testEngineForOwnUpdate(t))
	next := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, contributorReq(t, "bad-id"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusNotFound)
	}
}

func TestRequireActivityUpdateOwn_ActivityNotFound(t *testing.T) {
	id := uuid.New()
	q := &guardQ{getErr: sql.ErrNoRows}
	guard := RequireActivityUpdateOwn(q, testEngineForOwnUpdate(t))
	next := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, contributorReq(t, id.String()))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusNotFound)
	}
}

func TestRequireActivityUpdateOwn_DBError500(t *testing.T) {
	id := uuid.New()
	q := &guardQ{getErr: errors.New("db down")}
	guard := RequireActivityUpdateOwn(q, testEngineForOwnUpdate(t))
	next := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, contributorReq(t, id.String()))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusInternalServerError)
	}
}
