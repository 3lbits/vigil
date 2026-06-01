// Package measures handles security controls and their lifecycle.
package measures

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/3lbits/vigil/internal/audit"
	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/middleware"
	measurestemplates "github.com/3lbits/vigil/internal/modules/measures/templates"
	"github.com/3lbits/vigil/internal/ui/layout"
	"github.com/google/uuid"
)

type Handler struct {
	q      db.Querier
	engine *authz.Engine
}

func NewHandler(q db.Querier, engine *authz.Engine) *Handler {
	return &Handler{q: q, engine: engine}
}

const measuresPageSize int32 = 50

// buildFilteredMeasureVMs fetches filtered measures and joins framework short names.
func (h *Handler) buildFilteredMeasureVMs(ctx context.Context, status, owner string, mine bool, assigneeID uuid.UUID, limit, offset int32) ([]measurestemplates.MeasureVM, error) {
	measures, err := h.q.FilterMeasures(ctx, db.FilterMeasuresParams{
		Status:     status,
		Owner:      owner,
		Mine:       mine,
		AssigneeID: assigneeID,
		PageSize:   limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("filter measures: %w", err)
	}

	links, err := h.q.ListMeasureFrameworkLinks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list measure framework links: %w", err)
	}

	riskLinkIDs, err := h.q.ListMeasureRiskLinkIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list measure risk link ids: %w", err)
	}

	fwByMeasure := make(map[uuid.UUID][]string)
	for _, l := range links {
		fwByMeasure[l.MeasureID] = append(fwByMeasure[l.MeasureID], l.FrameworkShortName)
	}

	riskLinked := make(map[uuid.UUID]bool, len(riskLinkIDs))
	for _, id := range riskLinkIDs {
		riskLinked[id] = true
	}

	vms := make([]measurestemplates.MeasureVM, 0, len(measures))
	for _, m := range measures {
		vms = append(vms, measurestemplates.MeasureVM{
			Measure:      m,
			Frameworks:   fwByMeasure[m.ID],
			HasRiskLinks: riskLinked[m.ID],
		})
	}
	return vms, nil
}

// ── Handlers ──

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := q.Get("filter")
	owner := q.Get("owner")
	mine := q.Get("mine") == "on"
	flash := q.Get("flash")
	flashType := q.Get("type")

	var offset int32
	if n, err := strconv.ParseInt(q.Get("offset"), 10, 32); err == nil && n > 0 {
		offset = int32(n)
	}

	user, _ := middleware.FromContext(r.Context())

	var assigneeID uuid.UUID
	if mine {
		assigneeID, _ = uuid.Parse(user.ID)
	}

	vms, err := h.buildFilteredMeasureVMs(r.Context(), filter, owner, mine, assigneeID, measuresPageSize+1, offset)
	if err != nil {
		slog.Error("list measures", "error", err)
		http.Error(w, "failed to load measures", http.StatusInternalServerError)
		return
	}

	hasMore := len(vms) > int(measuresPageSize)
	if hasMore {
		vms = vms[:measuresPageSize]
	}

	isHTMX := r.Header.Get("HX-Request") == "true"
	if isHTMX {
		w.Header().Set("HX-Title", "Measures")
	}

	if isHTMX && offset > 0 {
		httputil.Render(w, r, measurestemplates.MeasureRows(vms, hasMore, offset+measuresPageSize, filter, owner, mine))
		return
	}

	if isHTMX {
		httputil.Render(w, r, measurestemplates.MeasuresTable(vms, hasMore, measuresPageSize, filter, owner, mine))
		return
	}

	httputil.Render(w, r, layout.Layout("Measures", "Security controls and measures", "measures", user,
		measurestemplates.MeasureList(vms, filter, owner, mine, flash, flashType, hasMore, measuresPageSize),
	))
}

func (h *Handler) New(w http.ResponseWriter, r *http.Request) {
	users, err := h.q.ListUsers(r.Context())
	if err != nil {
		slog.Warn("list users for new measure", "error", err)
	}
	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("Add measure", "New security measure", "measures", user,
		measurestemplates.MeasureForm(nil, users, "", ""),
	))
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	status := r.FormValue("status")
	if status == "" {
		status = "planned"
	}
	owner, assigneeID := parseMeasureAssignee(r, h.q)
	created, err := h.q.CreateMeasure(r.Context(), db.CreateMeasureParams{
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Category:    strings.TrimSpace(r.FormValue("category")),
		Owner:       owner,
		AssigneeID:  assigneeID,
		Status:      status,
	})
	if err != nil {
		slog.Error("create measure", "error", err)
		users, userErr := h.q.ListUsers(r.Context())
		if userErr != nil {
			slog.Warn("list users for create measure error", "error", userErr)
		}
		user, _ := middleware.FromContext(r.Context())
		httputil.Render(w, r, layout.Layout("Add measure", "New security measure", "measures", user,
			measurestemplates.MeasureForm(nil, users, "Failed to create measure.", "error"),
		))
		return
	}

	if d := strings.TrimSpace(r.FormValue("due_date")); d != "" {
		if t, parseErr := time.Parse("2006-01-02", d); parseErr == nil {
			_, actErr := h.q.CreateActivity(r.Context(), db.CreateActivityParams{
				MeasureID:    uuid.NullUUID{UUID: created.ID, Valid: true},
				Title:        "Implement: " + created.Name,
				ActivityType: "one_off",
				Recurrence:   "none",
				Priority:     "medium",
				Kind:         "task",
				Owner:        created.Owner,
				DueDate:      sql.NullTime{Time: t, Valid: true},
			})
			if actErr != nil {
				slog.Warn("auto-create activity for measure", "error", actErr)
			}
		}
	}

	createUser, _ := middleware.FromContext(r.Context())
	if uid, parseErr := uuid.Parse(createUser.ID); parseErr == nil {
		if addErr := h.q.AddParticipant(r.Context(), db.AddParticipantParams{
			ResourceType: "measures",
			ResourceID:   created.ID,
			UserID:       uid,
			Role:         "owner",
		}); addErr != nil {
			slog.Warn("add participant on create measure", "error", addErr)
		}
	}

	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "measures.measure.create",
		Attrs: map[string]any{"measure_id": created.ID.String(), "name": strings.TrimSpace(r.FormValue("name"))},
	})
	httputil.RedirectWithFlash(w, r, "/measures", "Measure created.", "success")
}

func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	m, err := h.q.GetMeasure(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	linkedReqs, err := h.q.ListRequirementsForMeasure(r.Context(), id)
	if err != nil {
		slog.Warn("list requirements for measure", "measure_id", id.String(), "error", err) // #nosec G706
	}
	links, err := h.q.ListMeasureLinks(r.Context(), id)
	if err != nil {
		slog.Warn("list measure links", "measure_id", id.String(), "error", err) // #nosec G706
	}
	activities, err := h.q.ListActivitiesForMeasure(r.Context(), uuid.NullUUID{UUID: id, Valid: true})
	if err != nil {
		slog.Warn("list activities for measure", "measure_id", id.String(), "error", err) // #nosec G706
	}
	auditLog, err := h.q.ListAuditLogForMeasure(r.Context(), id.String())
	if err != nil {
		slog.Warn("list audit log for measure", "measure_id", id.String(), "error", err) // #nosec G706
	}
	linkedRisks, err := h.q.ListRisksForMeasure(r.Context(), id)
	if err != nil {
		slog.Warn("list risks for measure", "measure_id", id.String(), "error", err) // #nosec G706
	}
	user, _ := middleware.FromContext(r.Context())
	canEdit, err := h.engine.Allow(r.Context(), user.ID, user.Role, "measures", "write")
	if err != nil {
		slog.Error("authz eval", "error", err)
	}
	vm := measurestemplates.MeasureDetailVM{
		Measure:     m,
		LinkedReqs:  linkedReqs,
		Activities:  activities,
		Links:       links,
		LinkedRisks: linkedRisks,
		CanEdit:     canEdit,
		AuditLog:    auditLog,
	}
	measureTitle := m.Name
	if m.RefNum.Valid {
		measureTitle = fmt.Sprintf("M-%03d · %s", m.RefNum.Int32, m.Name)
	}
	httputil.Render(w, r, layout.Layout(measureTitle, m.Category, "measures", user,
		measurestemplates.MeasureDetail(vm),
	))
}

func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	m, ok := measureFromContext(r.Context())
	if !ok {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		m, err = h.q.GetMeasure(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	id := m.ID
	flash := r.URL.Query().Get("flash")
	flashType := r.URL.Query().Get("type")

	linkedReqs, err := h.q.ListRequirementsForMeasure(r.Context(), id)
	if err != nil {
		slog.Warn("list requirements for measure edit", "measure_id", id.String(), "error", err) // #nosec G706
	}

	users, err := h.q.ListUsers(r.Context())
	if err != nil {
		slog.Warn("list users for measure edit", "error", err)
	}
	links, err := h.q.ListMeasureLinks(r.Context(), id)
	if err != nil {
		slog.Warn("list measure links for edit", "measure_id", id.String(), "error", err) // #nosec G706
	}
	auditLog, err := h.q.ListAuditLogForMeasure(r.Context(), id.String())
	if err != nil {
		slog.Warn("list audit log for measure edit", "measure_id", id.String(), "error", err) // #nosec G706
	}
	vm := measurestemplates.MeasureEditVM{
		Measure:    m,
		LinkedReqs: linkedReqs,
		Users:      users,
		Links:      links,
		AuditLog:   auditLog,
	}

	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("Edit measure", m.Name, "measures", user,
		measurestemplates.MeasureEditPage(vm, flash, flashType),
	))
}

func (h *Handler) SearchMeasureRequirements(w http.ResponseWriter, r *http.Request) {
	m, ok := measureFromContext(r.Context())
	if !ok {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		m, err = h.q.GetMeasure(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	id := m.ID
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		httputil.Render(w, r, measurestemplates.MeasureRequirementSearchResults(id.String(), nil, query))
		return
	}
	results, err := h.searchRequirementsForMeasure(r.Context(), id, query)
	if err != nil {
		slog.Error("search requirements for measure", "measure_id", id.String(), "error", err) // #nosec G706
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}
	httputil.Render(w, r, measurestemplates.MeasureRequirementSearchResults(id.String(), results, query))
}

func (h *Handler) searchRequirementsForMeasure(
	ctx context.Context,
	measureID uuid.UUID,
	query string,
) ([]db.ListRequirementsForMeasureRow, error) {
	linkedReqs, err := h.q.ListRequirementsForMeasure(ctx, measureID)
	if err != nil {
		slog.Warn("list linked requirements for measure search", "measure_id", measureID.String(), "error", err) // #nosec G706
	}
	linkedSet := make(map[uuid.UUID]bool, len(linkedReqs))
	for _, req := range linkedReqs {
		linkedSet[req.ID] = true
	}
	lowerQuery := strings.ToLower(query)
	frameworks, err := h.q.ListFrameworks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list frameworks: %w", err)
	}
	results := make([]db.ListRequirementsForMeasureRow, 0, 10)
	for _, fw := range frameworks {
		remaining := 10 - len(results)
		rows, reqErr := h.searchRequirementsInFramework(ctx, fw, linkedSet, lowerQuery, remaining)
		if reqErr != nil {
			slog.Warn("list requirements by framework for measure search", "framework_id", fw.ID, "error", reqErr)
			continue
		}
		results = append(results, rows...)
		if len(results) >= 10 {
			return results, nil
		}
	}
	return results, nil
}

func (h *Handler) searchRequirementsInFramework(
	ctx context.Context,
	fw db.Framework,
	linkedSet map[uuid.UUID]bool,
	lowerQuery string,
	limit int,
) ([]db.ListRequirementsForMeasureRow, error) {
	reqs, err := h.q.ListRequirementsByFramework(ctx, fw.ID)
	if err != nil {
		return nil, fmt.Errorf("list requirements by framework: %w", err)
	}
	rows := make([]db.ListRequirementsForMeasureRow, 0, limit)
	for _, req := range reqs {
		if linkedSet[req.ID] {
			continue
		}
		if !strings.Contains(strings.ToLower(req.Ref), lowerQuery) && !strings.Contains(strings.ToLower(req.Title), lowerQuery) {
			continue
		}
		rows = append(rows, db.ListRequirementsForMeasureRow{
			ID:                 req.ID,
			Ref:                req.Ref,
			Title:              req.Title,
			FrameworkShortName: fw.ShortName,
		})
		if len(rows) >= limit {
			return rows, nil
		}
	}
	return rows, nil
}

func (h *Handler) LinkRequirement(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err = r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	reqID, err := uuid.Parse(r.FormValue("requirement_id"))
	if err != nil {
		httputil.RedirectWithFlash(w, r, "/measures/"+id.String()+"/edit", "Invalid requirement.", "error")
		return
	}
	if err := h.q.LinkMeasureToRequirement(r.Context(), db.LinkMeasureToRequirementParams{
		MeasureID:     id,
		RequirementID: reqID,
		Note:          "",
	}); err != nil {
		slog.Error("link requirement", "error", err)
		httputil.RedirectWithFlash(w, r, "/measures/"+id.String()+"/edit", "Failed to link requirement.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "measures.measure.requirement_linked",
		Attrs: map[string]any{"measure_id": id.String(), "requirement_id": reqID.String()},
	})
	httputil.RedirectWithFlash(w, r, "/measures/"+id.String()+"/edit", "Requirement linked.", "success")
}

func (h *Handler) UnlinkRequirement(w http.ResponseWriter, r *http.Request) { //nolint:dupl
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	reqID, err := uuid.Parse(r.PathValue("req_id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.q.UnlinkMeasureFromRequirement(r.Context(), db.UnlinkMeasureFromRequirementParams{
		MeasureID:     id,
		RequirementID: reqID,
	}); err != nil {
		slog.Error("unlink requirement", "error", err)
		httputil.RedirectWithFlash(w, r, "/measures/"+id.String()+"/edit", "Failed to unlink requirement.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "measures.measure.requirement_unlinked",
		Attrs: map[string]any{"measure_id": id.String(), "requirement_id": reqID.String()},
	})
	httputil.RedirectWithFlash(w, r, "/measures/"+id.String()+"/edit", "Requirement unlinked.", "success")
}

func (h *Handler) AddLink(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err = r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	rawURL := strings.TrimSpace(r.FormValue("url"))
	if rawURL == "" {
		httputil.RedirectWithFlash(w, r, "/measures/"+id.String()+"/edit", "URL is required.", "error")
		return
	}
	if err := httputil.ValidateURL(rawURL); err != nil {
		httputil.RedirectWithFlash(w, r, "/measures/"+id.String()+"/edit", "Invalid URL: "+err.Error(), "error")
		return
	}
	label := strings.TrimSpace(r.FormValue("label"))
	if _, err := h.q.AddMeasureLink(r.Context(), db.AddMeasureLinkParams{
		MeasureID: id,
		Url:       rawURL,
		Label:     label,
	}); err != nil {
		slog.Error("add measure link", "error", err)
		httputil.RedirectWithFlash(w, r, "/measures/"+id.String()+"/edit", "Failed to add link.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "measures.measure.link_added",
		Attrs: map[string]any{"measure_id": id.String(), "url": rawURL},
	})
	httputil.RedirectWithFlash(w, r, "/measures/"+id.String()+"/edit", "Link added.", "success")
}

func (h *Handler) DeleteLink(w http.ResponseWriter, r *http.Request) { //nolint:dupl
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	linkID, err := uuid.Parse(r.PathValue("link_id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.q.DeleteMeasureLink(r.Context(), db.DeleteMeasureLinkParams{
		ID:        linkID,
		MeasureID: id,
	}); err != nil {
		slog.Error("delete measure link", "error", err)
		httputil.RedirectWithFlash(w, r, "/measures/"+id.String()+"/edit", "Failed to remove link.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "measures.measure.link_deleted",
		Attrs: map[string]any{"measure_id": id.String(), "link_id": linkID.String()},
	})
	httputil.RedirectWithFlash(w, r, "/measures/"+id.String()+"/edit", "Link removed.", "success")
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	existing, ok := measureFromContext(r.Context())
	if !ok {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		existing, err = h.q.GetMeasure(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	id := existing.ID

	updateUser, _ := middleware.FromContext(r.Context())
	updateUserID, err := uuid.Parse(updateUser.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err = r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	status := r.FormValue("status")
	if status == "" {
		status = "planned"
	}
	owner, assigneeID := parseMeasureAssignee(r, h.q)
	updated, err := h.q.UpdateMeasure(r.Context(), db.UpdateMeasureParams{
		ID:          id,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Category:    strings.TrimSpace(r.FormValue("category")),
		Owner:       owner,
		AssigneeID:  assigneeID,
		Status:      status,
	})
	if err != nil {
		slog.Error("update measure", "error", err)
		existingForError, getErr := h.q.GetMeasure(r.Context(), id)
		if getErr != nil {
			slog.Warn("get measure for error render", "error", getErr)
		}
		users, userErr := h.q.ListUsers(r.Context())
		if userErr != nil {
			slog.Warn("list users for update measure error", "error", userErr)
		}
		user, _ := middleware.FromContext(r.Context())
		httputil.Render(w, r, layout.Layout("Edit measure", existingForError.Name, "measures", user,
			measurestemplates.MeasureForm(&existingForError, users, "Failed to save changes.", "error"),
		))
		return
	}
	h.handleMeasureTransition(r.Context(), existing.Status, updated.Status, updated.ID, updateUserID)
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "measures.measure.update",
		Attrs: map[string]any{"measure_id": id.String()},
	})
	httputil.RedirectWithFlash(w, r, "/measures", "Measure updated.", "success")
}

func (h *Handler) handleMeasureTransition(ctx context.Context, previousStatus, nextStatus string, measureID, actorID uuid.UUID) {
	if !shouldFlagRiskReassessment(previousStatus, nextStatus) {
		return
	}
	flagged := h.flagLinkedRisksForReassessment(ctx, measureID, nextStatus, actorID)
	if flagged == 0 {
		return
	}
	audit.RecordOrWarn(ctx, h.q, audit.Event{
		Event: "measures.measure.reassessment_flagged",
		Attrs: map[string]any{
			"measure_id":      measureID.String(),
			"status":          nextStatus,
			"linked_risk_cnt": flagged,
		},
	})
}

func shouldFlagRiskReassessment(previousStatus, nextStatus string) bool {
	if previousStatus == nextStatus {
		return false
	}
	return nextStatus == "implemented" || nextStatus == "deprecated"
}

func (h *Handler) flagLinkedRisksForReassessment(ctx context.Context, measureID uuid.UUID, nextStatus string, actorID uuid.UUID) int {
	risks, err := h.q.ListRisksForMeasure(ctx, measureID)
	if err != nil {
		slog.Error("list linked risks for measure transition", "measure_id", measureID, "error", err)
		return 0
	}
	actor := uuid.NullUUID{}
	if actorID != uuid.Nil {
		actor = uuid.NullUUID{UUID: actorID, Valid: true}
	}
	flagged := 0
	for _, risk := range risks {
		if err := h.q.FlagRiskForReview(ctx, risk.ID); err != nil {
			slog.Error("flag risk for reassessment", "risk_id", risk.ID, "measure_id", measureID, "error", err)
			continue
		}
		if err := h.q.CreateRiskReassessmentEvent(ctx, db.CreateRiskReassessmentEventParams{
			RiskID:        risk.ID,
			MeasureID:     measureID,
			TriggerStatus: nextStatus,
			TriggeredBy:   actor,
			Note:          "",
		}); err != nil {
			slog.Error("create risk reassessment event", "risk_id", risk.ID, "measure_id", measureID, "error", err)
			continue
		}
		flagged++
	}
	return flagged
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.q.DeleteMeasure(r.Context(), id); err != nil {
		slog.Error("delete measure", "error", err)
		httputil.RedirectWithFlash(w, r, "/measures", "Failed to delete measure.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "measures.measure.delete",
		Attrs: map[string]any{"measure_id": id.String()},
	})
	httputil.RedirectWithFlash(w, r, "/measures", "Measure deleted.", "success")
}

// parseMeasureAssignee resolves assignee from form while keeping owner as independent text.
func parseMeasureAssignee(r *http.Request, q db.Querier) (owner string, assigneeID uuid.NullUUID) {
	owner = strings.TrimSpace(r.FormValue("owner"))
	rawUID := strings.TrimSpace(r.FormValue("assignee_id"))
	if rawUID != "" {
		uid, err := uuid.Parse(rawUID)
		if err == nil {
			return owner, uuid.NullUUID{UUID: uid, Valid: true}
		}
	}
	lookup := strings.TrimSpace(r.FormValue("assignee_lookup"))
	if lookup == "" {
		return owner, uuid.NullUUID{}
	}
	users, err := q.ListUsers(r.Context())
	if err != nil {
		return owner, uuid.NullUUID{}
	}
	for _, u := range users {
		display := strings.TrimSpace(u.Name + " (" + u.Email + ")")
		if strings.EqualFold(lookup, display) || strings.EqualFold(lookup, u.Name) || strings.EqualFold(lookup, u.Email) {
			return owner, uuid.NullUUID{UUID: u.ID, Valid: true}
		}
	}
	return owner, uuid.NullUUID{}
}
