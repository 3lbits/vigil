package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/testutil"
)

// mockKeySet implements gooidc.KeySet for testing. It always succeeds and returns
// the pre-set payload bytes, or an error if err is non-nil.
type mockKeySet struct {
	payload []byte
	err     error
}

func (m *mockKeySet) VerifySignature(_ context.Context, _ string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.payload, nil
}

// makeTestJWT builds a minimal JWT (header.payload.sig) with the given claims.
// The signature is not cryptographically valid; the mock key set ignores it.
func makeTestJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payloadBytes, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + payload + ".fake-sig"
}

func validClaims(issuer, clientID string) map[string]any {
	return map[string]any{
		"iss":                issuer,
		"aud":                []string{clientID},
		"exp":                time.Now().Add(time.Hour).Unix(),
		"NAVident":           "Z999999",
		"preferred_username": "z999999@nav.no",
		"name":               "Test User",
		"groups":             []string{"group-a", "group-admin"},
	}
}

func requestWithSessionCtx(t *testing.T, sm *scs.SessionManager, r *http.Request) *http.Request {
	t.Helper()
	ctx, err := sm.Load(r.Context(), "")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	return r.WithContext(ctx)
}

// --- NaviktVerifier tests ---

func TestNaviktVerifier_ValidToken(t *testing.T) {
	issuer := "https://login.microsoftonline.com/test/v2.0"
	clientID := "test-client"
	claims := validClaims(issuer, clientID)
	payload, _ := json.Marshal(claims)
	ks := &mockKeySet{payload: payload}

	v := newNaviktVerifierWithKeySet(issuer, clientID, ks)
	got, err := v.Verify(t.Context(), makeTestJWT(claims))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.NAVident != "Z999999" {
		t.Errorf("NAVident = %q, want Z999999", got.NAVident)
	}
	if got.PreferredUsername != "z999999@nav.no" {
		t.Errorf("PreferredUsername = %q, want z999999@nav.no", got.PreferredUsername)
	}
	if got.Name != "Test User" {
		t.Errorf("Name = %q, want Test User", got.Name)
	}
	if len(got.Groups) != 2 {
		t.Errorf("Groups len = %d, want 2", len(got.Groups))
	}
}

func TestNaviktVerifier_SignatureError(t *testing.T) {
	issuer := "https://login.microsoftonline.com/test/v2.0"
	clientID := "test-client"
	ks := &mockKeySet{err: errors.New("signature invalid")}

	v := newNaviktVerifierWithKeySet(issuer, clientID, ks)
	_, err := v.Verify(t.Context(), makeTestJWT(validClaims(issuer, clientID)))
	if err == nil {
		t.Fatal("expected error for invalid signature, got nil")
	}
	if !strings.Contains(err.Error(), "navikt: verify token") {
		t.Fatalf("error %q does not contain expected prefix", err.Error())
	}
}

func TestNaviktVerifier_ExpiredToken(t *testing.T) {
	issuer := "https://login.microsoftonline.com/test/v2.0"
	clientID := "test-client"
	claims := map[string]any{
		"iss": issuer,
		"aud": []string{clientID},
		"exp": time.Now().Add(-time.Hour).Unix(),
	}
	payload, _ := json.Marshal(claims)
	ks := &mockKeySet{payload: payload}

	v := newNaviktVerifierWithKeySet(issuer, clientID, ks)
	_, err := v.Verify(t.Context(), makeTestJWT(claims))
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestNaviktVerifier_WrongIssuer(t *testing.T) {
	issuer := "https://login.microsoftonline.com/test/v2.0"
	clientID := "test-client"
	claims := map[string]any{
		"iss": "https://attacker.example/v2.0",
		"aud": []string{clientID},
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	payload, _ := json.Marshal(claims)
	ks := &mockKeySet{payload: payload}

	v := newNaviktVerifierWithKeySet(issuer, clientID, ks)
	_, err := v.Verify(t.Context(), makeTestJWT(claims))
	if err == nil {
		t.Fatal("expected error for wrong issuer, got nil")
	}
}

func TestNaviktVerifier_WrongAudience(t *testing.T) {
	issuer := "https://login.microsoftonline.com/test/v2.0"
	clientID := "test-client"
	claims := map[string]any{
		"iss": issuer,
		"aud": []string{"other-client"},
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	payload, _ := json.Marshal(claims)
	ks := &mockKeySet{payload: payload}

	v := newNaviktVerifierWithKeySet(issuer, clientID, ks)
	_, err := v.Verify(t.Context(), makeTestJWT(claims))
	if err == nil {
		t.Fatal("expected error for wrong audience, got nil")
	}
}

func TestNaviktVerifier_MissingNAVident_FallsBackToPreferredUsername(t *testing.T) {
	issuer := "https://login.microsoftonline.com/test/v2.0"
	clientID := "test-client"
	claims := map[string]any{
		"iss":                issuer,
		"aud":                []string{clientID},
		"exp":                time.Now().Add(time.Hour).Unix(),
		"preferred_username": "external@partner.example",
		"name":               "External User",
	}
	payload, _ := json.Marshal(claims)
	ks := &mockKeySet{payload: payload}

	v := newNaviktVerifierWithKeySet(issuer, clientID, ks)
	got, err := v.Verify(t.Context(), makeTestJWT(claims))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.NAVident != "" {
		t.Errorf("expected empty NAVident, got %q", got.NAVident)
	}
	if got.PreferredUsername != "external@partner.example" {
		t.Errorf("PreferredUsername = %q, want external@partner.example", got.PreferredUsername)
	}
}

// --- NaviktMiddleware tests ---

type naviktMWQ struct {
	testutil.StubQuerier
	upsertUser  db.User
	upsertErr   error
	claimUser   db.User
	claimErr    error
	claimCalls  int
	upsertCalls int
}

func (q *naviktMWQ) UpsertUser(_ context.Context, _ db.UpsertUserParams) (db.User, error) {
	q.upsertCalls++
	return q.upsertUser, q.upsertErr
}

func (q *naviktMWQ) ClaimPendingUser(_ context.Context, _ db.ClaimPendingUserParams) (db.User, error) {
	q.claimCalls++
	return q.claimUser, q.claimErr
}

func TestNaviktMiddleware_NoBearerHeader(t *testing.T) {
	sm := scs.New()
	q := &naviktMWQ{}
	issuer := "https://issuer.example"
	clientID := "client"
	claims := validClaims(issuer, clientID)
	payload, _ := json.Marshal(claims)
	v := newNaviktVerifierWithKeySet(issuer, clientID, &mockKeySet{payload: payload})

	var hadUser bool
	h := NaviktMiddleware(v, sm, q, nil, nil, "")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, hadUser = middleware.FromContext(r.Context())
	}))
	r := requestWithSessionCtx(t, sm, httptest.NewRequest(http.MethodGet, "/", nil))
	h.ServeHTTP(httptest.NewRecorder(), r)

	if hadUser {
		t.Fatal("expected no user in context when no Authorization header")
	}
	if q.claimCalls != 0 || q.upsertCalls != 0 {
		t.Fatalf("expected no DB calls, got claim=%d upsert=%d", q.claimCalls, q.upsertCalls)
	}
}

func TestNaviktMiddleware_ValidTokenSetsUser(t *testing.T) {
	sm := scs.New()
	uid := uuid.New()
	q := &naviktMWQ{
		claimErr:   errors.New("no pending"),
		upsertUser: db.User{ID: uid, Name: "Test User", Email: "z999999@nav.no", Role: "viewer"},
	}
	issuer := "https://issuer.example"
	clientID := "client"
	claims := validClaims(issuer, clientID)
	payload, _ := json.Marshal(claims)
	v := newNaviktVerifierWithKeySet(issuer, clientID, &mockKeySet{payload: payload})

	var got middleware.SessionUser
	var ok bool
	h := NaviktMiddleware(v, sm, q, nil, []string{"nav.no"}, "hmac")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, ok = middleware.FromContext(r.Context())
	}))
	r := requestWithSessionCtx(t, sm, httptest.NewRequest(http.MethodGet, "/", nil))
	r.Header.Set("Authorization", "Bearer "+makeTestJWT(claims))
	h.ServeHTTP(httptest.NewRecorder(), r)

	if !ok {
		t.Fatal("expected user in context")
	}
	if got.ID != uid.String() {
		t.Errorf("ID = %q, want %q", got.ID, uid.String())
	}
	if got.Role != "viewer" {
		t.Errorf("Role = %q, want viewer", got.Role)
	}
	if q.claimCalls != 1 || q.upsertCalls != 1 {
		t.Fatalf("expected claim+upsert once, got claim=%d upsert=%d", q.claimCalls, q.upsertCalls)
	}
}

func TestNaviktMiddleware_CacheHitSkipsDB(t *testing.T) {
	sm := scs.New()
	uid := uuid.New()
	q := &naviktMWQ{
		claimErr:   errors.New("no pending"),
		upsertUser: db.User{ID: uid, Name: "Test User", Email: "z999999@nav.no", Role: "viewer"},
	}
	issuer := "https://issuer.example"
	clientID := "client"
	claims := validClaims(issuer, clientID)
	payload, _ := json.Marshal(claims)
	v := newNaviktVerifierWithKeySet(issuer, clientID, &mockKeySet{payload: payload})

	h := NaviktMiddleware(v, sm, q, nil, []string{"nav.no"}, "")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))

	r1 := requestWithSessionCtx(t, sm, httptest.NewRequest(http.MethodGet, "/", nil))
	r1.Header.Set("Authorization", "Bearer "+makeTestJWT(claims))
	h.ServeHTTP(httptest.NewRecorder(), r1)
	token, _, err := sm.Commit(r1.Context())
	if err != nil {
		t.Fatalf("commit session: %v", err)
	}

	ctx2, err := sm.Load(context.Background(), token)
	if err != nil {
		t.Fatalf("load cached session: %v", err)
	}
	r2 := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx2)
	r2.Header.Set("Authorization", "Bearer "+makeTestJWT(claims))
	h.ServeHTTP(httptest.NewRecorder(), r2)

	if q.claimCalls != 1 || q.upsertCalls != 1 {
		t.Fatalf("expected cache hit on second request, got claim=%d upsert=%d", q.claimCalls, q.upsertCalls)
	}
}

func TestNaviktMiddleware_AdminGroupPromotion(t *testing.T) {
	sm := scs.New()
	uid := uuid.New()
	q := &naviktMWQ{
		claimErr:   errors.New("no pending"),
		upsertUser: db.User{ID: uid, Name: "Test User", Email: "z999999@nav.no", Role: "viewer"},
	}
	issuer := "https://issuer.example"
	clientID := "client"
	claims := validClaims(issuer, clientID)
	payload, _ := json.Marshal(claims)
	v := newNaviktVerifierWithKeySet(issuer, clientID, &mockKeySet{payload: payload})

	var got middleware.SessionUser
	h := NaviktMiddleware(v, sm, q, []string{"group-admin"}, []string{"nav.no"}, "")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = middleware.FromContext(r.Context())
	}))
	r := requestWithSessionCtx(t, sm, httptest.NewRequest(http.MethodGet, "/", nil))
	r.Header.Set("Authorization", "Bearer "+makeTestJWT(claims))
	h.ServeHTTP(httptest.NewRecorder(), r)

	if got.Role != "admin" {
		t.Errorf("Role = %q, want admin (user is in admin group)", got.Role)
	}
}

func TestNaviktMiddleware_InvalidTokenPassesThrough(t *testing.T) {
	sm := scs.New()
	v := newNaviktVerifierWithKeySet("https://issuer.example", "client", &mockKeySet{err: errors.New("bad key")})
	q := &naviktMWQ{}

	var hadUser bool
	h := NaviktMiddleware(v, sm, q, nil, nil, "")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, hadUser = middleware.FromContext(r.Context())
	}))
	r := requestWithSessionCtx(t, sm, httptest.NewRequest(http.MethodGet, "/", nil))
	r.Header.Set("Authorization", "Bearer some.fake.token")
	h.ServeHTTP(httptest.NewRecorder(), r)

	if hadUser {
		t.Fatal("expected no user in context when token is invalid")
	}
}

func TestNaviktMiddleware_DBErrorPassesThrough(t *testing.T) {
	sm := scs.New()
	issuer := "https://issuer.example"
	clientID := "client"
	claims := validClaims(issuer, clientID)
	payload, _ := json.Marshal(claims)
	v := newNaviktVerifierWithKeySet(issuer, clientID, &mockKeySet{payload: payload})
	q := &naviktMWQ{
		claimErr:  errors.New("no pending"),
		upsertErr: errors.New("db: connection reset"),
	}

	var hadUser bool
	h := NaviktMiddleware(v, sm, q, nil, []string{"nav.no"}, "")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, hadUser = middleware.FromContext(r.Context())
	}))
	r := requestWithSessionCtx(t, sm, httptest.NewRequest(http.MethodGet, "/", nil))
	r.Header.Set("Authorization", "Bearer "+makeTestJWT(claims))
	h.ServeHTTP(httptest.NewRecorder(), r)

	if hadUser {
		t.Fatal("expected no user in context when DB upsert fails")
	}
}

func TestNaviktMiddleware_PendingUserClaimed(t *testing.T) {
	sm := scs.New()
	uid := uuid.New()
	q := &naviktMWQ{
		claimUser: db.User{ID: uid, Name: "Pre-created", Email: "z999999@nav.no", Role: "editor"},
	}
	issuer := "https://issuer.example"
	clientID := "client"
	claims := validClaims(issuer, clientID)
	payload, _ := json.Marshal(claims)
	v := newNaviktVerifierWithKeySet(issuer, clientID, &mockKeySet{payload: payload})

	var got middleware.SessionUser
	h := NaviktMiddleware(v, sm, q, nil, []string{"nav.no"}, "")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = middleware.FromContext(r.Context())
	}))
	r := requestWithSessionCtx(t, sm, httptest.NewRequest(http.MethodGet, "/", nil))
	r.Header.Set("Authorization", "Bearer "+makeTestJWT(claims))
	h.ServeHTTP(httptest.NewRecorder(), r)

	if got.Role != "editor" {
		t.Errorf("Role = %q, want editor (claimed pending user)", got.Role)
	}
	if q.claimCalls != 1 || q.upsertCalls != 0 {
		t.Fatalf("expected only claim for pending user, got claim=%d upsert=%d", q.claimCalls, q.upsertCalls)
	}
}

func TestNaviktMiddleware_EmailDomainDenied(t *testing.T) {
	sm := scs.New()
	q := &naviktMWQ{}
	issuer := "https://issuer.example"
	clientID := "client"
	claims := validClaims(issuer, clientID)
	claims["preferred_username"] = "z999999@example.com"
	payload, _ := json.Marshal(claims)
	v := newNaviktVerifierWithKeySet(issuer, clientID, &mockKeySet{payload: payload})

	h := NaviktMiddleware(v, sm, q, nil, []string{"nav.no"}, "")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler should not be called on denied domain")
	}))
	r := requestWithSessionCtx(t, sm, httptest.NewRequest(http.MethodGet, "/", nil))
	r.Header.Set("Authorization", "Bearer "+makeTestJWT(claims))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	if q.claimCalls != 0 || q.upsertCalls != 0 {
		t.Fatalf("expected no DB calls for denied domain, got claim=%d upsert=%d", q.claimCalls, q.upsertCalls)
	}
}
