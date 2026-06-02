package app

import (
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

func withMiddleware(cfg *config.Config, state appState, sm *scs.SessionManager, mux *http.ServeMux) http.Handler {
	instMux := obs.NewInstrumentedMux(mux)
	sourceIP := obs.SourceIPMiddlewareWithTrustedCIDRs(state.trustedCIDRs)
	globalRateLimit := globalRateLimitMiddleware(cfg)

	var handler http.Handler
	if cfg.DevStubAuth {
		handler = middleware.Chain(instMux,
			middleware.StubDBMiddleware(state.queries),
			locale.LangMiddleware(state.bundle),
			csrf.Middleware(state.csrfKey, cfg.SessionCookieSecure, nil),
			middleware.ModuleFlagsMiddleware(state.queries, cfg.AvvikEnabled),
			sourceIP,
			globalRateLimit,
			obs.RequestMiddleware("/healthz", "/readyz", cfg.MetricsPath),
			obs.HTTPMetricsMiddleware(state.metrics),
			obs.PanicMiddleware,
		)
	} else {
		publicRoutes, publicPrefixes := loginExemptRoutes()
		handler = middleware.Chain(instMux,
			sm.LoadAndSave,
			auth.UserMiddleware(sm, state.queries),
			middleware.RequireLoginExcept(publicRoutes, publicPrefixes),
			locale.LangMiddleware(state.bundle),
			csrf.Middleware(state.csrfKey, cfg.SessionCookieSecure, func(r *http.Request) string {
				return sm.Token(r.Context())
			}),
			middleware.ModuleFlagsMiddleware(state.queries, cfg.AvvikEnabled),
			sourceIP,
			globalRateLimit,
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
