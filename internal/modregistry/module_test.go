package modregistry

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

type testModule struct {
	name  string
	err   error
	calls *[]string
}

func (m *testModule) Name() string {
	return m.name
}

func (m *testModule) Register(_ Dependencies, _ *Registrar) error {
	if m.calls != nil {
		*m.calls = append(*m.calls, m.name)
	}
	return m.err
}

type testSeedModule struct {
	testModule
	seedErr error
}

func (m *testSeedModule) DevSeed(_ context.Context, _ Dependencies) error {
	if m.calls != nil {
		*m.calls = append(*m.calls, "seed:"+m.name)
	}
	return m.seedErr
}

func TestRegistryRegisterIsIdempotentForSameInstance(t *testing.T) {
	reg := NewRegistry()
	mod := &testModule{name: "about"}

	if err := reg.Register(mod); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := reg.Register(mod); err != nil {
		t.Fatalf("second register should be idempotent: %v", err)
	}
}

func TestRegistryRegisterErrorsOnDuplicateName(t *testing.T) {
	reg := NewRegistry()
	first := &testModule{name: "about"}
	second := &testModule{name: "about"}

	if err := reg.Register(first); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := reg.Register(second); err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestRegistryMountAllRegistersInOrderAndWrapsErrors(t *testing.T) {
	reg := NewRegistry()
	calls := []string{}
	ok := &testModule{name: "about", calls: &calls}
	boom := &testModule{name: "risk", err: errors.New("boom"), calls: &calls}
	after := &testModule{name: "admin", calls: &calls}

	if err := reg.Register(ok); err != nil {
		t.Fatalf("register about failed: %v", err)
	}
	if err := reg.Register(boom); err != nil {
		t.Fatalf("register risk failed: %v", err)
	}
	if err := reg.Register(after); err != nil {
		t.Fatalf("register admin failed: %v", err)
	}

	err := reg.MountAll(http.NewServeMux(), Dependencies{})
	if err == nil {
		t.Fatal("expected mount error")
	}
	if got, want := err.Error(), `register module "risk": boom`; got != want {
		t.Fatalf("unexpected error: got %q want %q", got, want)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 calls before error, got %d", len(calls))
	}
	if calls[0] != "about" || calls[1] != "risk" {
		t.Fatalf("unexpected call order: %v", calls)
	}
}

func TestRegistryRouteMetaAggregatesModuleRegistrations(t *testing.T) {
	reg := NewRegistry()
	mod := &metaModule{name: "auth"}
	if err := reg.Register(mod); err != nil {
		t.Fatalf("register module failed: %v", err)
	}

	if err := reg.MountAll(http.NewServeMux(), Dependencies{}); err != nil {
		t.Fatalf("mount failed: %v", err)
	}

	meta := reg.RouteMeta()
	if len(meta) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(meta))
	}
	if meta[0].Pattern != "GET /protected" || meta[0].Policy == nil {
		t.Fatalf("expected protected route with policy metadata, got %+v", meta[0])
	}
	if meta[1].Pattern != "GET /public" || !meta[1].Public {
		t.Fatalf("expected public route metadata, got %+v", meta[1])
	}
}

type metaModule struct {
	name string
}

func (m *metaModule) Name() string { return m.name }

func (m *metaModule) Register(_ Dependencies, r *Registrar) error {
	r.Guarded("GET /protected", Policy{Resource: "about", Action: "read"}, func(http.ResponseWriter, *http.Request) {})
	r.Public("GET /public", func(http.ResponseWriter, *http.Request) {})
	return nil
}

func TestRegistrySeedDevRunsInOrderAndWrapsErrors(t *testing.T) {
	reg := NewRegistry()
	calls := []string{}
	nonSeeder := &testModule{name: "about", calls: &calls}
	okSeeder := &testSeedModule{
		testModule: testModule{name: "compliance", calls: &calls},
	}
	boomSeeder := &testSeedModule{
		testModule: testModule{name: "risk", calls: &calls},
		seedErr:    errors.New("boom"),
	}
	after := &testSeedModule{
		testModule: testModule{name: "admin", calls: &calls},
	}

	if err := reg.Register(nonSeeder); err != nil {
		t.Fatalf("register non-seeder failed: %v", err)
	}
	if err := reg.Register(okSeeder); err != nil {
		t.Fatalf("register compliance failed: %v", err)
	}
	if err := reg.Register(boomSeeder); err != nil {
		t.Fatalf("register risk failed: %v", err)
	}
	if err := reg.Register(after); err != nil {
		t.Fatalf("register admin failed: %v", err)
	}

	err := reg.SeedDev(context.Background(), Dependencies{})
	if err == nil {
		t.Fatal("expected seed error")
	}
	if got, want := err.Error(), `seed module "risk": boom`; got != want {
		t.Fatalf("unexpected error: got %q want %q", got, want)
	}
	if len(calls) != 2 {
		t.Fatalf("expected two seed calls before error, got %d (%v)", len(calls), calls)
	}
	if calls[0] != "seed:compliance" || calls[1] != "seed:risk" {
		t.Fatalf("unexpected seed call order: %v", calls)
	}
}
