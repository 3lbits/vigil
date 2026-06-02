package risk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/middleware"
)

type assessmentCtxKey struct{}

var errMissingLoadedAssessment = errors.New("missing loaded assessment in context")

func assessmentFromContext(ctx context.Context) (db.RiskAssessment, bool) {
	a, ok := ctx.Value(assessmentCtxKey{}).(db.RiskAssessment)
	return a, ok
}

func withAssessment(ctx context.Context, a db.RiskAssessment) context.Context {
	return context.WithValue(ctx, assessmentCtxKey{}, a)
}

func loadAssessmentForGuard(w http.ResponseWriter, r *http.Request, q db.Querier) (*http.Request, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	assessment, err := q.GetRiskAssessment(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return nil, false
		}
		slog.Error("load risk assessment for authz guard", "error", err)
		httputil.InternalServerError(w, r)
		return nil, false
	}
	return r.WithContext(withAssessment(r.Context(), assessment)), true
}

func scopedReadInput(r *http.Request, q db.Querier, user middleware.SessionUser) (map[string]any, error) {
	assessment, ok := assessmentFromContext(r.Context())
	if !ok {
		return nil, errMissingLoadedAssessment
	}
	userID, err := uuid.Parse(user.ID)
	if err != nil {
		return nil, fmt.Errorf("parse user id for scoped risk read: %w", err)
	}
	isParticipant, err := q.IsRiskAssessmentParticipant(r.Context(), db.IsRiskAssessmentParticipantParams{
		AssessmentID: assessment.ID,
		UserID:       userID,
	})
	if err != nil {
		return nil, fmt.Errorf("query risk assessment participant: %w", err)
	}
	return map[string]any{
		"is_public":      assessment.IsPublic,
		"is_participant": isParticipant,
		"is_owner":       assessment.RiskOwnerID.Valid && assessment.RiskOwnerID.UUID == userID,
		"is_creator":     assessment.CreatedBy.Valid && assessment.CreatedBy.UUID == userID,
	}, nil
}

func updateOwnInput(r *http.Request, q db.Querier, user middleware.SessionUser) (map[string]any, error) {
	assessment, ok := assessmentFromContext(r.Context())
	if !ok {
		return nil, errMissingLoadedAssessment
	}
	userID, err := uuid.Parse(user.ID)
	if err != nil {
		return nil, fmt.Errorf("parse user id for scoped risk update: %w", err)
	}
	isParticipant, err := q.IsRiskAssessmentParticipant(r.Context(), db.IsRiskAssessmentParticipantParams{
		AssessmentID: assessment.ID,
		UserID:       userID,
	})
	if err != nil {
		return nil, fmt.Errorf("query risk assessment participant: %w", err)
	}
	return map[string]any{
		"is_participant": isParticipant,
	}, nil
}

func ownerInput(r *http.Request, user middleware.SessionUser) (map[string]any, error) {
	assessment, ok := assessmentFromContext(r.Context())
	if !ok {
		return nil, errMissingLoadedAssessment
	}
	userID, err := uuid.Parse(user.ID)
	if err != nil {
		return nil, fmt.Errorf("parse user id for risk owner check: %w", err)
	}
	return map[string]any{
		"is_owner": assessment.RiskOwnerID.Valid && assessment.RiskOwnerID.UUID == userID,
	}, nil
}

func RequireRiskReadScoped(q db.Querier, e *authz.Engine) func(http.Handler) http.Handler {
	return requireRiskObjectPolicy("read_scoped", q, e, scopedReadInput)
}

func RequireRiskUpdateOwn(q db.Querier, e *authz.Engine) func(http.Handler) http.Handler {
	return requireRiskObjectPolicy("update_own", q, e, updateOwnInput)
}

func requireRiskObjectPolicy(
	action string,
	q db.Querier,
	e *authz.Engine,
	build func(*http.Request, db.Querier, middleware.SessionUser) (map[string]any, error),
) func(http.Handler) http.Handler {
	load := func(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
		return loadAssessmentForGuard(w, r, q)
	}
	buildInput := func(r *http.Request, user middleware.SessionUser) (map[string]any, error) {
		return build(r, q, user)
	}
	return authz.RequireObjectPolicy(e, "risk", action, load, buildInput)
}

func RequireRiskOwnerDecision(action string, q db.Querier, e *authz.Engine) func(http.Handler) http.Handler {
	load := func(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
		return loadAssessmentForGuard(w, r, q)
	}
	return authz.RequireObjectPolicy(e, "risks", action, load, ownerInput)
}
