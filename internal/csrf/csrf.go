// Package csrf provides a double-submit-cookie CSRF middleware for VULN-008.
//
// Flow:
//  1. Middleware issues a signed token cookie (_csrf) on every request.
//  2. For mutating methods (POST/PUT/PATCH/DELETE) it validates the token
//     from the X-CSRF-Token header (HTMX) or the _csrf form field.
//  3. TokenFromContext exposes the token to templ components via context.
package csrf

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/3lbits/vigil/internal/httpresp"
)

const (
	cookieName  = "_csrf"
	headerName  = "X-CSRF-Token"
	fieldName   = "_csrf"
	maxBodySize = 10 << 20 // 10 MB — matches the largest handler body limit
)

type ctxKey struct{}

type SessionTokenGetter func(*http.Request) string

// TokenFromContext returns the CSRF token injected by Middleware.
// Call this from templ components to populate hidden form inputs.
func TokenFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}

// Middleware generates or reuses a signed CSRF token, stores it in a
// SameSite=Strict cookie and in the request context, and validates it on
// mutating requests. If sessionToken is provided and returns a non-empty value,
// the token MAC is additionally bound to that session token.
func Middleware(key []byte, secure bool, sessionToken SessionTokenGetter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session := tokenForRequest(r, sessionToken)
			token := issueToken(key, session, r, w, secure)
			ctx := context.WithValue(r.Context(), ctxKey{}, token)
			r = r.WithContext(ctx)

			if isMutating(r.Method) && !validateToken(r, token) {
				httpresp.Forbidden(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// WithToken stores tok in ctx so TokenFromContext can read it.
// Middleware does this per-request; exported for tests and for callers
// rendering templates outside an HTTP request.
func WithToken(ctx context.Context, tok string) context.Context {
	return context.WithValue(ctx, ctxKey{}, tok)
}

func isMutating(method string) bool {
	return method == http.MethodPost || method == http.MethodPut ||
		method == http.MethodPatch || method == http.MethodDelete
}

func validateToken(r *http.Request, token string) bool {
	// X-CSRF-Token header takes precedence (used by HTMX global config).
	if v := r.Header.Get(headerName); v != "" {
		return hmac.Equal([]byte(token), []byte(v))
	}

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		// Parse multipart to read _csrf field. Body size is bounded here so that
		// the handler's MaxBytesReader wrapper does not affect multipart parsing.
		r.Body = http.MaxBytesReader(nil, r.Body, maxBodySize)
		if err := r.ParseMultipartForm(maxBodySize); err != nil { //nolint:gosec // body is already bounded by MaxBytesReader above
			return false
		}
		return hmac.Equal([]byte(token), []byte(r.FormValue(fieldName)))
	}

	// URL-encoded form — ParseForm caches the result so handlers can re-read.
	if err := r.ParseForm(); err != nil {
		return false
	}
	return hmac.Equal([]byte(token), []byte(r.FormValue(fieldName)))
}

func tokenForRequest(r *http.Request, get SessionTokenGetter) string {
	if get == nil {
		return ""
	}
	return get(r)
}

func issueToken(key []byte, sessionToken string, r *http.Request, w http.ResponseWriter, secure bool) string {
	if c, err := r.Cookie(cookieName); err == nil && verifyToken(key, sessionToken, c.Value) {
		return c.Value
	}
	tok := newSignedToken(key, sessionToken)
	c := &http.Cookie{ //nolint:gosec // cookie has HttpOnly, SameSite=Strict, and Secure=true by default; Secure is only relaxed in dev via the conditional below
		Name:     cookieName,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   true,
	}
	if !secure {
		c.Secure = false // dev only; production enforces secure=true via config panic guard
	}
	http.SetCookie(w, c)
	return tok
}

func newSignedToken(key []byte, sessionToken string) string {
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	nonceHex := hex.EncodeToString(nonce)
	return nonceHex + "." + macHex(key, nonceAndSession(nonceHex, sessionToken))
}

func verifyToken(key []byte, sessionToken string, tok string) bool {
	dot := strings.IndexByte(tok, '.')
	if dot < 0 {
		return false
	}
	nonce, gotMac := tok[:dot], tok[dot+1:]
	return hmac.Equal([]byte(macHex(key, nonceAndSession(nonce, sessionToken))), []byte(gotMac))
}

func nonceAndSession(nonce, sessionToken string) string {
	if sessionToken == "" {
		return nonce
	}
	return nonce + "." + sessionToken
}

func macHex(key []byte, msg string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}
