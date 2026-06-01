package assets

import (
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/modregistry"
)

type assetsModule struct{}

func New() modregistry.Module {
	return assetsModule{}
}

func (assetsModule) Name() string {
	return "assets"
}

// Register wires the assets routes onto mux.
func (assetsModule) Register(deps modregistry.Dependencies, r *modregistry.Registrar) error {
	h := NewHandler(deps.Queries, deps.Authz)
	moduleGuard := middleware.RequireModule("assets")
	readPolicy := modregistry.Policy{Resource: "assets", Action: "read"}
	writePolicy := modregistry.Policy{Resource: "assets", Action: "write"}
	deletePolicy := modregistry.Policy{Resource: "assets", Action: "delete"}

	r.Guarded("GET /assets", readPolicy, h.List, moduleGuard)
	r.Guarded("GET /assets/new", writePolicy, h.New, moduleGuard)
	r.Guarded("POST /assets", writePolicy, h.Create, moduleGuard)
	r.Guarded("GET /assets/{id}", readPolicy, h.Show, moduleGuard)
	r.Guarded("GET /assets/{id}/edit", writePolicy, h.Edit, moduleGuard)
	r.Guarded("POST /assets/{id}", writePolicy, h.Update, moduleGuard)
	r.Guarded("POST /assets/{id}/delete", deletePolicy, h.Delete, moduleGuard)
	return nil
}
