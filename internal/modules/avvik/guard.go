package avvik

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
)

type avvikCtxKey struct{}

func avvikFromContext(ctx context.Context) (db.Avvik, bool) {
	a, ok := ctx.Value(avvikCtxKey{}).(db.Avvik)
	return a, ok
}

func withAvvik(ctx context.Context, a db.Avvik) context.Context {
	return context.WithValue(ctx, avvikCtxKey{}, a)
}

func loadAvvikForGuard(w http.ResponseWriter, r *http.Request, q db.Querier) (*http.Request, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	a, err := q.GetAvvik(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	return r.WithContext(withAvvik(r.Context(), a)), true
}

var errMissingLoadedAvvik = errors.New("avvik not loaded in context")

// isReporterOrCreator returns true when user is the reporter (by email) or the
// user who created the avvik (from the event log). Admins always pass.
func isReporterOrCreator(ctx context.Context, q db.Querier, a db.Avvik, user middleware.SessionUser) bool {
	if user.Role == "admin" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(a.ReporterEmail), strings.TrimSpace(user.Email)) && strings.TrimSpace(user.Email) != "" {
		return true
	}
	uid, err := uuid.Parse(user.ID)
	if err != nil {
		return false
	}
	events, listErr := q.ListAvvikEvents(ctx, a.ID)
	if listErr != nil {
		return false
	}
	for _, e := range events {
		if e.EventType == "created" && e.ActorID.Valid && e.ActorID.UUID == uid {
			return true
		}
	}
	return false
}

func submitOwnInput(r *http.Request, q db.Querier, user middleware.SessionUser) (map[string]any, error) {
	a, ok := avvikFromContext(r.Context())
	if !ok {
		return nil, errMissingLoadedAvvik
	}
	return map[string]any{
		"is_participant": isReporterOrCreator(r.Context(), q, a, user),
	}, nil
}

// RequireAvvikSubmitOwn enforces the avvik/submit_own policy: the user must be
// the reporter, the creator, or an admin.
func RequireAvvikSubmitOwn(q db.Querier, e *authz.Engine) func(http.Handler) http.Handler {
	load := func(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
		return loadAvvikForGuard(w, r, q)
	}
	buildInput := func(r *http.Request, user middleware.SessionUser) (map[string]any, error) {
		return submitOwnInput(r, q, user)
	}
	return authz.RequireObjectPolicy(e, "avvik", "submit_own", load, buildInput)
}
