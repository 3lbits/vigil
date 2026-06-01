package activities

import (
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/modregistry"
)

type activitiesModule struct{}

func New() modregistry.Module {
	return activitiesModule{}
}

func (activitiesModule) Name() string {
	return "activities"
}

// Register wires the activities routes onto mux.
func (activitiesModule) Register(deps modregistry.Dependencies, r *modregistry.Registrar) error {
	h := NewHandler(deps.Queries, deps.Authz)
	moduleGuard := middleware.RequireModule("activities")
	readPolicy := modregistry.Policy{Resource: "activities", Action: "read"}
	writePolicy := modregistry.Policy{Resource: "activities", Action: "write"}
	deletePolicy := modregistry.Policy{Resource: "activities", Action: "delete"}
	ownWriteGuard := RequireActivityUpdateOwn(deps.Queries, deps.Authz)

	r.Guarded("GET /activities", readPolicy, h.List, moduleGuard)
	r.Guarded("GET /activities/new", writePolicy, h.New, moduleGuard)
	r.Guarded("GET /activities/{id}", readPolicy, h.Show, moduleGuard)
	r.Guarded("POST /activities", writePolicy, h.Create, moduleGuard)
	r.Guarded("GET /activities/{id}/edit", writePolicy, h.Edit, moduleGuard, ownWriteGuard)
	r.Guarded("POST /activities/{id}", writePolicy, h.Update, moduleGuard, ownWriteGuard)
	r.Guarded("POST /activities/{id}/complete", writePolicy, h.Complete, moduleGuard, ownWriteGuard)
	r.Guarded("POST /activities/{id}/reopen", writePolicy, h.Reopen, moduleGuard, ownWriteGuard)
	r.Guarded("POST /activities/{id}/start", writePolicy, h.StartProgress, moduleGuard, ownWriteGuard)
	r.Guarded("POST /activities/{id}/delete", deletePolicy, h.Delete, moduleGuard)
	return nil
}
