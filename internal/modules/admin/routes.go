package admin

import (
	"net/http"
	"time"

	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/modregistry"
	"github.com/3lbits/vigil/internal/obs"
)

type adminModule struct {
	pool      dbPinger
	startTime time.Time
	version   string
}

func New(pool dbPinger, startTime time.Time, version string) modregistry.Module {
	return adminModule{
		pool:      pool,
		startTime: startTime,
		version:   version,
	}
}

func (adminModule) Name() string {
	return "admin"
}

func (m adminModule) Register(deps modregistry.Dependencies, r *modregistry.Registrar) error {
	h := NewHandler(deps.Queries, m.pool, m.startTime, m.version)
	adminMutationLimiter := middleware.NewIPRateLimiterWithKey(20, time.Minute, func(r *http.Request) string {
		return obs.SourceIPFromContext(r.Context())
	})
	managePolicy := modregistry.Policy{Resource: "users", Action: "manage"}
	rateLimit := func(next http.Handler) http.Handler {
		return adminMutationLimiter.Wrap(next.ServeHTTP)
	}

	r.Guarded("GET /admin", managePolicy, h.AdminPage)
	r.Guarded("POST /admin/users/{id}/role", managePolicy, h.SetRole, rateLimit)
	r.Guarded("POST /admin/sessions/{id}/revoke", managePolicy, h.RevokeUserSessions, rateLimit)
	r.Guarded("POST /admin/users", managePolicy, h.PreCreateUser, rateLimit)
	r.Guarded("POST /admin/users/{id}/delete", managePolicy, h.DeleteUser, rateLimit)
	r.Guarded("GET /admin/orgs", managePolicy, h.OrgsPage)
	r.Guarded("POST /admin/orgs", managePolicy, h.CreateOrg, rateLimit)
	r.Guarded("POST /admin/orgs/{id}/delete", managePolicy, h.DeleteOrg, rateLimit)
	r.Guarded("GET /admin/risk-settings", managePolicy, h.RiskSettingsPage)
	r.Guarded("POST /admin/risk-settings", managePolicy, h.SaveRiskSettings, rateLimit)
	r.Guarded("POST /admin/users/{id}/org", managePolicy, h.SetUserOrg, rateLimit)
	r.Guarded("POST /admin/module-settings", managePolicy, h.SaveModuleSettings, rateLimit)
	return nil
}
