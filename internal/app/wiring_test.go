package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/config"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/locale"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/obs"
	"github.com/3lbits/vigil/internal/testutil"
)

type devUsersQ struct {
	testutil.StubQuerier
	users []db.User
	err   error
}

func (q *devUsersQ) ListDevStubUsers(context.Context) ([]db.User, error) {
	return q.users, q.err
}

func TestRun_RequiresConfig(t *testing.T) {
	err := Run(t.Context(), nil, Options{})
	if !errors.Is(err, errConfigRequired) {
		t.Fatalf("expected errConfigRequired, got %v", err)
	}
}

func TestBuildProviders(t *testing.T) {
	_, err := buildProviders(t.Context(), &config.Config{})
	if !errors.Is(err, errNoAuthProviders) {
		t.Fatalf("expected errNoAuthProviders, got %v", err)
	}

	_, err = buildProviders(t.Context(), &config.Config{AuthProviders: []string{"bogus"}})
	if !errors.Is(err, errUnknownAuthProvider) {
		t.Fatalf("expected errUnknownAuthProvider, got %v", err)
	}

	ps, err := buildProviders(t.Context(), &config.Config{
		AuthProviders:       []string{"github"},
		GitHubClientID:      "id",
		GitHubClientSecret:  "secret",
		AppBaseURL:          "https://example.test",
		SessionCookieSecure: true,
	})
	if err != nil {
		t.Fatalf("github provider should build: %v", err)
	}
	if len(ps) != 1 || ps[0].Name() != "github" {
		t.Fatalf("unexpected providers: %#v", ps)
	}
}

func TestRegisterLocaleRoute_SetsCookieAndRedirects(t *testing.T) {
	mux := http.NewServeMux()
	cfg := &config.Config{SessionCookieSecure: true}
	registerLocaleRoute(mux, cfg)

	form := url.Values{"lang": {"invalid"}}
	r := httptest.NewRequest(http.MethodPost, "/locale", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Referer", "https://app.example/dashboard?tab=1")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/dashboard?tab=1" {
		t.Fatalf("unexpected redirect location: %q", loc)
	}
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == locale.CookieName {
			found = true
			if c.Value != locale.DefaultLang {
				t.Fatalf("expected default lang cookie value %q, got %q", locale.DefaultLang, c.Value)
			}
		}
	}
	if !found {
		t.Fatalf("expected %q cookie to be set", locale.CookieName)
	}
}

func TestRegisterMetricsRoute_RegistersHandler(t *testing.T) {
	engine, err := authz.New(context.Background(), `
package authz
import rego.v1
default allow := false
allow if {
  input.resource == "admin"
  input.action == "read"
  input.user.role == "admin"
}
`)
	if err != nil {
		t.Fatalf("compile policy: %v", err)
	}
	state := appState{
		engine: engine,
		reg:    prometheus.NewRegistry(),
	}
	mux := http.NewServeMux()
	cfg := &config.Config{MetricsPath: "/metrics"}
	registerMetricsRoute(mux, cfg, state)

	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: uuid.NewString(), Name: "Admin", Role: "admin",
	}))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, r)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected metrics route to be registered, got 404")
	}
}

func TestRegisterDevRoleRoute(t *testing.T) {
	adminID := uuid.New()
	viewerID := uuid.New()
	q := &devUsersQ{users: []db.User{
		{ID: adminID, Role: "admin"},
		{ID: viewerID, Role: "viewer"},
	}}

	mux := http.NewServeMux()
	registerDevRoleRoute(mux, true, false, q)
	form := url.Values{"role": {"not-allowed"}}
	r := httptest.NewRequest(http.MethodPost, "/dev/role", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Referer", "https://app.example/dashboard")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/dashboard" {
		t.Fatalf("unexpected redirect location: %q", loc)
	}
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == middleware.DevUserCookieName {
			found = true
			if c.Value != adminID.String() {
				t.Fatalf("expected fallback admin ID cookie, got %q", c.Value)
			}
		}
	}
	if !found {
		t.Fatalf("expected %q cookie", middleware.DevUserCookieName)
	}
}

func TestRun_BootstrapError(t *testing.T) {
	cfg := &config.Config{
		Port:        "0",
		AppEnv:      "test",
		DatabaseURL: "postgres://127.0.0.1:1/invalid?sslmode=disable",
	}
	err := Run(t.Context(), cfg, Options{PolicySource: "package authz\nimport rego.v1\ndefault allow := false"})
	if err == nil || !strings.Contains(err.Error(), "database connect") {
		t.Fatalf("expected database connect error, got %v", err)
	}
}

func TestBuildMux_DevStubDisabled(t *testing.T) {
	engine, err := authz.New(context.Background(), `
package authz
import rego.v1
default allow := false
`)
	if err != nil {
		t.Fatalf("compile policy: %v", err)
	}
	state := appState{engine: engine}
	cfg := &config.Config{
		DevStubAuth:         false,
		AuthProviders:       []string{"github"},
		GitHubClientID:      "id",
		GitHubClientSecret:  "secret",
		AppBaseURL:          "https://app.example",
		SessionIdleTimeout:  time.Hour,
		SessionCookieName:   "session",
		SessionCookieSecure: true,
		SessionHMACKey:      "hmac",
	}
	mux, sm, err := buildMux(t.Context(), cfg, state, Options{})
	if err != nil {
		t.Fatalf("buildMux failed: %v", err)
	}
	if mux == nil || sm == nil {
		t.Fatalf("expected mux and session manager to be non-nil")
	}
}

func TestWithMiddleware_BothBranches(t *testing.T) {
	bundle, err := locale.NewBundle()
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	reg := prometheus.NewRegistry()
	state := appState{
		bundle:       bundle,
		metrics:      obs.NewMetrics(reg),
		reg:          reg,
		csrfKey:      []byte("01234567890123456789012345678901"),
		trustedCIDRs: []*net.IPNet{},
	}
	cfgDev := &config.Config{DevStubAuth: true, SessionCookieSecure: true, AppEnv: "test"}
	cfgAuth := &config.Config{DevStubAuth: false, SessionCookieSecure: true, AppEnv: "test"}
	sm := scs.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	h1 := withMiddleware(cfgDev, state, sm, mux)
	if h1 == nil {
		t.Fatal("expected dev handler")
	}
	h2 := withMiddleware(cfgAuth, state, sm, mux)
	if h2 == nil {
		t.Fatal("expected auth handler")
	}
}

type closeTrackState struct {
	closed bool
}
type closeTrackDriver struct{ state *closeTrackState }
type closeTrackConn struct{ state *closeTrackState }

func (d *closeTrackDriver) Open(string) (driver.Conn, error) {
	return &closeTrackConn{state: d.state}, nil
}
func (c *closeTrackConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unsupported") }
func (c *closeTrackConn) Close() error {
	c.state.closed = true
	return nil
}
func (c *closeTrackConn) Begin() (driver.Tx, error)  { return nil, errors.New("unsupported") }
func (c *closeTrackConn) Ping(context.Context) error { return nil }

func TestCloseDB_ClosesSQLDB(t *testing.T) {
	state := &closeTrackState{}
	driverName := "close_track_" + uuid.NewString()
	sql.Register(driverName, &closeTrackDriver{state: state})
	dbConn, err := sql.Open(driverName, "unused")
	if err != nil {
		t.Fatalf("open sql: %v", err)
	}
	if err := dbConn.Ping(); err != nil {
		t.Fatalf("ping sql: %v", err)
	}
	s := appState{sqlDB: dbConn}
	s.closeDB()
	if !state.closed {
		t.Fatal("expected sql db close to be called")
	}
}
