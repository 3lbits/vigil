package compliance

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
	framework         db.Framework
	requirement       db.Requirement
	getFrameworkErr   error
	getRequirementErr error
	isParticipant     bool
	isParticipantErr  error
}

func (q *guardQ) GetFramework(_ context.Context, _ uuid.UUID) (db.Framework, error) {
	return q.framework, q.getFrameworkErr
}

func (q *guardQ) GetRequirement(_ context.Context, _ uuid.UUID) (db.Requirement, error) {
	return q.requirement, q.getRequirementErr
}

func (q *guardQ) IsParticipant(_ context.Context, _ db.IsParticipantParams) (bool, error) {
	return q.isParticipant, q.isParticipantErr
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
	input.resource in {"frameworks", "requirements"}
	input.is_participant == true
}
`)
	if err != nil {
		t.Fatalf("compile authz policy: %v", err)
	}
	return e
}

func contributorReq(t *testing.T, path string, id string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, nil)
	r.SetPathValue("id", id)
	return r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID:   "00000000-0000-0000-0000-000000000123",
		Role: "contributor",
	}))
}

func TestRequireFrameworkUpdateOwn_Allowed(t *testing.T) {
	id := uuid.New()
	q := &guardQ{
		framework:     db.Framework{ID: id, Name: "ISO"},
		isParticipant: true,
	}
	guard := RequireComplianceUpdateOwn("frameworks", q, guardEngine(t))
	next := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fw, ok := frameworkFromContext(r.Context())
		if !ok || fw.ID != id {
			t.Fatal("expected framework in context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, contributorReq(t, "/compliance/frameworks/"+id.String(), id.String()))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusNoContent)
	}
}

func TestRequireFrameworkUpdateOwn_Denied(t *testing.T) {
	id := uuid.New()
	q := &guardQ{framework: db.Framework{ID: id}, isParticipant: false}
	guard := RequireComplianceUpdateOwn("frameworks", q, guardEngine(t))
	next := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, contributorReq(t, "/compliance/frameworks/"+id.String(), id.String()))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireRequirementUpdateOwn_Allowed(t *testing.T) {
	id := uuid.New()
	q := &guardQ{
		requirement:   db.Requirement{ID: id},
		isParticipant: true,
	}
	guard := RequireComplianceUpdateOwn("requirements", q, guardEngine(t))
	next := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, ok := requirementFromContext(r.Context())
		if !ok || req.ID != id {
			t.Fatal("expected requirement in context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, contributorReq(t, "/compliance/requirements/"+id.String(), id.String()))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusNoContent)
	}
}

func TestRequireRequirementUpdateOwn_NotFoundAndErrors(t *testing.T) {
	id := uuid.New()
	engine := guardEngine(t)

	tests := []struct {
		name   string
		q      *guardQ
		want   int
		pathID string
	}{
		{name: "bad id", q: &guardQ{}, want: http.StatusNotFound, pathID: "bad-id"},
		{name: "not found", q: &guardQ{getRequirementErr: sql.ErrNoRows}, want: http.StatusNotFound, pathID: id.String()},
		{name: "db error", q: &guardQ{getRequirementErr: errors.New("db down")}, want: http.StatusInternalServerError, pathID: id.String()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			next := RequireComplianceUpdateOwn("requirements", tc.q, engine)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			w := httptest.NewRecorder()
			next.ServeHTTP(w, contributorReq(t, "/compliance/requirements/"+tc.pathID, tc.pathID))
			if w.Code != tc.want {
				t.Fatalf("status: got %d want %d", w.Code, tc.want)
			}
		})
	}
}
