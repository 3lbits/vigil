package authz

import (
	"log/slog"
	"net/http"

	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/obs"
)

// ObjectLoader prepares request context with the loaded object data.
// It returns (updatedRequest, true) when the request can proceed.
// It should write the response and return false on not-found/internal failures.
type ObjectLoader func(http.ResponseWriter, *http.Request) (*http.Request, bool)

// ObjectInputBuilder computes dynamic OPA input for object-level decisions.
type ObjectInputBuilder func(*http.Request, middleware.SessionUser) (map[string]any, error)

// RequireObjectPolicy enforces OPA policy for object-level checks while letting
// modules keep ownership of resource loading details.
func RequireObjectPolicy(
	e *Engine,
	resource, action string,
	load ObjectLoader,
	buildInput ObjectInputBuilder,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := sessionUserFromContext(r)
			if !ok {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}

			req, proceed := loadObjectRequest(w, r, load)
			if !proceed {
				return
			}

			input, ok := objectInputForPolicy(w, req, user, resource, action, buildInput)
			if !ok {
				return
			}

			if !authorizeOrRespond(w, req, e, user, resource, action, input) {
				return
			}

			next.ServeHTTP(w, req)
		})
	}
}

// RequirePolicy returns middleware that evaluates the OPA policy for every
// request. resource and action are static per route (e.g. "frameworks", "write").
// Unauthenticated requests are redirected to /login.
func RequirePolicy(e *Engine, resource, action string) func(http.Handler) http.Handler {
	return RequireObjectPolicy(e, resource, action, nil, nil)
}

func sessionUserFromContext(r *http.Request) (middleware.SessionUser, bool) {
	return middleware.FromContext(r.Context())
}

func loadObjectRequest(w http.ResponseWriter, r *http.Request, load ObjectLoader) (*http.Request, bool) {
	if load == nil {
		return r, true
	}
	return load(w, r)
}

func objectInputForPolicy(
	w http.ResponseWriter,
	r *http.Request,
	user middleware.SessionUser,
	resource, action string,
	buildInput ObjectInputBuilder,
) (map[string]any, bool) {
	if buildInput == nil {
		return nil, true
	}
	input, err := buildInput(r, user)
	if err != nil {
		slog.Error("authz object input", "resource", resource, "action", action, "error", err)
		httputil.InternalServerError(w, r)
		return nil, false
	}
	return input, true
}

func authorizeOrRespond(
	w http.ResponseWriter,
	r *http.Request,
	e *Engine,
	user middleware.SessionUser,
	resource, action string,
	input map[string]any,
) bool {
	allowed, err := e.Allow(r.Context(), user.ID, user.Role, resource, action, input)
	if err != nil {
		slog.Error("authz eval", "resource", resource, "action", action, "error", err)
		httputil.InternalServerError(w, r)
		return false
	}
	if !allowed {
		obs.SecurityEvent(r.Context(), "authz.denied",
			"resource", resource,
			"action", action,
			"user_id", user.ID,
			"user_role", user.Role)
		httputil.Forbidden(w, r)
		return false
	}
	return true
}
