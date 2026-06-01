package auth

import (
	"net/http"
	"time"

	"github.com/3lbits/vigil/internal/auth"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/modregistry"
	"github.com/3lbits/vigil/internal/obs"
	"github.com/alexedwards/scs/v2"
)

type authModule struct {
	providers      []auth.Provider
	sm             *scs.SessionManager
	sessionHMACKey string
	cookieSecure   bool
}

func New(providers []auth.Provider, sm *scs.SessionManager, sessionHMACKey string, cookieSecure bool) modregistry.Module {
	return authModule{
		providers:      providers,
		sm:             sm,
		sessionHMACKey: sessionHMACKey,
		cookieSecure:   cookieSecure,
	}
}

func (authModule) Name() string {
	return "auth"
}

func (m authModule) Register(deps modregistry.Dependencies, r *modregistry.Registrar) error {
	var q db.Querier = deps.Queries
	h := NewHandler(q, m.providers, m.sm, m.sessionHMACKey, m.cookieSecure)
	rl := middleware.NewIPRateLimiterWithKey(20, time.Minute, func(r *http.Request) string {
		return obs.SourceIPFromContext(r.Context())
	})
	rateLimit := func(next http.Handler) http.Handler {
		return rl.Wrap(next.ServeHTTP)
	}

	r.Public("GET /login", h.LoginPage)
	r.Public("GET /auth/{slug}", h.Redirect, rateLimit)
	r.Public("GET /auth/{slug}/callback", h.Callback, rateLimit)
	r.Public("POST /logout", h.Logout, rateLimit)
	return nil
}
