package app

import (
	"context"
	"net/http"
	"strings"

	"github.com/alexedwards/scs/v2"

	"github.com/3lbits/vigil/internal/auth"
	"github.com/3lbits/vigil/internal/config"
	"github.com/3lbits/vigil/internal/csrf"
	"github.com/3lbits/vigil/internal/locale"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/obs"
)

func loginExemptRoutes() ([]middleware.PublicRoute, []string) {
	publicRoutes := []middleware.PublicRoute{
		{Method: http.MethodGet, Path: "/healthz"},
		{Method: http.MethodGet, Path: "/readyz"},
		{Method: http.MethodPost, Path: "/locale"},
		{Method: http.MethodGet, Path: "/login"},
	}
	publicPrefixes := []string{"/public/", "/auth/"}
	return publicRoutes, publicPrefixes
}

// loginExemptRoutesNais returns the public routes for NaisAuth mode. The /oauth2/
// prefix is owned by the Wonderwall proxy and never processed by the app, but
// we exempt it here defensively. /healthz and /readyz must be reachable by the
// NAIS load balancer without a bearer token.
func loginExemptRoutesNais() ([]middleware.PublicRoute, []string) {
	publicRoutes := []middleware.PublicRoute{
		{Method: http.MethodGet, Path: "/healthz"},
		{Method: http.MethodGet, Path: "/readyz"},
		{Method: http.MethodPost, Path: "/locale"},
		{Method: http.MethodGet, Path: "/login"},
	}
	publicPrefixes := []string{"/public/", "/oauth2/"}
	return publicRoutes, publicPrefixes
}

func globalRateLimitExemptRoutes(cfg *config.Config) ([]middleware.PublicRoute, []string) {
	exact := []middleware.PublicRoute{
		{Method: http.MethodGet, Path: "/healthz"},
		{Method: http.MethodGet, Path: "/readyz"},
	}
	if cfg.MetricsPath != "" {
		exact = append(exact, middleware.PublicRoute{Method: http.MethodGet, Path: cfg.MetricsPath})
	}
	prefixes := []string{"/public/"}
	return exact, prefixes
}

func globalRateLimitMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	if cfg.GlobalRateLimitPerWindow == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	limiter := middleware.NewIPRateLimiterWithKey(cfg.GlobalRateLimitPerWindow, cfg.GlobalRateLimitWindow, func(r *http.Request) string {
		if ip := strings.TrimSpace(obs.SourceIPFromContext(r.Context())); ip != "" {
			return ip
		}
		return ""
	})
	exact, prefixes := globalRateLimitExemptRoutes(cfg)
	return func(next http.Handler) http.Handler {
		limited := limiter.Wrap(next.ServeHTTP)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if middleware.IsPublicRoute(r, exact, prefixes) {
				next.ServeHTTP(w, r)
				return
			}
			limited(w, r)
		})
	}
}

func withMiddleware(ctx context.Context, cfg *config.Config, state appState, sm *scs.SessionManager, mux *http.ServeMux) http.Handler {
	instMux := obs.NewInstrumentedMux(mux)
	sourceIP := obs.SourceIPMiddlewareWithTrustedCIDRs(state.trustedCIDRs)
	globalRateLimit := globalRateLimitMiddleware(cfg)
	moduleFlags := middleware.ModuleFlagsMiddleware(state.moduleFlags)

	var handler http.Handler
	switch {
	case cfg.DevStubAuth:
		handler = middleware.Chain(instMux,
			sourceIP,
			globalRateLimit,
			middleware.StubDBMiddleware(state.queries),
			locale.LangMiddleware(state.bundle),
			csrf.Middleware(state.csrfKey, cfg.SessionCookieSecure, nil),
			moduleFlags,
			obs.RequestMiddleware("/healthz", "/readyz", cfg.MetricsPath),
			obs.HTTPMetricsMiddleware(state.metrics),
			obs.PanicMiddleware,
		)
	case cfg.NaisAuth:
		verifier := auth.NewNaviktVerifier(ctx, cfg.NaisIssuer, cfg.NaisClientID, cfg.NaisJWKSURI)
		naisAuth := auth.NaviktMiddleware(
			verifier,
			sm,
			state.queries,
			cfg.AdminGroups,
			cfg.AllowedEmailDomains,
			cfg.SessionHMACKey,
		)
		publicRoutes, publicPrefixes := loginExemptRoutesNais()
		handler = middleware.Chain(instMux,
			sourceIP,
			globalRateLimit,
			sm.LoadAndSave,
			naisAuth,
			middleware.RequireLoginExcept(publicRoutes, publicPrefixes),
			locale.LangMiddleware(state.bundle),
			// Bind the CSRF token to the authenticated user's stable DB ID so
			// that the token survives token refreshes (the bearer token changes
			// on every Wonderwall session renewal, but the user ID is stable).
			csrf.Middleware(state.csrfKey, cfg.SessionCookieSecure, func(r *http.Request) string {
				if u, ok := middleware.FromContext(r.Context()); ok {
					return u.ID
				}
				return ""
			}),
			moduleFlags,
			obs.RequestMiddleware("/healthz", "/readyz", cfg.MetricsPath),
			obs.HTTPMetricsMiddleware(state.metrics),
			obs.PanicMiddleware,
		)
	default:
		publicRoutes, publicPrefixes := loginExemptRoutes()
		handler = middleware.Chain(instMux,
			sourceIP,
			globalRateLimit,
			sm.LoadAndSave,
			auth.UserMiddleware(sm, state.queries),
			middleware.RequireLoginExcept(publicRoutes, publicPrefixes),
			locale.LangMiddleware(state.bundle),
			csrf.Middleware(state.csrfKey, cfg.SessionCookieSecure, func(r *http.Request) string {
				return sm.Token(r.Context())
			}),
			moduleFlags,
			obs.RequestMiddleware("/healthz", "/readyz", cfg.MetricsPath),
			obs.HTTPMetricsMiddleware(state.metrics),
			obs.PanicMiddleware,
		)
	}

	hstsEnabled := strings.EqualFold(cfg.AppEnv, "production")
	handler = middleware.SecurityHeaders(hstsEnabled)(handler)
	handler = obs.TraceMiddleware(handler)
	return handler
}
