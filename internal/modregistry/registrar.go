package modregistry

import (
	"fmt"
	"net/http"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/middleware"
)

// Middleware is a route middleware function.
type Middleware func(http.Handler) http.Handler

type Policy struct {
	Resource string
	Action   string
}

type RouteMeta struct {
	Pattern string
	Policy  *Policy
	Public  bool
}

type Registrar struct {
	mux   *http.ServeMux
	authz *authz.Engine
	base  []Middleware
	meta  []RouteMeta
}

func NewRegistrar(mux *http.ServeMux, engine *authz.Engine, base ...Middleware) *Registrar {
	return &Registrar{
		mux:   mux,
		authz: engine,
		base:  append([]Middleware{}, base...),
		meta:  make([]RouteMeta, 0),
	}
}

// Guarded always applies policy middleware to the route.
func (r *Registrar) Guarded(pattern string, p Policy, h http.HandlerFunc, extra ...Middleware) {
	if p.Resource == "" || p.Action == "" {
		panic(fmt.Sprintf("modregistry: %q registered without resource/action", pattern))
	}
	mws := append(append([]Middleware{}, r.base...), authz.RequirePolicy(r.authz, p.Resource, p.Action))
	mws = append(mws, extra...)
	r.mux.Handle(pattern, middleware.Chain(h, toMiddleware(mws)...))
	policy := p
	r.meta = append(r.meta, RouteMeta{
		Pattern: pattern,
		Policy:  &policy,
	})
}

// Public intentionally omits policy middleware.
func (r *Registrar) Public(pattern string, h http.HandlerFunc, extra ...Middleware) {
	mws := append(append([]Middleware{}, r.base...), extra...)
	r.mux.Handle(pattern, middleware.Chain(h, toMiddleware(mws)...))
	r.meta = append(r.meta, RouteMeta{
		Pattern: pattern,
		Public:  true,
	})
}

func (r *Registrar) Meta() []RouteMeta {
	return append([]RouteMeta{}, r.meta...)
}

func toMiddleware(in []Middleware) []func(http.Handler) http.Handler {
	out := make([]func(http.Handler) http.Handler, 0, len(in))
	for _, mw := range in {
		out = append(out, mw)
	}
	return out
}
