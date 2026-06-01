package auth

import (
	"log/slog"
	"net/http"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
)

// UserMiddleware reads the authenticated user ID from the SCS session and
// injects a SessionUser into the request context. Requests with no session
// or an invalid user ID continue unauthenticated.
func UserMiddleware(sm *scs.SessionManager, q db.Querier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userIDStr := sm.GetString(r.Context(), "userID")
			if userIDStr == "" {
				next.ServeHTTP(w, r)
				return
			}

			id, err := uuid.Parse(userIDStr)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			user, err := q.GetUserByID(r.Context(), id)
			if err != nil {
				slog.Error("load session user", "error", err)
				next.ServeHTTP(w, r)
				return
			}

			ctx := middleware.SetUser(r.Context(), middleware.SessionUser{
				ID:    user.ID.String(),
				Name:  user.Name,
				Email: user.Email,
				Role:  user.Role,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
