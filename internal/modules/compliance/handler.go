// Package compliance manages frameworks, requirements, and their mapping to measures.
package compliance

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/3lbits/vigil/internal/audit"
	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/middleware"
	compliancetemplates "github.com/3lbits/vigil/internal/modules/compliance/templates"
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

// ── Helpers ──

func (h *Handler) listPage(w http.ResponseWriter, r *http.Request, flash, flashType string) {
	expandID := r.URL.Query().Get("expand")
	user, _ := middleware.FromContext(r.Context())

	frameworks, err := h.q.ListFrameworks(r.Context())
	if err != nil {
		slog.Error("list frameworks", "error", err)
		http.Error(w, "failed to load frameworks", http.StatusInternalServerError)
		return
	}

	vms := make([]compliancetemplates.FrameworkVM, 0, len(frameworks))
	for _, fw := range frameworks {
		total, err := h.q.CountRequirementsByFramework(r.Context(), fw.ID)
		if err != nil {
			slog.Warn("count requirements by framework", "framework_id", fw.ID, "error", err)
		}
		covered, err := h.q.CountCoveredRequirementsByFramework(r.Context(), fw.ID)
		if err != nil {
			slog.Warn("count covered requirements by framework", "framework_id", fw.ID, "error", err)
		}
		pct := 0
		if total > 0 {
			pct = int(covered * 100 / total)
		}

		reqs, err := h.q.ListRequirementsByFramework(r.Context(), fw.ID)
		if err != nil {
			slog.Warn("list requirements by framework", "framework_id", fw.ID, "error", err)
		}
		reqVMs := make([]compliancetemplates.RequirementVM, 0, len(reqs))
		for _, req := range reqs {
			measures, err := h.q.ListMeasuresForRequirement(r.Context(), req.ID)
			if err != nil {
				slog.Warn("list measures for requirement", "requirement_id", req.ID, "error", err)
			}
			reqVMs = append(reqVMs, compliancetemplates.RequirementVM{
				Requirement: req,
				Measures:    measures,
			})
		}

		vms = append(vms, compliancetemplates.FrameworkVM{
			Framework:    fw,
			Coverage:     pct,
			TotalReqs:    total,
			CoveredReqs:  covered,
			Requirements: reqVMs,
		})
	}

	httputil.Render(w, r, layout.Layout("Compliance", "Frameworks and requirements", "compliance", user,
		compliancetemplates.FrameworkList(vms, expandID, flash, flashType),
	))
}

// ── Framework handlers ──

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	flash := r.URL.Query().Get("flash")
	flashType := r.URL.Query().Get("type")
	h.listPage(w, r, flash, flashType)
}

func (h *Handler) NewFramework(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("Add framework", "Register a new compliance standard", "compliance", user,
		compliancetemplates.FrameworkForm(nil, "", "", nil),
	))
}

func (h *Handler) CreateFramework(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	fwType := strings.TrimSpace(r.FormValue("framework_type"))
	if fwType == "" {
		fwType = "regulation"
	}
	fw, err := h.q.CreateFramework(r.Context(), db.CreateFrameworkParams{
		Name:          strings.TrimSpace(r.FormValue("name")),
		ShortName:     strings.TrimSpace(r.FormValue("short_name")),
		Version:       strings.TrimSpace(r.FormValue("version")),
		Description:   strings.TrimSpace(r.FormValue("description")),
		FrameworkType: fwType,
	})
	if err != nil {
		slog.Error("create framework", "error", err)
		user, _ := middleware.FromContext(r.Context())
		httputil.Render(w, r, layout.Layout("Add framework", "Register a new compliance standard", "compliance", user,
			compliancetemplates.FrameworkForm(nil, "Failed to create framework.", "error", nil),
		))
		return
	}

	createFwUser, _ := middleware.FromContext(r.Context())
	if uid, parseErr := uuid.Parse(createFwUser.ID); parseErr == nil {
		if addErr := h.q.AddParticipant(r.Context(), db.AddParticipantParams{
			ResourceType: "frameworks",
			ResourceID:   fw.ID,
			UserID:       uid,
			Role:         "owner",
		}); addErr != nil {
			slog.Warn("add participant on create framework", "error", addErr)
		}
	}

	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "compliance.framework.create",
		Attrs: map[string]any{"framework_id": fw.ID.String(), "name": fw.Name},
	})
	httputil.RedirectWithFlash(w, r, "/compliance", "Framework created.", "success")
}

func (h *Handler) EditFramework(w http.ResponseWriter, r *http.Request) {
	fw, ok := frameworkFromContext(r.Context())
	if !ok {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		fw, err = h.q.GetFramework(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	id := fw.ID
	auditLog, err := h.q.ListAuditLogForFramework(r.Context(), id.String())
	if err != nil {
		slog.Warn("list audit log for framework edit", "framework_id", id.String(), "error", err) // #nosec G706
	}
	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("Edit framework", fw.Name, "compliance", user,
		compliancetemplates.FrameworkForm(&fw, "", "", auditLog),
	))
}

func (h *Handler) UpdateFramework(w http.ResponseWriter, r *http.Request) {
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
	updFwType := strings.TrimSpace(r.FormValue("framework_type"))
	if updFwType == "" {
		updFwType = "regulation"
	}
	fw, err := h.q.UpdateFramework(r.Context(), db.UpdateFrameworkParams{
		ID:                id,
		Name:              strings.TrimSpace(r.FormValue("name")),
		ShortName:         strings.TrimSpace(r.FormValue("short_name")),
		Version:           strings.TrimSpace(r.FormValue("version")),
		Description:       strings.TrimSpace(r.FormValue("description")),
		FrameworkType:     updFwType,
		NotRelevant:       r.FormValue("not_relevant") == "on",
		NotRelevantReason: strings.TrimSpace(r.FormValue("not_relevant_reason")),
	})
	if err != nil {
		slog.Error("update framework", "error", err)
		existing, getErr := h.q.GetFramework(r.Context(), id)
		if getErr != nil {
			slog.Warn("get framework for error render", "error", getErr)
		}
		auditLog, logErr := h.q.ListAuditLogForFramework(r.Context(), id.String())
		if logErr != nil {
			slog.Warn("list audit log for framework edit", "framework_id", id.String(), "error", logErr) // #nosec G706
		}
		user, _ := middleware.FromContext(r.Context())
		httputil.Render(w, r, layout.Layout("Edit framework", existing.Name, "compliance", user,
			compliancetemplates.FrameworkForm(&existing, "Failed to save changes.", "error", auditLog),
		))
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "compliance.framework.update",
		Attrs: map[string]any{"framework_id": fw.ID.String(), "name": fw.Name},
	})
	httputil.RedirectWithFlash(w, r, "/compliance", "Framework updated.", "success")
}

func (h *Handler) DeleteFramework(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.q.DeleteFramework(r.Context(), id); err != nil {
		slog.Error("delete framework", "error", err)
		httputil.RedirectWithFlash(w, r, "/compliance", "Failed to delete framework.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "compliance.framework.delete",
		Attrs: map[string]any{"framework_id": id.String()},
	})
	httputil.RedirectWithFlash(w, r, "/compliance", "Framework deleted.", "success")
}

// ── Requirement handlers ──

func (h *Handler) NewRequirement(w http.ResponseWriter, r *http.Request) {
	fwID := r.PathValue("id")
	if _, err := uuid.Parse(fwID); err != nil {
		http.NotFound(w, r)
		return
	}
	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("Add requirement", "New requirement", "compliance", user,
		compliancetemplates.RequirementForm(fwID, nil, "", "", nil),
	))
}

func (h *Handler) CreateRequirement(w http.ResponseWriter, r *http.Request) {
	fwID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err = r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sortOrder, _ := strconv.ParseInt(r.FormValue("sort_order"), 10, 32)
	req, err := h.q.CreateRequirement(r.Context(), db.CreateRequirementParams{
		FrameworkID: fwID,
		Ref:         strings.TrimSpace(r.FormValue("ref")),
		Title:       strings.TrimSpace(r.FormValue("title")),
		Description: strings.TrimSpace(r.FormValue("description")),
		SortOrder:   int32(sortOrder),
	})
	if err != nil {
		slog.Error("create requirement", "error", err)
		user, _ := middleware.FromContext(r.Context())
		httputil.Render(w, r, layout.Layout("Add requirement", "New requirement", "compliance", user,
			compliancetemplates.RequirementForm(fwID.String(), nil, "Failed to create requirement.", "error", nil),
		))
		return
	}

	createReqUser, _ := middleware.FromContext(r.Context())
	if uid, parseErr := uuid.Parse(createReqUser.ID); parseErr == nil {
		if addErr := h.q.AddParticipant(r.Context(), db.AddParticipantParams{
			ResourceType: "requirements",
			ResourceID:   req.ID,
			UserID:       uid,
			Role:         "owner",
		}); addErr != nil {
			slog.Warn("add participant on create requirement", "error", addErr)
		}
	}

	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "compliance.requirement.create",
		Attrs: map[string]any{"requirement_id": req.ID.String(), "framework_id": fwID.String(), "ref": strings.TrimSpace(r.FormValue("ref"))},
	})
	httputil.RedirectWithFlash(w, r, "/compliance", "Requirement created.", "success")
}

func (h *Handler) ShowFramework(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	fw, err := h.q.GetFramework(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	reqs, err := h.q.ListRequirementsByFramework(r.Context(), id)
	if err != nil {
		slog.Warn("list requirements for framework", "framework_id", id.String(), "error", err) // #nosec G706
	}
	covered, err := h.q.CountCoveredRequirementsByFramework(r.Context(), id)
	if err != nil {
		slog.Warn("count covered reqs", "framework_id", id.String(), "error", err) // #nosec G706
	}
	auditLog, err := h.q.ListAuditLogForFramework(r.Context(), id.String())
	if err != nil {
		slog.Warn("list audit log for framework", "framework_id", id.String(), "error", err) // #nosec G706
	}
	user, _ := middleware.FromContext(r.Context())
	canEdit, err := h.engine.Allow(r.Context(), user.ID, user.Role, "frameworks", "write")
	if err != nil {
		slog.Error("authz eval", "error", err)
	}
	vm := compliancetemplates.FrameworkDetailVM{
		Framework:    fw,
		Requirements: reqs,
		CoveredReqs:  covered,
		CanEdit:      canEdit,
		AuditLog:     auditLog,
	}
	httputil.Render(w, r, layout.Layout(fw.Name, fw.ShortName+" · "+fw.FrameworkType, "compliance", user,
		compliancetemplates.FrameworkDetail(vm),
	))
}

func (h *Handler) ShowRequirement(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	req, err := h.q.GetRequirement(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	fw, err := h.q.GetFramework(r.Context(), req.FrameworkID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	measures, err := h.q.ListMeasuresForRequirement(r.Context(), id)
	if err != nil {
		slog.Warn("list measures for requirement", "requirement_id", id.String(), "error", err) // #nosec G706
	}
	auditLog, err := h.q.ListAuditLogForRequirement(r.Context(), id.String())
	if err != nil {
		slog.Warn("list audit log for requirement", "requirement_id", id.String(), "error", err) // #nosec G706
	}
	user, _ := middleware.FromContext(r.Context())
	canEdit, err := h.engine.Allow(r.Context(), user.ID, user.Role, "frameworks", "write")
	if err != nil {
		slog.Error("authz eval", "error", err)
	}
	vm := compliancetemplates.RequirementDetailVM{
		Requirement:    req,
		FrameworkName:  fw.Name,
		FrameworkShort: fw.ShortName,
		Measures:       measures,
		CanEdit:        canEdit,
		AuditLog:       auditLog,
	}
	httputil.Render(w, r, layout.Layout(req.Title, req.Ref+" · "+fw.Name, "compliance", user,
		compliancetemplates.RequirementDetail(vm),
	))
}

func (h *Handler) EditRequirement(w http.ResponseWriter, r *http.Request) {
	req, ok := requirementFromContext(r.Context())
	if !ok {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		req, err = h.q.GetRequirement(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	id := req.ID
	flash := r.URL.Query().Get("flash")
	flashType := r.URL.Query().Get("type")

	linkedMeasures, err := h.q.ListMeasuresForRequirement(r.Context(), id)
	if err != nil {
		slog.Warn("list linked measures for requirement edit", "requirement_id", id.String(), "error", err) // #nosec G706
	}
	auditLog, err := h.q.ListAuditLogForRequirement(r.Context(), id.String())
	if err != nil {
		slog.Warn("list audit log for requirement edit", "requirement_id", id.String(), "error", err) // #nosec G706
	}

	vm := compliancetemplates.RequirementEditVM{
		Requirement:    req,
		LinkedMeasures: linkedMeasures,
		AuditLog:       auditLog,
	}

	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("Edit requirement", req.Title, "compliance", user,
		compliancetemplates.RequirementEditPage(vm, flash, flashType),
	))
}

func (h *Handler) SearchRequirementMeasures(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, reqErr := h.q.GetRequirement(r.Context(), id); reqErr != nil {
		http.NotFound(w, r)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		httputil.Render(w, r, compliancetemplates.RequirementMeasureSearchResults(id.String(), nil, query))
		return
	}
	linkedMeasures, err := h.q.ListMeasuresForRequirement(r.Context(), id)
	if err != nil {
		slog.Warn("list linked measures for requirement search", "requirement_id", id.String(), "error", err) // #nosec G706
	}
	linkedSet := make(map[uuid.UUID]bool, len(linkedMeasures))
	for _, m := range linkedMeasures {
		linkedSet[m.ID] = true
	}
	measures, err := h.q.SearchMeasures(r.Context(), "%"+query+"%")
	if err != nil {
		slog.Error("search measures for requirement", "requirement_id", id.String(), "error", err) // #nosec G706
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}
	available := make([]db.Measure, 0, len(measures))
	for _, m := range measures {
		if !linkedSet[m.ID] {
			available = append(available, m)
		}
	}
	httputil.Render(w, r, compliancetemplates.RequirementMeasureSearchResults(id.String(), available, query))
}

func (h *Handler) LinkMeasure(w http.ResponseWriter, r *http.Request) {
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
	measureID, err := uuid.Parse(r.FormValue("measure_id"))
	if err != nil {
		httputil.RedirectWithFlash(w, r, "/compliance/requirements/"+id.String()+"/edit", "Invalid measure.", "error")
		return
	}
	if err := h.q.LinkMeasureToRequirement(r.Context(), db.LinkMeasureToRequirementParams{
		MeasureID:     measureID,
		RequirementID: id,
		Note:          "",
	}); err != nil {
		slog.Error("link measure", "error", err)
		httputil.RedirectWithFlash(w, r, "/compliance/requirements/"+id.String()+"/edit", "Failed to link measure.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "compliance.requirement.measure_linked",
		Attrs: map[string]any{"requirement_id": id.String(), "measure_id": measureID.String()},
	})
	httputil.RedirectWithFlash(w, r, "/compliance/requirements/"+id.String()+"/edit", "Measure linked.", "success")
}

func (h *Handler) UnlinkMeasure(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	measureID, err := uuid.Parse(r.PathValue("measure_id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.q.UnlinkMeasureFromRequirement(r.Context(), db.UnlinkMeasureFromRequirementParams{
		MeasureID:     measureID,
		RequirementID: id,
	}); err != nil {
		slog.Error("unlink measure", "error", err)
		httputil.RedirectWithFlash(w, r, "/compliance/requirements/"+id.String()+"/edit", "Failed to unlink measure.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "compliance.requirement.measure_unlinked",
		Attrs: map[string]any{"requirement_id": id.String(), "measure_id": measureID.String()},
	})
	httputil.RedirectWithFlash(w, r, "/compliance/requirements/"+id.String()+"/edit", "Measure unlinked.", "success")
}

func (h *Handler) UpdateRequirement(w http.ResponseWriter, r *http.Request) {
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
	sortOrder, _ := strconv.ParseInt(r.FormValue("sort_order"), 10, 32)
	_, err = h.q.UpdateRequirement(r.Context(), db.UpdateRequirementParams{
		ID:                id,
		Ref:               strings.TrimSpace(r.FormValue("ref")),
		Title:             strings.TrimSpace(r.FormValue("title")),
		Description:       strings.TrimSpace(r.FormValue("description")),
		SortOrder:         int32(sortOrder),
		NotRelevant:       r.FormValue("not_relevant") == "on",
		NotRelevantReason: strings.TrimSpace(r.FormValue("not_relevant_reason")),
	})
	if err != nil {
		slog.Error("update requirement", "error", err)
		existing, getErr := h.q.GetRequirement(r.Context(), id)
		if getErr != nil {
			slog.Warn("get requirement for error render", "error", getErr)
		}
		auditLog, logErr := h.q.ListAuditLogForRequirement(r.Context(), id.String())
		if logErr != nil {
			slog.Warn("list audit log for requirement edit", "requirement_id", id.String(), "error", logErr) // #nosec G706
		}
		user, _ := middleware.FromContext(r.Context())
		httputil.Render(w, r, layout.Layout("Edit requirement", existing.Title, "compliance", user,
			compliancetemplates.RequirementForm(existing.FrameworkID.String(), &existing, "Failed to save changes.", "error", auditLog),
		))
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "compliance.requirement.update",
		Attrs: map[string]any{"requirement_id": id.String()},
	})
	httputil.RedirectWithFlash(w, r, "/compliance", "Requirement updated.", "success")
}

func (h *Handler) DeleteRequirement(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	req, err := h.q.GetRequirement(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	redirectPath := "/compliance/frameworks/" + req.FrameworkID.String()
	if err := h.q.DeleteRequirement(r.Context(), id); err != nil {
		slog.Error("delete requirement", "error", err)
		httputil.RedirectWithFlash(w, r, redirectPath, "Failed to delete requirement.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "compliance.requirement.delete",
		Attrs: map[string]any{"requirement_id": id.String(), "framework_id": req.FrameworkID.String()},
	})
	httputil.RedirectWithFlash(w, r, redirectPath, "Requirement deleted.", "success")
}

// ── CSV import ──

func (h *Handler) ShowImportForm(w http.ResponseWriter, r *http.Request) {
	fwID := r.PathValue("id")
	if _, err := uuid.Parse(fwID); err != nil {
		http.NotFound(w, r)
		return
	}
	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("Import requirements", "Upload CSV", "compliance", user,
		compliancetemplates.ImportForm(fwID, "", "", nil),
	))
}

func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	fwID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	if err = r.ParseMultipartForm(5 << 20); err != nil { // #nosec G120 -- body already capped by MaxBytesReader above
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("csv_file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("close upload file", "error", err)
		}
	}()

	result := parseAndImportCSV(r, h.q, fwID, file)

	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("Import requirements", "Upload CSV", "compliance", user,
		compliancetemplates.ImportForm(fwID.String(), "", "", result),
	))
}

func parseAndImportCSV(r *http.Request, q db.Querier, fwID uuid.UUID, f io.Reader) *compliancetemplates.ImportResult { //nolint:gocognit,cyclop
	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		slog.Warn("csv import: failed to read header", "error", err)
		return &compliancetemplates.ImportResult{Errors: []string{"could not read CSV header"}}
	}

	// Build column index map (case-insensitive).
	colIdx := make(map[string]int)
	for i, h := range header {
		colIdx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	refIdx, hasRef := colIdx["ref"]
	titleIdx, hasTitle := colIdx["title"]
	if !hasRef || !hasTitle {
		return &compliancetemplates.ImportResult{Errors: []string{"CSV must have 'ref' and 'title' columns"}}
	}
	descIdx, hasDesc := colIdx["description"]
	sortIdx, hasSort := colIdx["sort_order"]

	result := &compliancetemplates.ImportResult{}
	lineNum := 1
	for {
		lineNum++
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			slog.Warn("csv import: failed to read row", "line", lineNum, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: could not parse row", lineNum))
			continue
		}
		if len(row) == 0 {
			continue
		}

		ref := strings.TrimSpace(row[refIdx])
		title := strings.TrimSpace(row[titleIdx])
		if ref == "" || title == "" {
			result.Errors = append(result.Errors, "line "+strconv.Itoa(lineNum)+": ref and title are required")
			continue
		}

		desc := ""
		if hasDesc && descIdx < len(row) {
			desc = strings.TrimSpace(row[descIdx])
		}
		var sortOrder int64
		if hasSort && sortIdx < len(row) {
			sortOrder, _ = strconv.ParseInt(strings.TrimSpace(row[sortIdx]), 10, 32)
		}

		_, err = q.CreateRequirement(r.Context(), db.CreateRequirementParams{
			FrameworkID: fwID,
			Ref:         ref,
			Title:       title,
			Description: desc,
			SortOrder:   int32(sortOrder),
		})
		if err != nil {
			slog.Warn("csv import: failed to insert requirement", "line", lineNum, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: failed to import (duplicate or invalid data)", lineNum))
			continue
		}
		result.Count++
	}
	return result
}
