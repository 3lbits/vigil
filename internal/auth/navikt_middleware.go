package auth

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/alexedwards/scs/v2"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/obs"
)

const (
	naisCacheSubjectKey = "nais_auth_subject"
	naisCacheUserIDKey  = "nais_auth_user_id"
	naisCacheNameKey    = "nais_auth_name"
	naisCacheEmailKey   = "nais_auth_email"
	naisCacheDBRoleKey  = "nais_auth_db_role"
)

// NaviktMiddleware verifies the Bearer token injected by the Wonderwall proxy,
// populates the SessionUser context, and only touches the DB on cache misses.
// Requests without a valid token (or with a token that fails verification)
// continue unauthenticated — the RequireLoginExcept middleware enforces the gate.
//
// If the token's groups[] contains any entry in adminGroups, the role is
// promoted to "admin" for the duration of this request. Otherwise the stored
// DB role is used.
func NaviktMiddleware(
	v *NaviktVerifier,
	sm *scs.SessionManager,
	q db.Querier,
	adminGroups []string,
	allowedDomains []string,
	sessionHMACKey string,
) func(http.Handler) http.Handler {
	authCfg := naviktAuthConfig{
		verifier:       v,
		sm:             sm,
		queries:        q,
		adminGroups:    adminGroups,
		allowedDomains: allowedDomains,
		sessionHMACKey: sessionHMACKey,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessionUser, ok, stop := authenticateNaviktRequest(r, w, authCfg)
			if stop {
				return
			}
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			ctx := middleware.SetUser(r.Context(), sessionUser)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type naviktAuthConfig struct {
	verifier       *NaviktVerifier
	sm             *scs.SessionManager
	queries        db.Querier
	adminGroups    []string
	allowedDomains []string
	sessionHMACKey string
}

func authenticateNaviktRequest(
	r *http.Request,
	w http.ResponseWriter,
	cfg naviktAuthConfig,
) (middleware.SessionUser, bool, bool) {
	rawToken := extractBearerToken(r)
	if rawToken == "" {
		return middleware.SessionUser{}, false, false
	}

	claims, err := cfg.verifier.Verify(r.Context(), rawToken)
	if err != nil {
		obs.SecurityEvent(r.Context(), "auth.login_failed",
			"provider", "navikt",
			"reason", "token_verify_failed")
		slog.Warn("navikt: bearer token verification failed", "error", err)
		return middleware.SessionUser{}, false, false
	}
	if !isAllowedEmailDomain(claims.PreferredUsername, cfg.allowedDomains) {
		obs.SecurityEvent(r.Context(), "auth.login_denied",
			"provider", "navikt",
			"reason", "email_domain_not_allowed")
		http.Error(w, "email domain not allowed", http.StatusForbidden)
		return middleware.SessionUser{}, false, true
	}

	providerID := naviktProviderID(claims)
	if providerID == "" {
		obs.SecurityEvent(r.Context(), "auth.login_failed",
			"provider", "navikt",
			"reason", "missing_subject")
		return middleware.SessionUser{}, false, false
	}
	if cfg.sm.GetString(r.Context(), naisCacheSubjectKey) == providerID {
		if cachedUser, ok := cachedSessionUser(r, cfg.sm, claims, cfg.adminGroups); ok {
			return cachedUser, true, false
		}
	}

	user, err := upsertNaviktUser(r, cfg.queries, providerID, claims)
	if err != nil {
		obs.SecurityEvent(r.Context(), "auth.login_failed",
			"provider", "navikt",
			"reason", "upsert_failed")
		slog.Error("navikt: upsert user", "error", err)
		return middleware.SessionUser{}, false, false
	}

	cacheNaviktUser(r, cfg.sm, providerID, user)
	role := userRoleForRequest(user.Role, claims, cfg.adminGroups)
	obs.SecurityEvent(r.Context(), "auth.login",
		"provider", "navikt",
		"user_id", user.ID.String(),
		"session_token_hash", obs.HashToken(cfg.sessionHMACKey, cfg.sm.Token(r.Context())))

	return middleware.SessionUser{
		ID:    user.ID.String(),
		Name:  user.Name,
		Email: user.Email,
		Role:  role,
	}, true, false
}

func naviktProviderID(claims NaviktClaims) string {
	if claims.NAVident != "" {
		return claims.NAVident
	}
	return claims.PreferredUsername
}

func upsertNaviktUser(r *http.Request, q db.Querier, providerID string, claims NaviktClaims) (db.User, error) {
	user, err := q.ClaimPendingUser(r.Context(), db.ClaimPendingUserParams{
		Email:      claims.PreferredUsername,
		Provider:   "navikt",
		ProviderID: providerID,
		Name:       claims.Name,
	})
	if err == nil {
		return user, nil
	}
	return q.UpsertUser(r.Context(), db.UpsertUserParams{
		Provider:   "navikt",
		ProviderID: providerID,
		Email:      claims.PreferredUsername,
		Name:       claims.Name,
	})
}

func cacheNaviktUser(r *http.Request, sm *scs.SessionManager, providerID string, user db.User) {
	sm.Put(r.Context(), naisCacheSubjectKey, providerID)
	sm.Put(r.Context(), naisCacheUserIDKey, user.ID.String())
	sm.Put(r.Context(), naisCacheNameKey, user.Name)
	sm.Put(r.Context(), naisCacheEmailKey, user.Email)
	sm.Put(r.Context(), naisCacheDBRoleKey, user.Role)
}

func userRoleForRequest(defaultRole string, claims NaviktClaims, adminGroups []string) string {
	if tokenHasAdminGroup(claims.Groups, adminGroups) {
		return "admin"
	}
	return defaultRole
}

func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}

func tokenHasAdminGroup(userGroups, adminGroups []string) bool {
	for _, ug := range userGroups {
		for _, ag := range adminGroups {
			if ug == ag {
				return true
			}
		}
	}
	return false
}

func cachedSessionUser(
	r *http.Request,
	sm *scs.SessionManager,
	claims NaviktClaims,
	adminGroups []string,
) (middleware.SessionUser, bool) {
	id := sm.GetString(r.Context(), naisCacheUserIDKey)
	if id == "" {
		return middleware.SessionUser{}, false
	}
	role := sm.GetString(r.Context(), naisCacheDBRoleKey)
	if role == "" {
		return middleware.SessionUser{}, false
	}
	if tokenHasAdminGroup(claims.Groups, adminGroups) {
		role = "admin"
	}
	return middleware.SessionUser{
		ID:    id,
		Name:  sm.GetString(r.Context(), naisCacheNameKey),
		Email: sm.GetString(r.Context(), naisCacheEmailKey),
		Role:  role,
	}, true
}

func isAllowedEmailDomain(email string, allowedDomains []string) bool {
	if len(allowedDomains) == 0 {
		return true
	}
	i := strings.LastIndex(email, "@")
	if i <= 0 || i == len(email)-1 {
		return false
	}
	domain := strings.ToLower(email[i+1:])
	for _, allowed := range allowedDomains {
		if domain == strings.ToLower(strings.TrimPrefix(strings.TrimSpace(allowed), "@")) {
			return true
		}
	}
	return false
}
