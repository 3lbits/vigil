package app

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/modregistry"
	"github.com/3lbits/vigil/internal/modules/about"
	"github.com/3lbits/vigil/internal/modules/activities"
	"github.com/3lbits/vigil/internal/modules/admin"
	"github.com/3lbits/vigil/internal/modules/assets"
	authmodule "github.com/3lbits/vigil/internal/modules/auth"
	"github.com/3lbits/vigil/internal/modules/avvik"
	"github.com/3lbits/vigil/internal/modules/compliance"
	"github.com/3lbits/vigil/internal/modules/dashboard"
	"github.com/3lbits/vigil/internal/modules/me"
	"github.com/3lbits/vigil/internal/modules/measures"
	"github.com/3lbits/vigil/internal/modules/risk"
	"github.com/alexedwards/scs/v2"
)

func TestLoginExemptRoutesAreExactlyExpected(t *testing.T) {
	gotExact, gotPrefixes := loginExemptRoutes()
	wantExact := []middleware.PublicRoute{
		{Method: http.MethodGet, Path: "/healthz"},
		{Method: http.MethodGet, Path: "/readyz"},
		{Method: http.MethodPost, Path: "/locale"},
		{Method: http.MethodGet, Path: "/login"},
	}
	wantPrefixes := []string{"/public/", "/auth/"}

	if !reflect.DeepEqual(gotExact, wantExact) {
		t.Fatalf("public exact route list changed: got %+v want %+v", gotExact, wantExact)
	}
	if !reflect.DeepEqual(gotPrefixes, wantPrefixes) {
		t.Fatalf("public prefix list changed: got %+v want %+v", gotPrefixes, wantPrefixes)
	}
}

func TestModulePublicRoutesAreExactlyExpected(t *testing.T) {
	reg := modregistry.NewRegistry()
	mods := []modregistry.Module{
		about.New(),
		dashboard.New(),
		compliance.New(),
		measures.New(),
		me.New(),
		assets.New(),
		activities.New(),
		risk.New(),
		avvik.New(nil),
		admin.New(nil, time.Unix(0, 0), "test"),
		authmodule.New(nil, scs.New(), "test-key", false),
	}
	for _, m := range mods {
		if err := reg.Register(m); err != nil {
			t.Fatalf("register module %q: %v", m.Name(), err)
		}
	}
	if err := reg.MountAll(http.NewServeMux(), modregistry.Dependencies{}); err != nil {
		t.Fatalf("mount modules: %v", err)
	}

	got := map[string]bool{}
	for _, m := range reg.RouteMeta() {
		if m.Public {
			got[m.Pattern] = true
		}
	}
	want := map[string]bool{
		"GET /login":                true,
		"GET /auth/{slug}":          true,
		"GET /auth/{slug}/callback": true,
		"POST /logout":              true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("module public route set changed: got %v want %v", got, want)
	}
}
