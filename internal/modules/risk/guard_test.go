package risk

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

type riskGuardQ struct {
	testutil.StubQuerier
	assessment    db.RiskAssessment
	participant   bool
	assessmentErr error
}

func (q *riskGuardQ) GetRiskAssessment(_ context.Context, _ uuid.UUID) (db.RiskAssessment, error) {
	if q.assessmentErr != nil {
		return db.RiskAssessment{}, q.assessmentErr
	}
	return q.assessment, nil
}

func (q *riskGuardQ) IsRiskAssessmentParticipant(_ context.Context, _ db.IsRiskAssessmentParticipantParams) (bool, error) {
	return q.participant, nil
}

func riskGuardEngine(t *testing.T) *authz.Engine {
	t.Helper()
	e, err := authz.New(context.Background(), `
package authz
import rego.v1
default allow := false

allow if {
	input.resource == "risks"
	input.action in {"accept", "decline"}
	input.user.role in {"editor", "admin"}
	input.is_owner == true
}

allow if {
	input.resource == "risk"
	input.action == "read_scoped"
	input.user.role in {"contributor", "editor", "admin"}
}

allow if {
	input.resource == "risk"
	input.action == "read_scoped"
	input.user.role == "viewer"
	input.is_public == true
}

allow if {
	input.resource == "risk"
	input.action == "read_scoped"
	input.user.role == "viewer"
	input.is_participant == true
}

allow if {
	input.resource == "risk"
	input.action == "read_scoped"
	input.user.role == "viewer"
	input.is_owner == true
}

allow if {
	input.resource == "risk"
	input.action == "read_scoped"
	input.user.role == "viewer"
	input.is_creator == true
}
`)
	if err != nil {
		t.Fatalf("compile authz engine: %v", err)
	}
	return e
}

func requestWithPathAndUser(path, id, userID, role string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.SetPathValue("id", id)
	return r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID:   userID,
		Name: "Tester",
		Role: role,
	}))
}

func TestRequireRiskOwnerDecision_AcceptOwnerAllowed(t *testing.T) {
	ownerID := uuid.New()
	assessmentID := uuid.New()
	q := &riskGuardQ{
		assessment: db.RiskAssessment{
			ID:          assessmentID,
			RiskOwnerID: uuid.NullUUID{UUID: ownerID, Valid: true},
		},
	}
	guard := RequireRiskOwnerDecision("accept", q, riskGuardEngine(t))
	w := httptest.NewRecorder()
	r := requestWithPathAndUser("/risks/"+assessmentID.String()+"/accept", assessmentID.String(), ownerID.String(), "editor")

	guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusNoContent)
	}
}

func TestRequireRiskOwnerDecision_AcceptNonOwnerDenied(t *testing.T) {
	ownerID := uuid.New()
	userID := uuid.New()
	assessmentID := uuid.New()
	q := &riskGuardQ{
		assessment: db.RiskAssessment{
			ID:          assessmentID,
			RiskOwnerID: uuid.NullUUID{UUID: ownerID, Valid: true},
		},
	}
	guard := RequireRiskOwnerDecision("accept", q, riskGuardEngine(t))
	w := httptest.NewRecorder()
	r := requestWithPathAndUser("/risks/"+assessmentID.String()+"/accept", assessmentID.String(), userID.String(), "editor")

	guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireRiskReadScoped_ViewerVisibilityMatrix(t *testing.T) {
	assessmentID := uuid.New()
	viewerID := uuid.New()
	tests := []struct {
		name          string
		assessment    db.RiskAssessment
		participant   bool
		assessmentErr error
		wantStatus    int
	}{
		{
			name: "allowed when public",
			assessment: db.RiskAssessment{
				ID:       assessmentID,
				IsPublic: true,
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name: "allowed when participant",
			assessment: db.RiskAssessment{
				ID:       assessmentID,
				IsPublic: false,
			},
			participant: true,
			wantStatus:  http.StatusNoContent,
		},
		{
			name: "allowed when owner",
			assessment: db.RiskAssessment{
				ID:          assessmentID,
				RiskOwnerID: uuid.NullUUID{UUID: viewerID, Valid: true},
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name: "allowed when creator",
			assessment: db.RiskAssessment{
				ID:        assessmentID,
				CreatedBy: uuid.NullUUID{UUID: viewerID, Valid: true},
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name: "denied when disconnected and private",
			assessment: db.RiskAssessment{
				ID:       assessmentID,
				IsPublic: false,
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:          "missing assessment returns 404",
			assessmentErr: sql.ErrNoRows,
			wantStatus:    http.StatusNotFound,
		},
		{
			name:          "query failure returns 500",
			assessmentErr: errors.New("boom"),
			wantStatus:    http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := &riskGuardQ{
				assessment:    tc.assessment,
				participant:   tc.participant,
				assessmentErr: tc.assessmentErr,
			}
			guard := RequireRiskReadScoped(q, riskGuardEngine(t))
			w := httptest.NewRecorder()
			r := requestWithPathAndUser("/risks/"+assessmentID.String(), assessmentID.String(), viewerID.String(), "viewer")

			guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(w, r)

			if w.Code != tc.wantStatus {
				t.Fatalf("status: got %d want %d", w.Code, tc.wantStatus)
			}
		})
	}
}
