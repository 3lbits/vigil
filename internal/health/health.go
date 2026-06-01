package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

// Pinger is satisfied by *pgxpool.Pool.
type Pinger interface {
	Ping(ctx context.Context) error
}

var moduleFlagsSettingsLoadFailed atomic.Bool

// SetModuleFlagsSettingsLoadFailure marks whether module flag loading is failing.
// This is process-wide state used by readiness checks.
func SetModuleFlagsSettingsLoadFailure(failed bool) {
	moduleFlagsSettingsLoadFailed.Store(failed)
}

// ModuleFlagsSettingsLoadFailed reports whether module flag loading is degraded.
func ModuleFlagsSettingsLoadFailed() bool {
	return moduleFlagsSettingsLoadFailed.Load()
}

// Liveness handles GET /healthz. It returns 200 as long as the process is running.
func Liveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Readiness returns a handler for GET /readyz. It pings the database and
// returns 503 if the ping fails.
func Readiness(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		w.Header().Set("Content-Type", "application/json")

		if ModuleFlagsSettingsLoadFailed() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "degraded"})
			return
		}

		if err := db.Ping(ctx); err != nil {
			slog.ErrorContext(r.Context(), "readyz ping failed", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "unavailable"})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
