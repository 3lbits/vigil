package activities

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

type activityCtxKey struct{}

var errMissingLoadedActivity = errors.New("missing loaded activity in context")

func activityFromContext(ctx context.Context) (db.GetActivityRow, bool) {
	a, ok := ctx.Value(activityCtxKey{}).(db.GetActivityRow)
	return a, ok
}

func withActivity(ctx context.Context, a db.GetActivityRow) context.Context {
	return context.WithValue(ctx, activityCtxKey{}, a)
}

func loadActivityForGuard(w http.ResponseWriter, r *http.Request, q db.Querier) (uuid.UUID, db.GetActivityRow, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return uuid.Nil, db.GetActivityRow{}, false
	}
	a, err := q.GetActivity(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return uuid.Nil, db.GetActivityRow{}, false
		}
		slog.Error("load activity for authz guard", "error", err)
		httputil.InternalServerError(w, r)
		return uuid.Nil, db.GetActivityRow{}, false
	}
	return id, a, true
}

func participantForActivity(ctx context.Context, q db.Querier, userID string, activityID uuid.UUID) (bool, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return false, fmt.Errorf("parse user id: %w", err)
	}
	isParticipant, err := q.IsParticipant(ctx, db.IsParticipantParams{
		ResourceType: "activities",
		ResourceID:   activityID,
		UserID:       uid,
	})
	if err != nil {
		return false, fmt.Errorf("is participant: %w", err)
	}
	return isParticipant, nil
}

// RequireActivityUpdateOwn loads the activity and enforces update_own policy
// with participant context before handing control to the route handler.
func RequireActivityUpdateOwn(q db.Querier, e *authz.Engine) func(http.Handler) http.Handler {
	load := func(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
		_, a, ok := loadActivityForGuard(w, r, q)
		if !ok {
			return nil, false
		}
		return r.WithContext(withActivity(r.Context(), a)), true
	}
	buildInput := func(r *http.Request, user middleware.SessionUser) (map[string]any, error) {
		a, ok := activityFromContext(r.Context())
		if !ok {
			return nil, errMissingLoadedActivity
		}
		isParticipant, err := participantForActivity(r.Context(), q, user.ID, a.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"is_participant": isParticipant}, nil
	}
	return authz.RequireObjectPolicy(e, "activities", "update_own", load, buildInput)
}
