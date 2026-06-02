// Package admin provides system administration endpoints.
package admin

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/audit"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/middleware"
	admintemplates "github.com/3lbits/vigil/internal/modules/admin/templates"
	"github.com/3lbits/vigil/internal/ui/layout"
)

type dbPinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	q                  db.Querier
	pool               dbPinger
	startTime          time.Time
	version            string
	refreshModuleFlags func(context.Context) error
}

func NewHandler(
	q db.Querier,
	pool dbPinger,
	startTime time.Time,
	version string,
	refreshModuleFlags func(context.Context) error,
) *Handler {
	return &Handler{
		q:                  q,
		pool:               pool,
		startTime:          startTime,
		version:            version,
		refreshModuleFlags: refreshModuleFlags,
	}
}

func (h *Handler) refreshModuleFlagsCache(ctx context.Context) {
	if h.refreshModuleFlags == nil {
		return
	}
	if err := h.refreshModuleFlags(ctx); err != nil {
		slog.Warn("refresh module flags cache", "error", err)
	}
}

func (h *Handler) AdminPage(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.FromContext(r.Context())
	flash := r.URL.Query().Get("flash")
	flashType := r.URL.Query().Get("type")
	activeTab := r.URL.Query().Get("tab")
	auditFilter := r.URL.Query().Get("audit")
	validTabs := map[string]bool{
		"users": true, "sessions": true, "audit": true, "status": true,
		"orgs": true, "risk": true, "modules": true,
	}
	if !validTabs[activeTab] {
		activeTab = "users"
	}
	if auditFilter != "delete" && auditFilter != "update" {
		auditFilter = "all"
	}

	users, err := h.q.ListUsers(r.Context())
	if err != nil {
		slog.Error("list users", "error", err)
		http.Error(w, "failed to load users", http.StatusInternalServerError)
		return
	}

	sessions, err := h.q.ListActiveSessionsByUser(r.Context())
	if err != nil {
		slog.Error("list sessions", "error", err)
		http.Error(w, "failed to load sessions", http.StatusInternalServerError)
		return
	}

	auditLog, err := h.q.ListAuditLogAdmin(r.Context())
	if err != nil {
		slog.Warn("list audit log admin", "error", err)
	}
	auditLog = filterAuditLog(auditLog, auditFilter)

	orgs, _ := h.q.ListOrganizations(r.Context())
	gs, _ := h.q.GetRiskGlobalSettings(r.Context())
	scaleLabels, _ := h.q.ListRiskScaleLabels(r.Context())
	appSettings, _ := h.q.GetAppSettings(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	dbOK := h.pool.Ping(ctx) == nil

	httputil.Render(w, r, layout.Layout("Admin", "System administration", "admin", user,
		admintemplates.AdminPage(users, sessions, auditLog, h.version, h.startTime, dbOK, flash, flashType, activeTab, auditFilter, orgs, gs, scaleLabels, appSettings),
	))
}

func filterAuditLog(rows []db.ListAuditLogAdminRow, filter string) []db.ListAuditLogAdminRow {
	if filter == "all" {
		return rows
	}
	filtered := make([]db.ListAuditLogAdminRow, 0, len(rows))
	for _, row := range rows {
		if strings.HasSuffix(row.Event, filter) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func (h *Handler) SetRole(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	role := r.FormValue("role")
	if role != "admin" && role != "editor" && role != "viewer" && role != "contributor" {
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}
	if _, err := h.q.SetUserRole(r.Context(), db.SetUserRoleParams{ID: id, Role: role}); err != nil {
		slog.Error("set user role", "error", err)
		http.Redirect(w, r, "/admin?tab=users&flash=Failed+to+update+role.&type=error", http.StatusSeeOther)
		return
	}
	if err := audit.Record(r.Context(), h.q, audit.Event{
		Event: "admin.role_change",
		Attrs: map[string]any{"target_user_id": id.String(), "new_role": role},
	}); err != nil {
		slog.Error("audit role change", "error", err)
	}
	http.Redirect(w, r, "/admin?tab=users&flash=Role+updated.&type=success", http.StatusSeeOther)
}

func (h *Handler) SetUserOrg(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	orgStr := strings.TrimSpace(r.FormValue("org_id"))
	var orgID uuid.NullUUID
	if orgStr != "" {
		parsed, err := uuid.Parse(orgStr)
		if err != nil {
			http.Error(w, "invalid org_id", http.StatusBadRequest)
			return
		}
		orgID = uuid.NullUUID{UUID: parsed, Valid: true}
	}
	if _, err := h.q.SetUserOrg(r.Context(), db.SetUserOrgParams{ID: id, OrgID: orgID}); err != nil {
		slog.Error("set user org", "error", err)
		http.Redirect(w, r, "/admin?tab=users&flash=Failed+to+update+org.&type=error", http.StatusSeeOther)
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "admin.user.org_assigned",
		Attrs: map[string]any{"target_user_id": id.String(), "org_id": orgStr},
	})
	http.Redirect(w, r, "/admin?tab=users&flash=Org+updated.&type=success", http.StatusSeeOther)
}

func (h *Handler) RevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.q.DeleteSessionsByUserID(r.Context(), uuid.NullUUID{UUID: id, Valid: true}); err != nil {
		slog.Error("revoke user sessions", "error", err)
		http.Redirect(w, r, "/admin?tab=sessions&flash=Failed+to+revoke+sessions.&type=error", http.StatusSeeOther)
		return
	}
	if err := audit.Record(r.Context(), h.q, audit.Event{
		Event: "admin.sessions_revoked",
		Attrs: map[string]any{"target_user_id": id.String()},
	}); err != nil {
		slog.Error("audit sessions revoke", "error", err)
	}
	http.Redirect(w, r, "/admin?tab=sessions&flash=Sessions+revoked.&type=success", http.StatusSeeOther)
}

func (h *Handler) PreCreateUser(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	name := strings.TrimSpace(r.FormValue("name"))
	role := strings.TrimSpace(r.FormValue("role"))
	if email == "" {
		http.Redirect(w, r, "/admin?tab=users&flash=Email+required.&type=error", http.StatusSeeOther)
		return
	}
	if role != "admin" && role != "editor" && role != "viewer" && role != "contributor" {
		http.Redirect(w, r, "/admin?tab=users&flash=Invalid+role.&type=error", http.StatusSeeOther)
		return
	}
	// If the user already exists with a real provider, just update their role.
	existing, err := h.q.GetUserByEmail(r.Context(), email)
	if err == nil && existing.Provider != "pending" {
		if _, err := h.q.SetUserRole(r.Context(), db.SetUserRoleParams{ID: existing.ID, Role: role}); err != nil {
			slog.Error("pre-create: set role on existing user", "error", err)
			http.Redirect(w, r, "/admin?tab=users&flash=Failed+to+update+role.&type=error", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/admin?tab=users&flash=User+role+updated.&type=success", http.StatusSeeOther)
		return
	}
	if _, err := h.q.PreCreateUser(r.Context(), db.PreCreateUserParams{
		ProviderID: email,
		Name:       name,
		Role:       role,
	}); err != nil {
		slog.Error("pre-create user", "error", err)
		http.Redirect(w, r, "/admin?tab=users&flash=Failed+to+add+user.&type=error", http.StatusSeeOther)
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "admin.user_pre_created",
		Attrs: map[string]any{"email": email, "name": name, "role": role},
	})
	http.Redirect(w, r, "/admin?tab=users&flash=User+added.+They+can+now+sign+in+via+SSO.&type=success", http.StatusSeeOther)
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	actor, _ := middleware.FromContext(r.Context())
	if actor.ID == id.String() {
		http.Redirect(w, r, "/admin?tab=users&flash=Cannot+delete+your+own+account.&type=error", http.StatusSeeOther)
		return
	}
	if err := h.q.DeleteUser(r.Context(), id); err != nil {
		slog.Error("delete user", "error", err)
		http.Redirect(w, r, "/admin?tab=users&flash=Failed+to+delete+user.&type=error", http.StatusSeeOther)
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "admin.user_deleted",
		Attrs: map[string]any{"target_user_id": id.String()},
	})
	http.Redirect(w, r, "/admin?tab=users&flash=User+deleted.&type=success", http.StatusSeeOther)
}

// ── Organisations ─────────────────────────────────────────────────────────────

func (h *Handler) OrgsPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin?tab=orgs", http.StatusSeeOther)
}

func (h *Handler) CreateOrg(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	key := strings.TrimSpace(r.FormValue("key"))
	if name == "" {
		http.Redirect(w, r, "/admin/orgs?flash=Name+required&type=error", http.StatusSeeOther)
		return
	}
	var parentID uuid.NullUUID
	if p := strings.TrimSpace(r.FormValue("parent_id")); p != "" {
		if pid, err := uuid.Parse(p); err == nil {
			parentID = uuid.NullUUID{UUID: pid, Valid: true}
		}
	}
	org, err := h.q.CreateOrganization(r.Context(), db.CreateOrganizationParams{
		Name:     name,
		ParentID: parentID,
		Key:      sql.NullString{String: key, Valid: key != ""},
	})
	if err != nil {
		slog.Error("create organization", "error", err)
		http.Redirect(w, r, "/admin?tab=orgs&flash=Failed+to+create&type=error", http.StatusSeeOther)
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "admin.organization.create",
		Attrs: map[string]any{"organization_id": org.ID.String(), "name": name},
	})
	h.refreshModuleFlagsCache(r.Context())
	http.Redirect(w, r, "/admin?tab=orgs&flash=Organisation+added.&type=success", http.StatusSeeOther)
}

func (h *Handler) DeleteOrg(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.q.DeleteOrganization(r.Context(), id); err != nil {
		slog.Error("delete organization", "error", err)
		http.Redirect(w, r, "/admin?tab=orgs&flash=Failed+to+delete&type=error", http.StatusSeeOther)
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "admin.organization.delete",
		Attrs: map[string]any{"organization_id": id.String()},
	})
	h.refreshModuleFlagsCache(r.Context())
	http.Redirect(w, r, "/admin?tab=orgs&flash=Organisation+deleted.&type=success", http.StatusSeeOther)
}

// ── Risk settings ─────────────────────────────────────────────────────────────

func (h *Handler) RiskSettingsPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin?tab=risk", http.StatusSeeOther)
}

func (h *Handler) SaveRiskSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	criteria := strings.TrimSpace(r.FormValue("acceptance_criteria"))
	lowMax := parseInt32(r.FormValue("low_max"), 5)
	highMin := parseInt32(r.FormValue("high_min"), 12)
	if err := h.q.UpdateRiskGlobalSettings(r.Context(), db.UpdateRiskGlobalSettingsParams{
		AcceptanceCriteria: criteria,
		LowMax:             lowMax,
		HighMin:            highMin,
	}); err != nil {
		slog.Error("update risk global settings", "error", err)
		http.Redirect(w, r, "/admin?tab=risk&flash=Save+failed&type=error", http.StatusSeeOther)
		return
	}
	h.upsertRiskScaleLabels(r)
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "admin.risk_settings.update",
		Attrs: map[string]any{"low_max": lowMax, "high_min": highMin},
	})
	http.Redirect(w, r, "/admin?tab=risk&flash=Saved.&type=success", http.StatusSeeOther)
}

func (h *Handler) SaveModuleSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	params := db.UpdateAppSettingsParams{
		ComplianceEnabled: r.FormValue("compliance_enabled") == "on",
		RiskEnabled:       r.FormValue("risk_enabled") == "on",
		ActivitiesEnabled: r.FormValue("activities_enabled") == "on",
		AssetsEnabled:     r.FormValue("assets_enabled") == "on",
		AvvikEnabled:      r.FormValue("avvik_enabled") == "on",
	}
	current, err := h.q.GetAppSettings(r.Context())
	if err != nil {
		slog.Error("load app settings", "error", err)
		http.Redirect(w, r, "/admin?tab=modules&flash=Save+failed.&type=error", http.StatusSeeOther)
		return
	}
	params.PlaygroundEnabled = current.PlaygroundEnabled
	if err := h.q.UpdateAppSettings(r.Context(), params); err != nil {
		slog.Error("update app settings", "error", err)
		http.Redirect(w, r, "/admin?tab=modules&flash=Save+failed.&type=error", http.StatusSeeOther)
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "admin.module_settings.update",
		Attrs: map[string]any{
			"compliance_enabled": params.ComplianceEnabled,
			"risk_enabled":       params.RiskEnabled,
			"activities_enabled": params.ActivitiesEnabled,
			"assets_enabled":     params.AssetsEnabled,
			"avvik_enabled":      params.AvvikEnabled,
		},
	})
	h.refreshModuleFlagsCache(r.Context())
	http.Redirect(w, r, "/admin?tab=modules&flash=Saved.&type=success", http.StatusSeeOther)
}

func parseInt32(s string, def int32) int32 {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n < 1 {
		return def
	}
	return int32(n)
}

func (h *Handler) upsertRiskScaleLabels(r *http.Request) {
	for i := int32(1); i <= 5; i++ {
		h.upsertProbabilityScaleLabel(r, i)
		h.upsertConsequenceScaleLabel(r, i)
	}
}

func (h *Handler) upsertProbabilityScaleLabel(r *http.Request, level int32) {
	label := strings.TrimSpace(r.FormValue(fmt.Sprintf("prob_label_%d", level)))
	desc := strings.TrimSpace(r.FormValue(fmt.Sprintf("prob_desc_%d", level)))
	if label == "" && desc == "" {
		return
	}
	if err := h.q.UpsertRiskScaleLabel(r.Context(), db.UpsertRiskScaleLabelParams{
		Scale:       "probability",
		Level:       level,
		Label:       label,
		Description: desc,
	}); err != nil {
		slog.Warn("upsert scale label", "scale", "probability", "level", level, "error", err)
	}
}

func (h *Handler) upsertConsequenceScaleLabel(r *http.Request, level int32) {
	label := strings.TrimSpace(r.FormValue(fmt.Sprintf("cons_label_%d", level)))
	desc := formatConsequenceTopics(
		strings.TrimSpace(r.FormValue(fmt.Sprintf("cons_finops_desc_%d", level))),
		strings.TrimSpace(r.FormValue(fmt.Sprintf("cons_confdata_desc_%d", level))),
		strings.TrimSpace(r.FormValue(fmt.Sprintf("cons_reglegal_desc_%d", level))),
		strings.TrimSpace(r.FormValue(fmt.Sprintf("cons_reptrust_desc_%d", level))),
	)
	// Backward compatibility if old field is still posted by stale pages.
	if desc == "" {
		desc = strings.TrimSpace(r.FormValue(fmt.Sprintf("cons_desc_%d", level)))
	}
	if label == "" && desc == "" {
		return
	}
	if err := h.q.UpsertRiskScaleLabel(r.Context(), db.UpsertRiskScaleLabelParams{
		Scale:       "consequence",
		Level:       level,
		Label:       label,
		Description: desc,
	}); err != nil {
		slog.Warn("upsert scale label", "scale", "consequence", "level", level, "error", err)
	}
}

func formatConsequenceTopics(finOps, confData, regLegal, repTrust string) string {
	lines := make([]string, 0, 4)
	if finOps != "" {
		lines = append(lines, "Financial / Operational: "+finOps)
	}
	if confData != "" {
		lines = append(lines, "Confidentiality / Data: "+confData)
	}
	if regLegal != "" {
		lines = append(lines, "Regulatory / Legal: "+regLegal)
	}
	if repTrust != "" {
		lines = append(lines, "Reputation / Trust: "+repTrust)
	}
	return strings.Join(lines, "\n")
}
