package dashboard

import "github.com/3lbits/vigil/internal/modregistry"

type dashboardModule struct{}

func New() modregistry.Module {
	return dashboardModule{}
}

func (dashboardModule) Name() string {
	return "dashboard"
}

// Register wires the dashboard routes onto mux.
func (dashboardModule) Register(deps modregistry.Dependencies, r *modregistry.Registrar) error {
	h := NewHandler(deps.Queries)
	r.Guarded("GET /{$}", modregistry.Policy{Resource: "dashboard", Action: "read"}, h.Dashboard)
	return nil
}
