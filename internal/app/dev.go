package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/devseed"
	"github.com/3lbits/vigil/internal/middleware"
)

func registerDevRoleRoute(mux *http.ServeMux, enabled, secureCookie bool, q db.Querier) {
	if !enabled {
		return
	}
	mux.HandleFunc("POST /dev/role", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 256)
		role := strings.TrimSpace(r.FormValue("role"))
		if !middleware.IsAllowedDevRole(role) {
			role = "admin"
		}
		users, err := q.ListDevStubUsers(r.Context())
		if err != nil || len(users) == 0 {
			http.Error(w, "failed to load development users", http.StatusInternalServerError)
			return
		}
		selected := users[0]
		for _, u := range users {
			if u.Role == role {
				selected = u
				break
			}
		}
		http.SetCookie(w, &http.Cookie{ //nolint:gosec // nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
			Name:     middleware.DevUserCookieName,
			Value:    selected.ID.String(),
			Path:     "/",
			MaxAge:   365 * 24 * 3600,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   secureCookie,
		})
		http.SetCookie(w, &http.Cookie{ //nolint:gosec // nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
			Name:     middleware.DevRoleCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   secureCookie,
		})
		http.Redirect(w, r, redirectPathFromReferer(r.Header.Get("Referer")), http.StatusSeeOther) //nolint:gosec // redirectPathFromReferer enforces a safe local path via safeLocalRedirect
	})
}

func seedDevStubUsers(ctx context.Context, q db.Querier) error {
	_, err := devseed.SeedStubUsers(ctx, q)
	if err != nil {
		return fmt.Errorf("seed stub users: %w", err)
	}
	return nil
}
