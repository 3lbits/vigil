// Package auth handles OAuth2/OIDC login flows and session management.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/3lbits/vigil/internal/auth"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/obs"
	authtemplates "github.com/3lbits/vigil/internal/modules/auth/templates"
	"golang.org/x/oauth2"
)

type Handler struct {
	q              db.Querier
	providers      map[string]auth.Provider // slug → provider
	sm             *scs.SessionManager
	sessionHMACKey string
	cookieSecure   bool
}

func NewHandler(q db.Querier, providers []auth.Provider, sm *scs.SessionManager, sessionHMACKey string, cookieSecure bool) *Handler {
	m := make(map[string]auth.Provider, len(providers))
	for _, p := range providers {
		m[p.Name()] = p
	}
	return &Handler{
		q:              q,
		providers:      m,
		sm:             sm,
		sessionHMACKey: sessionHMACKey,
		cookieSecure:   cookieSecure,
	}
}

func providerLabel(slug string) string {
	switch slug {
	case "github":
		return "GitHub"
	case "entra":
		return "Microsoft"
	case "oidc":
		return "SSO"
	default:
		return slug
	}
}

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	infos := make([]authtemplates.ProviderInfo, 0, len(h.providers))
	for slug := range h.providers {
		infos = append(infos, authtemplates.ProviderInfo{
			Slug:  slug,
			Label: providerLabel(slug),
		})
	}
	if err := authtemplates.LoginPage(infos).Render(r.Context(), w); err != nil {
		slog.Error("render login page", "error", err)
	}
}

func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	provider, ok := h.providers[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}

	state, err := generateState()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- Secure is configurable; HttpOnly+SameSite are always set
		Name:     stateCookieName(slug),
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	authOpts := auth.AuthRequestOptions{}
	if isOIDCProvider(slug) {
		codeVerifier := oauth2.GenerateVerifier()
		nonce, nonceErr := generateState()
		if nonceErr != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		authOpts = auth.AuthRequestOptions{
			CodeVerifier: codeVerifier,
			Nonce:        nonce,
		}
		http.SetCookie(w, &http.Cookie{ // #nosec G124 -- nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- Secure is configurable; HttpOnly+SameSite are always set
			Name:     pkceCookieName(slug),
			Value:    codeVerifier,
			Path:     "/",
			MaxAge:   300,
			HttpOnly: true,
			Secure:   h.cookieSecure,
			SameSite: http.SameSiteLaxMode,
		})
		http.SetCookie(w, &http.Cookie{ // #nosec G124 -- nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- Secure is configurable; HttpOnly+SameSite are always set
			Name:     nonceCookieName(slug),
			Value:    nonce,
			Path:     "/",
			MaxAge:   300,
			HttpOnly: true,
			Secure:   h.cookieSecure,
			SameSite: http.SameSiteLaxMode,
		})
	}
	http.Redirect(w, r, provider.AuthCodeURL(state, authOpts), http.StatusFound)
}

func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	provider, ok := h.providers[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}

	stateCookie, err := r.Cookie(stateCookieName(slug))
	state := r.URL.Query().Get("state")
	if err != nil || subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(state)) != 1 {
		obs.SecurityEvent(r.Context(), "auth.state_mismatch", "provider", slug)
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- clearing cookie; security attributes set for defence in depth
		Name:     stateCookieName(slug),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	exchangeOpts := auth.ExchangeOptions{}
	if isOIDCProvider(slug) {
		verifierCookie, verifierErr := r.Cookie(pkceCookieName(slug))
		nonceCookie, nonceErr := r.Cookie(nonceCookieName(slug))
		if verifierErr != nil || nonceErr != nil || verifierCookie.Value == "" || nonceCookie.Value == "" {
			obs.SecurityEvent(r.Context(), "auth.pkce_or_nonce_missing", "provider", slug)
			http.Error(w, "invalid authentication flow", http.StatusBadRequest)
			return
		}
		exchangeOpts = auth.ExchangeOptions{
			CodeVerifier: verifierCookie.Value,
			Nonce:        nonceCookie.Value,
		}
		http.SetCookie(w, &http.Cookie{ // #nosec G124 -- nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- clearing cookie; security attributes set for defence in depth
			Name:     pkceCookieName(slug),
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   h.cookieSecure,
			SameSite: http.SameSiteLaxMode,
		})
		http.SetCookie(w, &http.Cookie{ // #nosec G124 -- nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- clearing cookie; security attributes set for defence in depth
			Name:     nonceCookieName(slug),
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   h.cookieSecure,
			SameSite: http.SameSiteLaxMode,
		})
	}

	identity, err := provider.Exchange(r.Context(), r.URL.Query().Get("code"), exchangeOpts)
	if err != nil {
		if errors.Is(err, auth.ErrVerifiedEmailRequired) {
			obs.SecurityEvent(r.Context(), "auth.login_denied", "provider", slug, "reason", "verified_email_required")
			http.Error(w, "verified email required", http.StatusUnauthorized)
			return
		}
		slog.Error("oauth exchange", "error", err)
		obs.SecurityEvent(r.Context(), "auth.login_failed", "provider", slug)
		http.Error(w, "authentication failed", http.StatusInternalServerError)
		return
	}

	// Claim a pre-created pending user (admin pre-provisioned by email) if one exists.
	// Falls back to normal upsert, which creates a new viewer account.
	user, err := h.q.ClaimPendingUser(r.Context(), db.ClaimPendingUserParams{
		Email:      identity.Email,
		Provider:   identity.Provider,
		ProviderID: identity.ProviderID,
		Name:       identity.Name,
	})
	if err != nil {
		user, err = h.q.UpsertUser(r.Context(), db.UpsertUserParams{
			Provider:   identity.Provider,
			ProviderID: identity.ProviderID,
			Email:      identity.Email,
			Name:       identity.Name,
		})
	}
	if err != nil {
		slog.Error("upsert user", "error", err)
		obs.SecurityEvent(r.Context(), "auth.login_failed", "provider", slug)
		http.Error(w, "authentication failed", http.StatusInternalServerError)
		return
	}

	// Rotate token to prevent session fixation.
	if err := h.sm.RenewToken(r.Context()); err != nil {
		slog.Error("renew session token", "error", err)
		http.Error(w, "authentication failed", http.StatusInternalServerError)
		return
	}
	h.sm.Put(r.Context(), "userID", user.ID.String())

	obs.SecurityEvent(r.Context(), "auth.login",
		"provider", slug,
		"user_id", user.ID.String(),
		"session_token_hash", obs.HashToken(h.sessionHMACKey, h.sm.Token(r.Context())))

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	obs.SecurityEvent(r.Context(), "auth.logout",
		"session_token_hash", obs.HashToken(h.sessionHMACKey, h.sm.Token(r.Context())))

	if err := h.sm.Destroy(r.Context()); err != nil {
		slog.Error("destroy session", "error", err)
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func stateCookieName(slug string) string {
	return "vigil_oauth_state_" + slug
}

func pkceCookieName(slug string) string {
	return "vigil_oauth_pkce_" + slug
}

func nonceCookieName(slug string) string {
	return "vigil_oauth_nonce_" + slug
}

func isOIDCProvider(slug string) bool {
	return slug == "oidc" || slug == "entra"
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	return hex.EncodeToString(b), nil
}
