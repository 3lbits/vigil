package modregistry

import (
	"net/http"
	"testing"
)

func TestRegistrarGuardedPanicsOnEmptyPolicy(t *testing.T) {
	r := NewRegistrar(http.NewServeMux(), nil)
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for empty policy")
		}
	}()
	r.Guarded("GET /x", Policy{Resource: "risk"}, func(http.ResponseWriter, *http.Request) {})
}

func TestRegistrarMetaTracksGuardedAndPublic(t *testing.T) {
	r := NewRegistrar(http.NewServeMux(), nil)
	r.Guarded("GET /guarded", Policy{Resource: "about", Action: "read"}, func(http.ResponseWriter, *http.Request) {})
	r.Public("GET /public", func(http.ResponseWriter, *http.Request) {})

	meta := r.Meta()
	if len(meta) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(meta))
	}
	if meta[0].Policy == nil || meta[0].Public {
		t.Fatalf("expected first route to be guarded, got %+v", meta[0])
	}
	if !meta[1].Public || meta[1].Policy != nil {
		t.Fatalf("expected second route to be public, got %+v", meta[1])
	}
}
