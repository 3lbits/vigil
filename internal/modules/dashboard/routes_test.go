package dashboard

import (
	"net/http"
	"testing"

	"github.com/3lbits/vigil/internal/modregistry"
)

func TestModuleContract(t *testing.T) {
	m := New()
	if got := m.Name(); got != "dashboard" {
		t.Fatalf("module name = %q, want %q", got, "dashboard")
	}
	mux := http.NewServeMux()
	r := modregistry.NewRegistrar(mux, nil)
	if err := m.Register(modregistry.Dependencies{}, r); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	meta := r.Meta()
	if len(meta) != 1 || meta[0].Pattern != "GET /{$}" {
		t.Fatalf("unexpected route meta: %#v", meta)
	}
}
