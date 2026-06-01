package measures

import (
	"net/http"
	"testing"

	"github.com/3lbits/vigil/internal/modregistry"
)

func TestModuleContract(t *testing.T) {
	m := New()
	if got := m.Name(); got != "measures" {
		t.Fatalf("module name = %q, want %q", got, "measures")
	}
	mux := http.NewServeMux()
	r := modregistry.NewRegistrar(mux, nil)
	if err := m.Register(modregistry.Dependencies{}, r); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if got := len(r.Meta()); got != 12 {
		t.Fatalf("expected 12 routes, got %d", got)
	}
}
