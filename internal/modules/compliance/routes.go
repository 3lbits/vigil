package compliance

import (
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/modregistry"
)

type complianceModule struct{}

func New() modregistry.Module {
	return complianceModule{}
}

func (complianceModule) Name() string {
	return "compliance"
}

// Register wires the compliance routes onto mux.
func (complianceModule) Register(deps modregistry.Dependencies, r *modregistry.Registrar) error {
	h := NewHandler(deps.Queries, deps.Authz)
	moduleGuard := middleware.RequireModule("compliance")
	readPolicy := modregistry.Policy{Resource: "frameworks", Action: "read"}
	writePolicy := modregistry.Policy{Resource: "frameworks", Action: "write"}
	deletePolicy := modregistry.Policy{Resource: "frameworks", Action: "delete"}
	ownFrameworkWrite := RequireComplianceUpdateOwn("frameworks", deps.Queries, deps.Authz)
	ownRequirementWrite := RequireComplianceUpdateOwn("requirements", deps.Queries, deps.Authz)

	r.Guarded("GET /compliance", readPolicy, h.List, moduleGuard)

	// Framework CRUD
	r.Guarded("GET /compliance/frameworks/new", writePolicy, h.NewFramework, moduleGuard)
	r.Guarded("POST /compliance/frameworks", writePolicy, h.CreateFramework, moduleGuard)
	r.Guarded("GET /compliance/frameworks/{id}", readPolicy, h.ShowFramework, moduleGuard)
	r.Guarded("GET /compliance/frameworks/{id}/edit", writePolicy, h.EditFramework, moduleGuard, ownFrameworkWrite)
	r.Guarded("POST /compliance/frameworks/{id}", writePolicy, h.UpdateFramework, moduleGuard, ownFrameworkWrite)
	r.Guarded("POST /compliance/frameworks/{id}/delete", deletePolicy, h.DeleteFramework, moduleGuard)

	// Requirement CRUD
	r.Guarded("GET /compliance/frameworks/{id}/requirements/new", writePolicy, h.NewRequirement, moduleGuard)
	r.Guarded("POST /compliance/frameworks/{id}/requirements", writePolicy, h.CreateRequirement, moduleGuard)
	r.Guarded("GET /compliance/requirements/{id}", readPolicy, h.ShowRequirement, moduleGuard)
	r.Guarded("GET /compliance/requirements/{id}/edit", writePolicy, h.EditRequirement, moduleGuard, ownRequirementWrite)
	r.Guarded("GET /compliance/requirements/{id}/measures/search", writePolicy, h.SearchRequirementMeasures, moduleGuard, ownRequirementWrite)
	r.Guarded("POST /compliance/requirements/{id}", writePolicy, h.UpdateRequirement, moduleGuard, ownRequirementWrite)
	r.Guarded("POST /compliance/requirements/{id}/delete", deletePolicy, h.DeleteRequirement, moduleGuard)
	r.Guarded("POST /compliance/requirements/{id}/measures", writePolicy, h.LinkMeasure, moduleGuard, ownRequirementWrite)
	r.Guarded("POST /compliance/requirements/{id}/measures/{measure_id}/delete", writePolicy, h.UnlinkMeasure, moduleGuard, ownRequirementWrite)

	// CSV import
	r.Guarded("GET /compliance/frameworks/{id}/requirements/import", writePolicy, h.ShowImportForm, moduleGuard)
	r.Guarded("POST /compliance/frameworks/{id}/requirements/import", writePolicy, h.Import, moduleGuard)
	return nil
}
