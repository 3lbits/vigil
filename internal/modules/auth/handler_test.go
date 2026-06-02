package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"

	authpkg "github.com/3lbits/vigil/internal/auth"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/testutil"
)

type authProviderStub struct {
	name         string
	lastState    string
	lastAuthOpts authpkg.AuthRequestOptions
	lastCode     string
	lastExOpts   authpkg.ExchangeOptions
	identity     authpkg.Identity
	exchangeErr  error
}

func (p *authProviderStub) Name() string { return p.name }

func (p *authProviderStub) AuthCodeURL(state string, opts authpkg.AuthRequestOptions) string {
	p.lastState = state
	p.lastAuthOpts = opts
	return "/idp/auth?state=" + state
}

func (p *authProviderStub) Exchange(_ context.Context, code string, opts authpkg.ExchangeOptions) (authpkg.Identity, error) {
	p.lastCode = code
	p.lastExOpts = opts
	if p.exchangeErr != nil {
		return authpkg.Identity{}, p.exchangeErr
	}
	return p.identity, nil
}

type authQ struct {
	testutil.StubQuerier
	claimUser   db.User
	claimErr    error
	upsertUser  db.User
	upsertErr   error
	claimCalls  int
	upsertCalls int
}

func (q *authQ) ClaimPendingUser(_ context.Context, _ db.ClaimPendingUserParams) (db.User, error) {
	q.claimCalls++
	return q.claimUser, q.claimErr
}

func (q *authQ) UpsertUser(_ context.Context, _ db.UpsertUserParams) (db.User, error) {
	q.upsertCalls++
	return q.upsertUser, q.upsertErr
}

func withSession(t *testing.T, sm *scs.SessionManager, r *http.Request) *http.Request {
	t.Helper()
	ctx, err := sm.Load(r.Context(), "")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	return r.WithContext(ctx)
}

func responseCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestRedirect_UnknownProvider(t *testing.T) {
	sm := scs.New()
	h := NewHandler(&authQ{}, nil, nil, sm, "test-key", false)
	r := withSession(t, sm, httptest.NewRequest(http.MethodGet, "/auth/unknown", nil))
	r.SetPathValue("slug", "unknown")
	w := httptest.NewRecorder()

	h.Redirect(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRedirect_OIDC_SetsStatePKCEAndNonceCookies(t *testing.T) {
	sm := scs.New()
	provider := &authProviderStub{name: "oidc"}
	h := NewHandler(&authQ{}, []authpkg.Provider{provider}, nil, sm, "test-key", false)

	r := withSession(t, sm, httptest.NewRequest(http.MethodGet, "/auth/oidc", nil))
	r.SetPathValue("slug", "oidc")
	w := httptest.NewRecorder()
	h.Redirect(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if provider.lastState == "" {
		t.Fatal("expected provider to receive state")
	}
	if provider.lastAuthOpts.CodeVerifier == "" || provider.lastAuthOpts.Nonce == "" {
		t.Fatal("expected provider to receive PKCE verifier and nonce for OIDC")
	}
	cookies := w.Result().Cookies()
	if responseCookie(cookies, stateCookieName("oidc")) == nil {
		t.Fatal("missing state cookie")
	}
	if responseCookie(cookies, pkceCookieName("oidc")) == nil {
		t.Fatal("missing pkce cookie")
	}
	if responseCookie(cookies, nonceCookieName("oidc")) == nil {
		t.Fatal("missing nonce cookie")
	}
}

func TestCallback_StateMismatchRejected(t *testing.T) {
	sm := scs.New()
	provider := &authProviderStub{name: "oidc"}
	h := NewHandler(&authQ{}, []authpkg.Provider{provider}, nil, sm, "test-key", false)

	r := withSession(t, sm, httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=bad&code=x", nil))
	r.SetPathValue("slug", "oidc")
	r.AddCookie(&http.Cookie{Name: stateCookieName("oidc"), Value: "good"})
	w := httptest.NewRecorder()
	h.Callback(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCallback_OIDCMissingPKCEOrNonceRejected(t *testing.T) {
	sm := scs.New()
	provider := &authProviderStub{name: "oidc"}
	h := NewHandler(&authQ{}, []authpkg.Provider{provider}, nil, sm, "test-key", false)

	r := withSession(t, sm, httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=s1&code=c1", nil))
	r.SetPathValue("slug", "oidc")
	r.AddCookie(&http.Cookie{Name: stateCookieName("oidc"), Value: "s1"})
	w := httptest.NewRecorder()
	h.Callback(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCallback_OIDCSuccessClearsCookiesAndSetsSession(t *testing.T) {
	sm := scs.New()
	userID := uuid.New()
	provider := &authProviderStub{
		name: "oidc",
		identity: authpkg.Identity{
			Provider:   "oidc",
			ProviderID: "sub-123",
			Email:      "user@example.com",
			Name:       "User",
		},
	}
	h := NewHandler(&authQ{
		claimUser: db.User{ID: userID, Email: "user@example.com", Name: "User", Role: "viewer"},
	}, []authpkg.Provider{provider}, nil, sm, "test-key", false)

	r := withSession(t, sm, httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=s-ok&code=code-ok", nil))
	r.SetPathValue("slug", "oidc")
	r.AddCookie(&http.Cookie{Name: stateCookieName("oidc"), Value: "s-ok"})
	r.AddCookie(&http.Cookie{Name: pkceCookieName("oidc"), Value: "verifier-ok"})
	r.AddCookie(&http.Cookie{Name: nonceCookieName("oidc"), Value: "nonce-ok"})
	w := httptest.NewRecorder()
	h.Callback(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Fatalf("expected redirect to '/', got %q", loc)
	}
	if provider.lastCode != "code-ok" {
		t.Fatalf("expected code passed to exchange, got %q", provider.lastCode)
	}
	if provider.lastExOpts.CodeVerifier != "verifier-ok" || provider.lastExOpts.Nonce != "nonce-ok" {
		t.Fatalf("unexpected exchange options: %+v", provider.lastExOpts)
	}
	if got := sm.GetString(r.Context(), "userID"); got != userID.String() {
		t.Fatalf("expected session userID set to %q, got %q", userID.String(), got)
	}

	cookies := w.Result().Cookies()
	for _, name := range []string{stateCookieName("oidc"), pkceCookieName("oidc"), nonceCookieName("oidc")} {
		c := responseCookie(cookies, name)
		if c == nil || c.MaxAge >= 0 {
			t.Fatalf("expected cleared cookie for %s, got %+v", name, c)
		}
	}
}

func TestLogout_DestroysSessionAndRedirectsToLogin(t *testing.T) {
	sm := scs.New()
	h := NewHandler(&authQ{}, nil, nil, sm, "test-key", false)

	r := withSession(t, sm, httptest.NewRequest(http.MethodPost, "/auth/logout", nil))
	sm.Put(r.Context(), "userID", "uid-logout")
	w := httptest.NewRecorder()

	h.Logout(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/login" {
		t.Fatalf("expected redirect to /login, got %q", got)
	}
	if got := sm.GetString(r.Context(), "userID"); got != "" {
		t.Fatalf("expected session to be destroyed, userID=%q", got)
	}
}

func TestCallback_ExchangeError(t *testing.T) {
	sm := scs.New()
	provider := &authProviderStub{name: "oidc", exchangeErr: errors.New("exchange failed")}
	h := NewHandler(&authQ{}, []authpkg.Provider{provider}, nil, sm, "test-key", false)

	r := withSession(t, sm, httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=s1&code=c1", nil))
	r.SetPathValue("slug", "oidc")
	r.AddCookie(&http.Cookie{Name: stateCookieName("oidc"), Value: "s1"})
	r.AddCookie(&http.Cookie{Name: pkceCookieName("oidc"), Value: "v1"})
	r.AddCookie(&http.Cookie{Name: nonceCookieName("oidc"), Value: "n1"})
	w := httptest.NewRecorder()
	h.Callback(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCallback_GitHubMissingVerifiedEmailRejected(t *testing.T) {
	sm := scs.New()
	provider := &authProviderStub{name: "github", exchangeErr: authpkg.ErrVerifiedEmailRequired}
	q := &authQ{}
	h := NewHandler(q, []authpkg.Provider{provider}, nil, sm, "test-key", false)

	r := withSession(t, sm, httptest.NewRequest(http.MethodGet, "/auth/github/callback?state=s1&code=c1", nil))
	r.SetPathValue("slug", "github")
	r.AddCookie(&http.Cookie{Name: stateCookieName("github"), Value: "s1"})
	w := httptest.NewRecorder()
	h.Callback(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if q.claimCalls != 0 {
		t.Fatalf("expected no claim call, got %d", q.claimCalls)
	}
	if q.upsertCalls != 0 {
		t.Fatalf("expected no upsert call, got %d", q.upsertCalls)
	}
}

func TestCallback_EmailDomainNotAllowedRejected(t *testing.T) {
	sm := scs.New()
	provider := &authProviderStub{
		name: "github",
		identity: authpkg.Identity{
			Provider:   "github",
			ProviderID: "123",
			Email:      "user@outside.test",
			Name:       "User",
		},
	}
	q := &authQ{}
	h := NewHandler(q, []authpkg.Provider{provider}, []string{"example.com"}, sm, "test-key", false)

	r := withSession(t, sm, httptest.NewRequest(http.MethodGet, "/auth/github/callback?state=s1&code=c1", nil))
	r.SetPathValue("slug", "github")
	r.AddCookie(&http.Cookie{Name: stateCookieName("github"), Value: "s1"})
	w := httptest.NewRecorder()
	h.Callback(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if q.claimCalls != 0 {
		t.Fatalf("expected no claim call, got %d", q.claimCalls)
	}
	if q.upsertCalls != 0 {
		t.Fatalf("expected no upsert call, got %d", q.upsertCalls)
	}
}

func TestCallback_EmailDomainAllowedIsCaseInsensitive(t *testing.T) {
	sm := scs.New()
	userID := uuid.New()
	provider := &authProviderStub{
		name: "github",
		identity: authpkg.Identity{
			Provider:   "github",
			ProviderID: "123",
			Email:      "User@Example.com",
			Name:       "User",
		},
	}
	q := &authQ{
		claimUser: db.User{ID: userID, Email: "User@Example.com", Name: "User", Role: "viewer"},
	}
	h := NewHandler(q, []authpkg.Provider{provider}, []string{"example.com"}, sm, "test-key", false)

	r := withSession(t, sm, httptest.NewRequest(http.MethodGet, "/auth/github/callback?state=s1&code=c1", nil))
	r.SetPathValue("slug", "github")
	r.AddCookie(&http.Cookie{Name: stateCookieName("github"), Value: "s1"})
	w := httptest.NewRecorder()
	h.Callback(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if q.claimCalls != 1 {
		t.Fatalf("expected one claim call, got %d", q.claimCalls)
	}
}
