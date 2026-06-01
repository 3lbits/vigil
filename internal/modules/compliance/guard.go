package compliance

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

var errUnsupportedResource = errors.New("unsupported resource")
var errMissingLoadedFramework = errors.New("missing loaded framework in context")
var errMissingLoadedRequirement = errors.New("missing loaded requirement in context")

type frameworkCtxKey struct{}
type requirementCtxKey struct{}

func frameworkFromContext(ctx context.Context) (db.Framework, bool) {
	fw, ok := ctx.Value(frameworkCtxKey{}).(db.Framework)
	return fw, ok
}

func requirementFromContext(ctx context.Context) (db.Requirement, bool) {
	req, ok := ctx.Value(requirementCtxKey{}).(db.Requirement)
	return req, ok
}

func withFramework(ctx context.Context, fw db.Framework) context.Context {
	return context.WithValue(ctx, frameworkCtxKey{}, fw)
}

func withRequirement(ctx context.Context, req db.Requirement) context.Context {
	return context.WithValue(ctx, requirementCtxKey{}, req)
}

func participantForResource(ctx context.Context, q db.Querier, userID string, resourceType string, resourceID uuid.UUID) (bool, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return false, fmt.Errorf("parse user id: %w", err)
	}
	ok, err := q.IsParticipant(ctx, db.IsParticipantParams{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		UserID:       uid,
	})
	if err != nil {
		return false, fmt.Errorf("is participant: %w", err)
	}
	return ok, nil
}

type loadedComplianceResource struct {
	framework   *db.Framework
	requirement *db.Requirement
}

func loadComplianceResource(ctx context.Context, q db.Querier, resource string, id uuid.UUID) (loadedComplianceResource, error) {
	switch resource {
	case "frameworks":
		fw, err := q.GetFramework(ctx, id)
		if err != nil {
			return loadedComplianceResource{}, fmt.Errorf("get framework: %w", err)
		}
		return loadedComplianceResource{framework: &fw}, nil
	case "requirements":
		req, err := q.GetRequirement(ctx, id)
		if err != nil {
			return loadedComplianceResource{}, fmt.Errorf("get requirement: %w", err)
		}
		return loadedComplianceResource{requirement: &req}, nil
	default:
		return loadedComplianceResource{}, fmt.Errorf("%w: %s", errUnsupportedResource, resource)
	}
}

func pathIDOrNotFound(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return uuid.Nil, false
	}
	return id, true
}

func loadComplianceResourceOrRespond(
	w http.ResponseWriter,
	r *http.Request,
	q db.Querier,
	resource string,
	id uuid.UUID,
) (loadedComplianceResource, bool) {
	loaded, err := loadComplianceResource(r.Context(), q, resource, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return loadedComplianceResource{}, false
		}
		slog.Error("load compliance resource for authz guard", "resource", resource, "error", err)
		httputil.InternalServerError(w, r)
		return loadedComplianceResource{}, false
	}
	return loaded, true
}

func requestWithLoadedContextOrRespond(w http.ResponseWriter, r *http.Request, resource string, loaded loadedComplianceResource) (*http.Request, bool) {
	ctx := r.Context()
	switch resource {
	case "frameworks":
		if loaded.framework == nil {
			httputil.InternalServerError(w, r)
			return nil, false
		}
		ctx = withFramework(ctx, *loaded.framework)
	case "requirements":
		if loaded.requirement == nil {
			httputil.InternalServerError(w, r)
			return nil, false
		}
		ctx = withRequirement(ctx, *loaded.requirement)
	default:
		httputil.InternalServerError(w, r)
		return nil, false
	}
	return r.WithContext(ctx), true
}

func RequireComplianceUpdateOwn(resource string, q db.Querier, e *authz.Engine) func(http.Handler) http.Handler {
	load := func(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
		id, ok := pathIDOrNotFound(w, r)
		if !ok {
			return nil, false
		}
		loaded, ok := loadComplianceResourceOrRespond(w, r, q, resource, id)
		if !ok {
			return nil, false
		}
		rWithLoaded, ok := requestWithLoadedContextOrRespond(w, r, resource, loaded)
		if !ok {
			return nil, false
		}
		return rWithLoaded, true
	}
	buildInput := func(r *http.Request, user middleware.SessionUser) (map[string]any, error) {
		id, err := resourceIDFromContext(resource, r.Context())
		if err != nil {
			return nil, err
		}
		isParticipant, err := participantForResource(r.Context(), q, user.ID, resource, id)
		if err != nil {
			return nil, err
		}
		return map[string]any{"is_participant": isParticipant}, nil
	}
	return authz.RequireObjectPolicy(e, resource, "update_own", load, buildInput)
}

func resourceIDFromContext(resource string, ctx context.Context) (uuid.UUID, error) {
	switch resource {
	case "frameworks":
		fw, ok := frameworkFromContext(ctx)
		if !ok {
			return uuid.Nil, errMissingLoadedFramework
		}
		return fw.ID, nil
	case "requirements":
		req, ok := requirementFromContext(ctx)
		if !ok {
			return uuid.Nil, errMissingLoadedRequirement
		}
		return req.ID, nil
	default:
		return uuid.Nil, fmt.Errorf("%w: %s", errUnsupportedResource, resource)
	}
}
