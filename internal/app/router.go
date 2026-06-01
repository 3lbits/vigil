package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/3lbits/vigil/internal/auth/scsstore"
	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/config"
	"github.com/3lbits/vigil/internal/health"
	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/locale"
	"github.com/3lbits/vigil/internal/modregistry"
	"github.com/3lbits/vigil/internal/modules/about"
	"github.com/3lbits/vigil/internal/modules/activities"
	"github.com/3lbits/vigil/internal/modules/admin"
	"github.com/3lbits/vigil/internal/modules/assets"
	authmodule "github.com/3lbits/vigil/internal/modules/auth"
	"github.com/3lbits/vigil/internal/modules/avvik"
	"github.com/3lbits/vigil/internal/modules/compliance"
	"github.com/3lbits/vigil/internal/modules/dashboard"
	"github.com/3lbits/vigil/internal/modules/me"
	"github.com/3lbits/vigil/internal/modules/measures"
	"github.com/3lbits/vigil/internal/modules/risk"
)

func buildMux(ctx context.Context, cfg *config.Config, state appState, opts Options) (*http.ServeMux, *scs.SessionManager, error) {
	mux := http.NewServeMux()
	staticHandler := http.FileServer(http.FS(opts.StaticFS))
	mux.Handle("GET /public/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		staticHandler.ServeHTTP(w, r)
	}))

	mux.HandleFunc("GET /healthz", health.Liveness)
	mux.HandleFunc("GET /readyz", health.Readiness(state.pool))
	registerMetricsRoute(mux, cfg, state)
	registerLocaleRoute(mux, cfg)

	if cfg.DevStubAuth {
		if err := seedDevStubUsers(ctx, state.queries); err != nil {
			return nil, nil, fmt.Errorf("seed dev users: %w", err)
		}
	}
	registerDevRoleRoute(mux, cfg.DevStubAuth, cfg.SessionCookieSecure, state.queries)

	sm := scs.New()
	sm.Store = scsstore.New(state.sqlDB)
	sm.HashTokenInStore = true
	sm.Lifetime = 24 * time.Hour
	sm.IdleTimeout = cfg.SessionIdleTimeout
	sm.Cookie.Name = cfg.SessionCookieName
	sm.Cookie.Secure = cfg.SessionCookieSecure
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.HttpOnly = true

	moduleDeps := modregistry.Dependencies{
		Queries: state.queries,
		Authz:   state.engine,
	}
	registry := modregistry.NewRegistry()
	coreModules := []modregistry.Module{
		about.New(),
		dashboard.New(),
		compliance.New(),
		measures.New(),
		me.New(),
		assets.New(),
		activities.New(),
		risk.New(),
		admin.New(state.pool, opts.StartTime, opts.Version),
	}
	if cfg.AvvikEnabled {
		coreModules = append(coreModules, avvik.New(state.sqlDB))
	}
	for _, m := range coreModules {
		if err := registry.Register(m); err != nil {
			return nil, nil, fmt.Errorf("register modules: %w", err)
		}
	}
	if !cfg.DevStubAuth {
		providers, err := buildProviders(ctx, cfg)
		if err != nil {
			return nil, nil, err
		}
		if err := registry.Register(authmodule.New(providers, sm, cfg.SessionHMACKey, cfg.SessionCookieSecure)); err != nil {
			return nil, nil, fmt.Errorf("register modules: %w", err)
		}
	}
	if err := registry.MountAll(mux, moduleDeps); err != nil {
		return nil, nil, fmt.Errorf("mount modules: %w", err)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httputil.NotFound(w, r)
	})
	return mux, sm, nil
}

func registerMetricsRoute(mux *http.ServeMux, cfg *config.Config, state appState) {
	if cfg.MetricsPath == "" {
		return
	}
	mux.Handle("GET "+cfg.MetricsPath,
		authz.RequirePolicy(state.engine, "admin", "read")(promhttp.HandlerFor(state.reg, promhttp.HandlerOpts{})))
	slog.Info("metrics enabled", "path", cfg.MetricsPath)
}

func registerLocaleRoute(mux *http.ServeMux, cfg *config.Config) {
	mux.HandleFunc("POST /locale", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 256)
		lang := r.FormValue("lang")
		if lang != locale.LangNB && lang != locale.LangNN && lang != locale.LangEN {
			lang = locale.DefaultLang
		}
		redirect := redirectPathFromReferer(r.Header.Get("Referer"))
		http.SetCookie(w, &http.Cookie{ //nolint:gosec // nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
			Name:     locale.CookieName,
			Value:    lang,
			Path:     "/",
			MaxAge:   365 * 24 * 3600,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   cfg.SessionCookieSecure,
		})
		http.Redirect(w, r, redirect, http.StatusSeeOther) //nolint:gosec // redirect is sanitised by redirectPathFromReferer via safeLocalRedirect
	})
}
