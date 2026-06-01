package measures

import "github.com/3lbits/vigil/internal/modregistry"

type measuresModule struct{}

func New() modregistry.Module {
	return measuresModule{}
}

func (measuresModule) Name() string {
	return "measures"
}

// Register wires the measures routes onto mux.
func (measuresModule) Register(deps modregistry.Dependencies, r *modregistry.Registrar) error {
	h := NewHandler(deps.Queries, deps.Authz)
	readPolicy := modregistry.Policy{Resource: "measures", Action: "read"}
	writePolicy := modregistry.Policy{Resource: "measures", Action: "write"}
	deletePolicy := modregistry.Policy{Resource: "measures", Action: "delete"}

	r.Guarded("GET /measures", readPolicy, h.List)
	r.Guarded("GET /measures/new", writePolicy, h.New)
	r.Guarded("GET /measures/{id}", readPolicy, h.Show)
	r.Guarded("POST /measures", writePolicy, h.Create)
	r.Guarded("GET /measures/{id}/edit", writePolicy, h.Edit, RequireMeasureUpdateOwn(deps.Queries, deps.Authz))
	r.Guarded("GET /measures/{id}/requirements/search", writePolicy, h.SearchMeasureRequirements, RequireMeasureUpdateOwn(deps.Queries, deps.Authz))
	r.Guarded("POST /measures/{id}", writePolicy, h.Update, RequireMeasureUpdateOwn(deps.Queries, deps.Authz))
	r.Guarded("POST /measures/{id}/delete", deletePolicy, h.Delete)
	r.Guarded("POST /measures/{id}/requirements", writePolicy, h.LinkRequirement, RequireMeasureUpdateOwn(deps.Queries, deps.Authz))
	r.Guarded("POST /measures/{id}/requirements/{req_id}/delete", writePolicy, h.UnlinkRequirement, RequireMeasureUpdateOwn(deps.Queries, deps.Authz))
	r.Guarded("POST /measures/{id}/links", writePolicy, h.AddLink, RequireMeasureUpdateOwn(deps.Queries, deps.Authz))
	r.Guarded("POST /measures/{id}/links/{link_id}/delete", writePolicy, h.DeleteLink, RequireMeasureUpdateOwn(deps.Queries, deps.Authz))
	return nil
}
