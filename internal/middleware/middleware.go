// Package middleware provides shared HTTP middleware and context helpers.
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/health"
	"github.com/3lbits/vigil/internal/httpresp"
)

// SessionUser holds the authenticated user in the request context.
type SessionUser struct {
	ID    string
	Name  string
	Role  string
	Email string
}

type ctxKey struct{}
type devStubAuthKey struct{}

// FromContext retrieves the SessionUser from the request context.
func FromContext(ctx context.Context) (SessionUser, bool) {
	u, ok := ctx.Value(ctxKey{}).(SessionUser)
	return u, ok
}

// SetUser returns a context with the SessionUser stored.
func SetUser(ctx context.Context, u SessionUser) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// StubMiddleware injects a dev admin user on every request. Never use in production.
func StubMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := "admin"
		if c, err := r.Cookie(DevRoleCookieName); err == nil && IsAllowedDevRole(c.Value) {
			role = c.Value
		}
		ctx := SetUser(r.Context(), SessionUser{ID: "00000000-0000-0000-0000-000000000000", Name: "Dev User", Role: role, Email: "dev@localhost"})
		ctx = context.WithValue(ctx, devStubAuthKey{}, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// DevRoleCookieName stores selected role for development stub auth mode.
const DevRoleCookieName = "vigil_dev_role"

// DevUserCookieName stores selected seeded development user ID.
const DevUserCookieName = "vigil_dev_user"

// IsAllowedDevRole reports whether role can be selected in dev stub auth.
func IsAllowedDevRole(role string) bool {
	switch role {
	case "admin", "editor", "viewer", "contributor":
		return true
	default:
		return false
	}
}

// StubDBMiddleware injects a seeded development user from DB on every request.
// Intended only for DevStubAuth mode.
func StubDBMiddleware(q db.Querier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			users, err := q.ListDevStubUsers(r.Context())
			if err != nil {
				slog.Error("list dev stub users", "error", err)
				http.Error(w, "failed to load development users", http.StatusInternalServerError)
				return
			}
			if len(users) == 0 {
				http.Error(w, "no development users seeded", http.StatusInternalServerError)
				return
			}

			cookie, cookieErr := r.Cookie(DevUserCookieName)
			selected := pickSeededDevUser(users, cookie, cookieErr)

			ctx := SetUser(r.Context(), SessionUser{
				ID:    selected.ID.String(),
				Name:  selected.Name,
				Role:  selected.Role,
				Email: selected.Email,
			})
			ctx = context.WithValue(ctx, devStubAuthKey{}, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func pickSeededDevUser(users []db.User, cookie *http.Cookie, cookieErr error) db.User {
	selected := users[0]
	if cookieErr != nil {
		return selected
	}
	id, err := uuid.Parse(cookie.Value)
	if err != nil {
		return selected
	}
	for _, u := range users {
		if u.ID == id {
			return u
		}
	}
	return selected
}

// DevStubAuthFromContext reports whether request was authenticated by stub auth.
func DevStubAuthFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(devStubAuthKey{}).(bool)
	return v
}

// RequireAuth redirects to / if no session user is present in context.
// Must be placed after an auth or stub middleware that sets the user.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := FromContext(r.Context()); !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type PublicRoute struct {
	Method string
	Path   string
}

// RequireLoginExcept redirects anonymous requests to /login unless they match
// one of the allowed exact routes or path prefixes.
func RequireLoginExcept(exact []PublicRoute, prefixes []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := FromContext(r.Context()); ok || IsPublicRoute(r, exact, prefixes) {
				next.ServeHTTP(w, r)
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
		})
	}
}

// IsPublicRoute reports whether request matches the allowed exact routes or path
// prefixes (GET exact routes also allow HEAD).
func IsPublicRoute(r *http.Request, exact []PublicRoute, prefixes []string) bool {
	return isPublicRoute(r, exact, prefixes)
}

func isPublicRoute(r *http.Request, exact []PublicRoute, prefixes []string) bool {
	path := r.URL.Path
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	for _, route := range exact {
		if path == route.Path && methodMatches(r.Method, route.Method) {
			return true
		}
	}
	return false
}

func methodMatches(got, want string) bool {
	if got == want {
		return true
	}
	// Go's GET handlers also serve HEAD.
	return got == http.MethodHead && want == http.MethodGet
}

// Chain applies multiple middleware functions to a handler in order, so that
// the first listed is the outermost wrapper (runs first on request).
func Chain(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

// ModuleFlags holds the per-request feature-toggle state for optional modules.
type ModuleFlags struct {
	ComplianceEnabled bool
	RiskEnabled       bool
	ActivitiesEnabled bool
	AssetsEnabled     bool
	AvvikEnabled      bool
}

type moduleFlagsKey struct{}
type topOrgNameKey struct{}

const defaultTopOrgName = "Vigil"
const moduleFlagsColdCacheFailClosed = true

// ModuleFlagsSnapshot is the cached view served to request handlers.
type ModuleFlagsSnapshot struct {
	Flags      ModuleFlags
	TopOrgName string
}

// ModuleFlagsCache maintains an atomic snapshot of module flags and top org
// name. Refresh is expected to run in background and on explicit invalidation
// paths; request middleware should only perform atomic loads.
type ModuleFlagsCache struct {
	q            db.Querier
	avvikEnabled bool
	lastKnown    atomic.Pointer[ModuleFlagsSnapshot]
}

func NewModuleFlagsCache(q db.Querier, avvikEnabled bool) *ModuleFlagsCache {
	return &ModuleFlagsCache{
		q:            q,
		avvikEnabled: avvikEnabled,
	}
}

func (c *ModuleFlagsCache) Refresh(ctx context.Context) error {
	settings, err := c.q.GetAppSettings(ctx)
	if err != nil {
		health.SetModuleFlagsSettingsLoadFailure(true)
		return fmt.Errorf("load app settings: %w", err)
	}

	flags := moduleFlagsFromSettings(settings, c.avvikEnabled)
	topOrgName := defaultTopOrgName
	orgs, orgErr := c.q.ListOrganizations(ctx)
	if orgErr != nil {
		slog.Warn("list organizations", "error", orgErr)
	} else {
		topOrgName = resolveTopOrgName(orgs)
	}

	snapshot := ModuleFlagsSnapshot{
		Flags:      flags,
		TopOrgName: topOrgName,
	}
	c.lastKnown.Store(&snapshot)
	health.SetModuleFlagsSettingsLoadFailure(false)
	return nil
}

func (c *ModuleFlagsCache) Snapshot() (ModuleFlagsSnapshot, bool) {
	if cached := c.lastKnown.Load(); cached != nil {
		return *cached, true
	}
	return ModuleFlagsSnapshot{}, false
}

// ModuleFlagsFromContext retrieves ModuleFlags from the request context.
// Returns all-enabled defaults if not set (safe for tests without the middleware).
func ModuleFlagsFromContext(ctx context.Context) ModuleFlags {
	f, ok := ctx.Value(moduleFlagsKey{}).(ModuleFlags)
	if !ok {
		return ModuleFlags{
			ComplianceEnabled: true,
			RiskEnabled:       true,
			ActivitiesEnabled: true,
			AssetsEnabled:     true,
			AvvikEnabled:      true,
		}
	}
	return f
}

// TopOrgNameFromContext returns top-level organisation name for UI copy.
// Falls back to a sensible default when no top org is configured.
func TopOrgNameFromContext(ctx context.Context) string {
	name, ok := ctx.Value(topOrgNameKey{}).(string)
	if !ok || strings.TrimSpace(name) == "" {
		return defaultTopOrgName
	}
	return strings.TrimSpace(name)
}

// ModuleFlagsMiddleware injects the current module flag snapshot into request
// context without hitting the database. Refresh is performed separately by
// startup/background loops and explicit invalidation paths.
func ModuleFlagsMiddleware(cache *ModuleFlagsCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			flags := ModuleFlags{
				ComplianceEnabled: true,
				RiskEnabled:       true,
				ActivitiesEnabled: true,
				AssetsEnabled:     true,
				AvvikEnabled:      true,
			}
			topOrgName := defaultTopOrgName
			if cache != nil {
				flags = moduleFlagsColdCacheDefault(cache.avvikEnabled)
				if snapshot, ok := cache.Snapshot(); ok {
					flags = snapshot.Flags
					topOrgName = snapshot.TopOrgName
				}
			}

			ctx := context.WithValue(r.Context(), moduleFlagsKey{}, flags)
			ctx = context.WithValue(ctx, topOrgNameKey{}, topOrgName)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func moduleFlagsFromSettings(settings db.AppSetting, avvikEnabled bool) ModuleFlags {
	return ModuleFlags{
		ComplianceEnabled: settings.ComplianceEnabled,
		RiskEnabled:       settings.RiskEnabled,
		ActivitiesEnabled: settings.ActivitiesEnabled,
		AssetsEnabled:     settings.AssetsEnabled,
		AvvikEnabled:      avvikEnabled,
	}
}

func moduleFlagsColdCacheDefault(avvikEnabled bool) ModuleFlags {
	// Fail closed when app_settings cannot be read before any successful load.
	// This preserves operator intent ("disabled stays disabled") in audit-focused
	// environments. Availability still degrades gracefully once a last-known-good
	// value has been observed.
	if moduleFlagsColdCacheFailClosed {
		return ModuleFlags{
			ComplianceEnabled: false,
			RiskEnabled:       false,
			ActivitiesEnabled: false,
			AssetsEnabled:     false,
			AvvikEnabled:      false,
		}
	}
	return ModuleFlags{
		ComplianceEnabled: true,
		RiskEnabled:       true,
		ActivitiesEnabled: true,
		AssetsEnabled:     true,
		AvvikEnabled:      avvikEnabled,
	}
}

func resolveTopOrgName(orgs []db.Organization) string {
	for _, org := range orgs {
		if !org.ParentID.Valid && strings.TrimSpace(org.Name) != "" {
			return strings.TrimSpace(org.Name)
		}
	}
	for _, org := range orgs {
		if strings.TrimSpace(org.Name) != "" {
			return strings.TrimSpace(org.Name)
		}
	}
	return defaultTopOrgName
}

// RequireModule returns a middleware that responds with 404 if the named module
// is disabled. name must be "compliance", "risk", "activities", "assets", or "avvik".
func RequireModule(name string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			flags := ModuleFlagsFromContext(r.Context())
			var enabled bool
			switch name {
			case "compliance":
				enabled = flags.ComplianceEnabled
			case "risk":
				enabled = flags.RiskEnabled
			case "activities":
				enabled = flags.ActivitiesEnabled
			case "assets":
				enabled = flags.AssetsEnabled
			case "avvik":
				enabled = flags.AvvikEnabled
			default:
				enabled = true
			}
			if !enabled {
				httpresp.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
