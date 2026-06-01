package avvik

import (
	"database/sql"

	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/modregistry"
)

type avvikModule struct {
	sqlDB *sql.DB
}

func New(sqlDB *sql.DB) modregistry.Module {
	return avvikModule{sqlDB: sqlDB}
}

func (avvikModule) Name() string {
	return "avvik"
}

// Register wires the avvik routes.
func (m avvikModule) Register(deps modregistry.Dependencies, r *modregistry.Registrar) error {
	h := NewHandler(deps.Queries, m.sqlDB, deps.Authz)
	moduleGuard := middleware.RequireModule("avvik")
	readPolicy := modregistry.Policy{Resource: "avvik", Action: "read"}
	submitPolicy := modregistry.Policy{Resource: "about", Action: "read"}
	managePolicy := modregistry.Policy{Resource: "admin", Action: "write"}

	r.Guarded("GET /avvik", readPolicy, h.List, moduleGuard)
	r.Guarded("GET /avvik/new", submitPolicy, h.New, moduleGuard)
	r.Guarded("POST /avvik", submitPolicy, h.Create, moduleGuard)
	r.Guarded("GET /avvik/{id}", readPolicy, h.Show, moduleGuard)
	r.Guarded("POST /avvik/{id}/triage", managePolicy, h.Triage, moduleGuard)
	r.Guarded("POST /avvik/{id}/status", managePolicy, h.UpdateStatus, moduleGuard)
	r.Guarded("POST /avvik/{id}/notes", submitPolicy, h.AddNote, moduleGuard)
	r.Guarded("POST /avvik/{id}/measures", managePolicy, h.LinkMeasure, moduleGuard)
	r.Guarded("DELETE /avvik/{id}/measures/{mid}", managePolicy, h.UnlinkMeasure, moduleGuard)
	r.Guarded("POST /avvik/{id}/activities", managePolicy, h.LinkActivity, moduleGuard)
	r.Guarded("DELETE /avvik/{id}/activities/{aid}", managePolicy, h.UnlinkActivity, moduleGuard)
	r.Guarded("POST /avvik/{id}/notifications", managePolicy, h.AddNotification, moduleGuard)
	r.Guarded("POST /avvik/{id}/attachments", submitPolicy, h.AddAttachment, moduleGuard)
	r.Guarded("POST /avvik/{id}/closure-flags", managePolicy, h.UpdateClosureFlags, moduleGuard)
	r.Guarded("POST /avvik/{id}/close", managePolicy, h.Close, moduleGuard)
	return nil
}
