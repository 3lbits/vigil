package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/3lbits/vigil/internal/httpresp"
)

type contextKey string

const cspNonceContextKey contextKey = "csp_nonce"

// CSPNonceFromContext returns the per-request CSP nonce for script tags.
func CSPNonceFromContext(ctx context.Context) string {
	nonce, _ := ctx.Value(cspNonceContextKey).(string)
	return nonce
}

// SecurityHeaders adds HTTP security headers to every response.
// hstsEnabled is evaluated once at startup (not per-request) to avoid
// env var reads on the hot path and case-sensitivity bugs.
// Must be outermost middleware so headers are set even on error responses.
func SecurityHeaders(hstsEnabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nonce, err := generateCSPNonce()
			if err != nil {
				httpresp.InternalServerError(w, r)
				return
			}
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("Cross-Origin-Resource-Policy", "same-origin")
			h.Set("Cache-Control", "no-store, private")
			h.Set("Content-Security-Policy",
				"default-src 'self'; "+
					fmt.Sprintf("script-src 'self' 'nonce-%s'; ", nonce)+
					"style-src 'self'; "+
					"style-src-elem 'self' 'unsafe-inline'; "+
					"font-src 'self'; "+
					"img-src 'self' data:; "+
					"frame-src 'self' blob: data:; "+
					"object-src 'none'; "+
					"form-action 'self'; "+
					"frame-ancestors 'none'; "+
					"base-uri 'self'")
			if hstsEnabled {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			ctx := context.WithValue(r.Context(), cspNonceContextKey, nonce)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func generateCSPNonce() (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate csp nonce: %w", err)
	}
	return base64.RawStdEncoding.EncodeToString(nonce), nil
}
