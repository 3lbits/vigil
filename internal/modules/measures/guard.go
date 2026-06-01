package measures

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/google/uuid"
)

type measureCtxKey struct{}

var errMissingLoadedMeasure = errors.New("missing loaded measure in context")

func measureFromContext(ctx context.Context) (db.Measure, bool) {
	m, ok := ctx.Value(measureCtxKey{}).(db.Measure)
	return m, ok
}

func withMeasure(ctx context.Context, m db.Measure) context.Context {
	return context.WithValue(ctx, measureCtxKey{}, m)
}

func loadMeasureForGuard(w http.ResponseWriter, r *http.Request, q db.Querier) (uuid.UUID, db.Measure, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return uuid.Nil, db.Measure{}, false
	}
	m, err := q.GetMeasure(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return uuid.Nil, db.Measure{}, false
		}
		slog.Error("load measure for authz guard", "error", err)
		httputil.InternalServerError(w, r)
		return uuid.Nil, db.Measure{}, false
	}
	return id, m, true
}

func participantForMeasure(ctx context.Context, q db.Querier, userID string, measureID uuid.UUID) (bool, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return false, fmt.Errorf("parse user id: %w", err)
	}
	isParticipant, err := q.IsParticipant(ctx, db.IsParticipantParams{
		ResourceType: "measures",
		ResourceID:   measureID,
		UserID:       uid,
	})
	if err != nil {
		return false, fmt.Errorf("is participant: %w", err)
	}
	return isParticipant, nil
}

// RequireMeasureUpdateOwn loads the measure and enforces update_own policy
// with participant context before handing control to the route handler.
func RequireMeasureUpdateOwn(q db.Querier, e *authz.Engine) func(http.Handler) http.Handler {
	load := func(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
		_, m, ok := loadMeasureForGuard(w, r, q)
		if !ok {
			return nil, false
		}
		return r.WithContext(withMeasure(r.Context(), m)), true
	}
	buildInput := func(r *http.Request, user middleware.SessionUser) (map[string]any, error) {
		m, ok := measureFromContext(r.Context())
		if !ok {
			return nil, errMissingLoadedMeasure
		}
		isParticipant, err := participantForMeasure(r.Context(), q, user.ID, m.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"is_participant": isParticipant}, nil
	}
	return authz.RequireObjectPolicy(e, "measures", "update_own", load, buildInput)
}
