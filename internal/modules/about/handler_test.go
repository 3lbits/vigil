package about

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/modregistry"
)

func TestShow_OK(t *testing.T) {
	h := NewHandler()
	r := httptest.NewRequest(http.MethodGet, "/about", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "00000000-0000-0000-0000-000000000001", Name: "Alice", Role: "viewer",
	}))
	w := httptest.NewRecorder()

	h.Show(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestModuleContract(t *testing.T) {
	m := New()
	if got := m.Name(); got != "about" {
		t.Fatalf("module name = %q, want %q", got, "about")
	}

	mux := http.NewServeMux()
	r := modregistry.NewRegistrar(mux, nil)
	if err := m.Register(modregistry.Dependencies{}, r); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	meta := r.Meta()
	if len(meta) != 1 || meta[0].Pattern != "GET /about" {
		t.Fatalf("unexpected route meta: %#v", meta)
	}
}
