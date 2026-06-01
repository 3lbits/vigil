package assets

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/modregistry"
	"github.com/3lbits/vigil/internal/testutil"
	"github.com/google/uuid"
)

type assetsQ struct {
	testutil.StubQuerier
	assets       []db.Asset
	listErr      error
	createResult db.Asset
	createErr    error
	deleteErr    error
}

func (q *assetsQ) ListAssets(_ context.Context, _ db.ListAssetsParams) ([]db.Asset, error) {
	return q.assets, q.listErr
}
func (q *assetsQ) CreateAsset(_ context.Context, _ db.CreateAssetParams) (db.Asset, error) {
	return q.createResult, q.createErr
}
func (q *assetsQ) DeleteAsset(_ context.Context, _ uuid.UUID) error {
	return q.deleteErr
}

func testEngine(t *testing.T) *authz.Engine {
	t.Helper()
	e, err := authz.New(context.Background(), `
package authz
import rego.v1
default allow := false
allow if { input.user.role == "admin" }
`)
	if err != nil {
		t.Fatalf("compile policy: %v", err)
	}
	return e
}

func assetsCtx(r *http.Request) *http.Request {
	return r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "00000000-0000-0000-0000-000000000001", Name: "Alice", Role: "admin",
	}))
}

func TestValidAssetStatus(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: "active"},
		{in: "planned", want: "planned"},
		{in: "retired", want: "retired"},
		{in: "other", want: "active"},
	}
	for _, tc := range tests {
		if got := validAssetStatus(tc.in); got != tc.want {
			t.Fatalf("validAssetStatus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidAssetCriticality(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: "medium"},
		{in: "low", want: "low"},
		{in: "high", want: "high"},
		{in: "other", want: "medium"},
	}
	for _, tc := range tests {
		if got := validAssetCriticality(tc.in); got != tc.want {
			t.Fatalf("validAssetCriticality(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestList_DBError(t *testing.T) {
	h := NewHandler(&assetsQ{listErr: errors.New("db down")}, testEngine(t))
	r := assetsCtx(httptest.NewRequest(http.MethodGet, "/assets", nil))
	w := httptest.NewRecorder()

	h.List(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestList_OK(t *testing.T) {
	h := NewHandler(&assetsQ{assets: []db.Asset{{ID: uuid.New(), Name: "S3"}}}, testEngine(t))
	r := assetsCtx(httptest.NewRequest(http.MethodGet, "/assets", nil))
	w := httptest.NewRecorder()

	h.List(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCreate_OK(t *testing.T) {
	q := &assetsQ{createResult: db.Asset{ID: uuid.New(), Name: "Vault"}}
	h := NewHandler(q, testEngine(t))
	form := url.Values{
		"name":        {"Vault"},
		"description": {"Secrets store"},
		"asset_type":  {"system"},
		"owner":       {"secops"},
		"status":      {"planned"},
		"criticality": {"high"},
	}
	r := assetsCtx(httptest.NewRequest(http.MethodPost, "/assets", strings.NewReader(form.Encode())))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.Create(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
}

func TestCreate_DBErrorRendersForm(t *testing.T) {
	q := &assetsQ{createErr: errors.New("db down")}
	h := NewHandler(q, testEngine(t))
	form := url.Values{"name": {"Vault"}}
	r := assetsCtx(httptest.NewRequest(http.MethodPost, "/assets", strings.NewReader(form.Encode())))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.Create(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestShowEditUpdateDelete_InvalidID(t *testing.T) {
	h := NewHandler(&assetsQ{}, testEngine(t))
	methods := []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
	}{
		{name: "show", call: h.Show},
		{name: "edit", call: h.Edit},
		{name: "update", call: h.Update},
		{name: "delete", call: h.Delete},
	}
	for _, tc := range methods {
		r := assetsCtx(httptest.NewRequest(http.MethodPost, "/assets/not-a-uuid", nil))
		r.SetPathValue("id", "not-a-uuid")
		w := httptest.NewRecorder()
		tc.call(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s expected 404, got %d", tc.name, w.Code)
		}
	}
}

func TestModuleContract(t *testing.T) {
	m := New()
	if got := m.Name(); got != "assets" {
		t.Fatalf("module name = %q, want %q", got, "assets")
	}
	mux := http.NewServeMux()
	r := modregistry.NewRegistrar(mux, nil)
	if err := m.Register(modregistry.Dependencies{}, r); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	meta := r.Meta()
	if len(meta) != 7 {
		t.Fatalf("expected 7 routes, got %d", len(meta))
	}
}
