package compliance

import (
	"net/http"
	"testing"

	"github.com/3lbits/vigil/internal/modregistry"
)

func TestModuleContract(t *testing.T) {
	m := New()
	if got := m.Name(); got != "compliance" {
		t.Fatalf("module name = %q, want %q", got, "compliance")
	}
	mux := http.NewServeMux()
	r := modregistry.NewRegistrar(mux, nil)
	if err := m.Register(modregistry.Dependencies{}, r); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if got := len(r.Meta()); got != 18 {
		t.Fatalf("expected 18 routes, got %d", got)
	}
}
