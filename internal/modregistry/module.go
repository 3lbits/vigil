// Package module defines the contract Vigil modules implement and the
// Registry used by cmd/server to compose them.
//
// Each module under modules/* provides a constructor returning Module
// and is registered in cmd/server/main.go before Registry.MountAll
// mounts all routes on the shared mux.
//
// Module-specific dependencies that aren't shared across modules
// (e.g. admin's start time and version, auth's provider config) are
// passed to the module constructor, not added to Dependencies. The
// shared Dependencies struct is kept deliberately minimal — every
// field added forces every module to deal with it.
package modregistry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
)

// Module is the explicit contract for feature modules.
type Module interface {
	Name() string
	Register(deps Dependencies, registrar *Registrar) error
}

// DevSeeder is an optional module interface for development data seeding.
type DevSeeder interface {
	DevSeed(ctx context.Context, deps Dependencies) error
}

// Dependencies contains shared module dependencies.
type Dependencies struct {
	Queries *db.Queries
	Authz   *authz.Engine
}

// Registry stores modules and mounts them in registration order.
type Registry struct {
	byName map[string]Module
	order  []string
	meta   []RouteMeta
}

var (
	ErrNilModule           = errors.New("module is nil")
	ErrEmptyModuleName     = errors.New("module name is empty")
	ErrDuplicateModuleName = errors.New("module name already registered")
	ErrNilMux              = errors.New("mux is nil")
)

func NewRegistry() *Registry {
	return &Registry{
		byName: make(map[string]Module),
		meta:   make([]RouteMeta, 0),
	}
}

func (r *Registry) Register(m Module) error {
	if m == nil {
		return ErrNilModule
	}
	name := m.Name()
	if name == "" {
		return ErrEmptyModuleName
	}
	existing, ok := r.byName[name]
	if ok {
		if sameModule(existing, m) {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrDuplicateModuleName, name)
	}
	r.byName[name] = m
	r.order = append(r.order, name)
	return nil
}

func (r *Registry) MountAll(mux *http.ServeMux, deps Dependencies) error {
	if mux == nil {
		return ErrNilMux
	}
	r.meta = r.meta[:0]
	for _, name := range r.order {
		m := r.byName[name]
		registrar := NewRegistrar(mux, deps.Authz)
		if err := m.Register(deps, registrar); err != nil {
			return fmt.Errorf("register module %q: %w", name, err)
		}
		r.meta = append(r.meta, registrar.Meta()...)
	}
	return nil
}

func (r *Registry) RouteMeta() []RouteMeta {
	return append([]RouteMeta{}, r.meta...)
}

func (r *Registry) SeedDev(ctx context.Context, deps Dependencies) error {
	for _, name := range r.order {
		m := r.byName[name]
		seeder, ok := m.(DevSeeder)
		if !ok {
			continue
		}
		if err := seeder.DevSeed(ctx, deps); err != nil {
			return fmt.Errorf("seed module %q: %w", name, err)
		}
	}
	return nil
}

func sameModule(a, b Module) bool {
	av := reflect.ValueOf(a)
	bv := reflect.ValueOf(b)
	if !av.IsValid() || !bv.IsValid() {
		return false
	}
	if !av.Comparable() || !bv.Comparable() {
		return false
	}
	if av.Type() != bv.Type() {
		return false
	}
	return av.Interface() == bv.Interface()
}
