package avvik

import (
	"net/http"
	"testing"

	"github.com/3lbits/vigil/internal/modregistry"
)

func TestModuleContract(t *testing.T) {
	m := New(nil)
	if got := m.Name(); got != "avvik" {
		t.Fatalf("module name = %q, want %q", got, "avvik")
	}

	mux := http.NewServeMux()
	r := modregistry.NewRegistrar(mux, nil)
	if err := m.Register(modregistry.Dependencies{}, r); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if got := len(r.Meta()); got != 15 {
		t.Fatalf("expected 15 routes, got %d", got)
	}
}
