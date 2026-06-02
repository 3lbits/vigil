package avvik

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/testutil"
)

type avvikGuardQ struct {
	testutil.StubQuerier
	avvik  db.Avvik
	events []db.AvvikEvent
}

func (q *avvikGuardQ) GetAvvik(_ context.Context, _ uuid.UUID) (db.Avvik, error) {
	return q.avvik, nil
}

func (q *avvikGuardQ) ListAvvikEvents(_ context.Context, _ uuid.UUID) ([]db.AvvikEvent, error) {
	return q.events, nil
}

func avvikGuardEngine(t *testing.T) *authz.Engine {
	t.Helper()
	e, err := authz.New(context.Background(), `
package authz
import rego.v1
default allow := false

allow if {
	input.user.role == "admin"
}

allow if {
	input.resource == "avvik"
	input.action == "submit_own"
	input.user.role in {"contributor", "editor"}
	input.is_participant == true
}
`)
	if err != nil {
		t.Fatalf("compile authz engine: %v", err)
	}
	return e
}

func avvikGuardRequest(avvikID, userID uuid.UUID, role, email string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/avvik/"+avvikID.String()+"/notes", nil)
	r.SetPathValue("id", avvikID.String())
	return r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID:    userID.String(),
		Role:  role,
		Email: email,
	}))
}

func TestRequireAvvikSubmitOwn_ReporterAllowed(t *testing.T) {
	avvikID := uuid.New()
	q := &avvikGuardQ{
		avvik: db.Avvik{ID: avvikID, ReporterEmail: "reporter@example.com"},
	}
	guard := RequireAvvikSubmitOwn(q, avvikGuardEngine(t))
	w := httptest.NewRecorder()
	r := avvikGuardRequest(avvikID, uuid.New(), "contributor", "reporter@example.com")

	guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("reporter: got %d want %d", w.Code, http.StatusNoContent)
	}
}

func TestRequireAvvikSubmitOwn_CreatorAllowed(t *testing.T) {
	avvikID := uuid.New()
	creatorID := uuid.New()
	q := &avvikGuardQ{
		avvik: db.Avvik{ID: avvikID},
		events: []db.AvvikEvent{
			{EventType: "created", ActorID: uuid.NullUUID{UUID: creatorID, Valid: true}},
		},
	}
	guard := RequireAvvikSubmitOwn(q, avvikGuardEngine(t))
	w := httptest.NewRecorder()
	r := avvikGuardRequest(avvikID, creatorID, "contributor", "other@example.com")

	guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("creator: got %d want %d", w.Code, http.StatusNoContent)
	}
}

func TestRequireAvvikSubmitOwn_UnrelatedContributorDenied(t *testing.T) {
	avvikID := uuid.New()
	q := &avvikGuardQ{
		avvik: db.Avvik{ID: avvikID, ReporterEmail: "reporter@example.com"},
	}
	guard := RequireAvvikSubmitOwn(q, avvikGuardEngine(t))
	w := httptest.NewRecorder()
	r := avvikGuardRequest(avvikID, uuid.New(), "contributor", "other@example.com")

	guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("unrelated: got %d want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireAvvikSubmitOwn_AdminAlwaysAllowed(t *testing.T) {
	avvikID := uuid.New()
	q := &avvikGuardQ{
		avvik: db.Avvik{ID: avvikID},
	}
	guard := RequireAvvikSubmitOwn(q, avvikGuardEngine(t))
	w := httptest.NewRecorder()
	r := avvikGuardRequest(avvikID, uuid.New(), "admin", "admin@example.com")

	guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("admin: got %d want %d", w.Code, http.StatusNoContent)
	}
}
