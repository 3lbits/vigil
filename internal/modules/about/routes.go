package about

import "github.com/3lbits/vigil/internal/modregistry"

type aboutModule struct{}

func New() modregistry.Module {
	return aboutModule{}
}

func (aboutModule) Name() string {
	return "about"
}

func (aboutModule) Register(_ modregistry.Dependencies, r *modregistry.Registrar) error {
	h := NewHandler()
	r.Guarded("GET /about", modregistry.Policy{Resource: "about", Action: "read"}, h.Show)
	return nil
}
