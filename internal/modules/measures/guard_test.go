package measures

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/testutil"
	"github.com/google/uuid"
)

type guardQ struct {
	testutil.StubQuerier
	measure        db.Measure
	getErr         error
	isParticipant  bool
	participantErr error
}

func (q *guardQ) GetMeasure(_ context.Context, _ uuid.UUID) (db.Measure, error) {
	return q.measure, q.getErr
}

func (q *guardQ) IsParticipant(_ context.Context, _ db.IsParticipantParams) (bool, error) {
	return q.isParticipant, q.participantErr
}

func guardEngine(t *testing.T) *authz.Engine {
	t.Helper()
	e, err := authz.New(context.Background(), `
package authz
import rego.v1
default allow := false
allow if {
	input.user.role == "contributor"
	input.action == "update_own"
	input.resource == "measures"
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
	r := httptest.NewRequest(http.MethodPost, "/measures/"+pathID, nil)
	r.SetPathValue("id", pathID)
	return r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID:   "00000000-0000-0000-0000-000000000123",
		Role: "contributor",
	}))
}

func TestRequireMeasureUpdateOwn_AllowsParticipantAndInjectsMeasure(t *testing.T) {
	id := uuid.New()
	q := &guardQ{
		measure:       db.Measure{ID: id, Name: "A"},
		isParticipant: true,
	}
	guard := RequireMeasureUpdateOwn(q, guardEngine(t))
	next := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m, ok := measureFromContext(r.Context())
		if !ok {
			t.Fatal("expected measure in context")
		}
		if m.ID != id {
			t.Fatalf("measure ID: got %s want %s", m.ID, id)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, contributorReq(t, id.String()))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusNoContent)
	}
}

func TestRequireMeasureUpdateOwn_DeniesNonParticipant(t *testing.T) {
	id := uuid.New()
	q := &guardQ{
		measure:       db.Measure{ID: id},
		isParticipant: false,
	}
	guard := RequireMeasureUpdateOwn(q, guardEngine(t))
	next := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, contributorReq(t, id.String()))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireMeasureUpdateOwn_BadIDNotFound(t *testing.T) {
	q := &guardQ{}
	guard := RequireMeasureUpdateOwn(q, guardEngine(t))
	next := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, contributorReq(t, "bad-id"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusNotFound)
	}
}

func TestRequireMeasureUpdateOwn_MeasureNotFound(t *testing.T) {
	id := uuid.New()
	q := &guardQ{getErr: sql.ErrNoRows}
	guard := RequireMeasureUpdateOwn(q, guardEngine(t))
	next := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, contributorReq(t, id.String()))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusNotFound)
	}
}

func TestRequireMeasureUpdateOwn_DBError500(t *testing.T) {
	id := uuid.New()
	q := &guardQ{getErr: errors.New("db down")}
	guard := RequireMeasureUpdateOwn(q, guardEngine(t))
	next := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, contributorReq(t, id.String()))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusInternalServerError)
	}
}
