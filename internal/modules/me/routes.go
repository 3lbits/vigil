package me

import "github.com/3lbits/vigil/internal/modregistry"

type meModule struct{}

func New() modregistry.Module {
	return meModule{}
}

func (meModule) Name() string {
	return "me"
}

func (meModule) Register(deps modregistry.Dependencies, r *modregistry.Registrar) error {
	h := NewHandler(deps.Queries)
	r.Guarded("GET /me", modregistry.Policy{Resource: "me", Action: "read"}, h.Show)
	return nil
}
