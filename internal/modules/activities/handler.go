// Package activities handles the compliance activity register.
package activities

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/audit"
	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/middleware"
	activitiestemplates "github.com/3lbits/vigil/internal/modules/activities/templates"
	"github.com/3lbits/vigil/internal/ui/layout"
)

type Handler struct {
	q      db.Querier
	engine *authz.Engine
}

func NewHandler(q db.Querier, engine *authz.Engine) *Handler {
	return &Handler{q: q, engine: engine}
}

var (
	allowedActivityType = map[string]bool{"one_off": true, "recurring": true}
	allowedRecurrence   = map[string]bool{"none": true, "monthly": true, "quarterly": true, "annual": true, "ad_hoc": true}
	allowedPriority     = map[string]bool{"low": true, "medium": true, "high": true}
	allowedKind         = map[string]bool{"review": true, "training": true, "exercise": true, "assessment": true, "audit": true, "remediation": true}
)

const activitiesPageSize int32 = 50

func normalizeActivitySort(raw string) string {
	switch raw {
	case "default", "title", "kind", "owner", "due_date", "status", "created_at":
		return raw
	default:
		return "default"
	}
}

// List renders activities with server-side filtering and load-more pagination.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if err := h.q.MarkOverdueActivities(r.Context()); err != nil {
		slog.Warn("mark overdue activities", "error", err)
	}

	q := r.URL.Query()
	filter := q.Get("filter")
	kind := q.Get("kind")
	search := q.Get("q")
	sort := normalizeActivitySort(q.Get("sort"))
	dir := httputil.NormalizeSortDir(q.Get("dir"))
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

	rows, err := h.q.FilterActivities(r.Context(), db.FilterActivitiesParams{
		Status:     filter,
		Kind:       kind,
		Q:          search,
		Sort:       sort,
		Dir:        dir,
		Mine:       mine,
		AssigneeID: assigneeID,
		PageSize:   activitiesPageSize + 1,
		PageOffset: offset,
	})
	if err != nil {
		slog.Error("filter activities", "error", err)
		http.Error(w, "failed to load activities", http.StatusInternalServerError)
		return
	}

	hasMore := len(rows) > int(activitiesPageSize)
	if hasMore {
		rows = rows[:activitiesPageSize]
	}

	isHTMX := r.Header.Get("HX-Request") == "true"
	if isHTMX {
		w.Header().Set("HX-Title", "Activities")
	}

	if isHTMX && offset > 0 {
		httputil.Render(w, r, activitiestemplates.ActivityRows(rows, hasMore, offset+activitiesPageSize, filter, kind, search, sort, dir, mine))
		return
	}

	if isHTMX {
		httputil.Render(w, r, activitiestemplates.ActivityTable(rows, hasMore, activitiesPageSize, filter, kind, search, sort, dir, mine))
		return
	}

	httputil.Render(w, r, layout.Layout("Activities", "Compliance activity register", "activities", user,
		activitiestemplates.ActivityList(rows, filter, kind, search, sort, dir, mine, flash, flashType, hasMore, activitiesPageSize),
	))
}

// New renders the activity creation form.
func (h *Handler) New(w http.ResponseWriter, r *http.Request) {
	measures, err := h.q.ListMeasures(r.Context())
	if err != nil {
		slog.Error("list measures for new activity", "error", err)
		http.Error(w, "failed to load measures", http.StatusInternalServerError)
		return
	}

	users, err := h.q.ListUsers(r.Context())
	if err != nil {
		slog.Warn("list users for new activity", "error", err)
	}
	user, _ := middleware.FromContext(r.Context())
	preselected := r.URL.Query().Get("measure_id")
	httputil.Render(w, r, layout.Layout("Add activity", "Create a new compliance activity", "activities", user,
		activitiestemplates.ActivityForm(measures, users, nil, preselected, "", "", nil),
	))
}

// Create handles the activity creation form submission.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var measureID uuid.NullUUID
	if raw := r.FormValue("measure_id"); raw != "" {
		if uid, parseErr := uuid.Parse(raw); parseErr == nil {
			measureID = uuid.NullUUID{UUID: uid, Valid: true}
		}
	}

	actType, recurrence, priority, kind, ok := parseActivityEnums(w,
		strings.TrimSpace(r.FormValue("activity_type")),
		strings.TrimSpace(r.FormValue("recurrence")),
		strings.TrimSpace(r.FormValue("priority")),
		strings.TrimSpace(r.FormValue("kind")),
	)
	if !ok {
		return
	}

	owner, assigneeID := parseActivityAssignee(r, h.q)
	created, err := h.q.CreateActivity(r.Context(), db.CreateActivityParams{
		MeasureID:    measureID,
		Title:        strings.TrimSpace(r.FormValue("title")),
		Description:  strings.TrimSpace(r.FormValue("description")),
		ActivityType: actType,
		Recurrence:   recurrence,
		Priority:     priority,
		Kind:         kind,
		Owner:        owner,
		AssigneeID:   assigneeID,
		DueDate:      parseDateInput(r.FormValue("due_date")),
	})
	if err != nil {
		slog.Error("create activity", "error", err)
		h.renderFormError(w, r, nil, r.FormValue("measure_id"), "Failed to create activity.")
		return
	}

	sessionUser, _ := middleware.FromContext(r.Context())
	if uid, parseErr := uuid.Parse(sessionUser.ID); parseErr == nil {
		if addErr := h.q.AddParticipant(r.Context(), db.AddParticipantParams{
			ResourceType: "activities",
			ResourceID:   created.ID,
			UserID:       uid,
			Role:         "owner",
		}); addErr != nil {
			slog.Warn("add participant on create activity", "error", addErr)
		}
	}

	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "activities.activity.create",
		Attrs: map[string]any{"activity_id": created.ID.String(), "title": strings.TrimSpace(r.FormValue("title"))},
	})
	httputil.RedirectWithFlash(w, r, "/activities", "Activity created.", "success")
}

// Show renders the activity detail page.
func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	a, err := h.q.GetActivity(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user, _ := middleware.FromContext(r.Context())
	canEdit, err := h.engine.Allow(r.Context(), user.ID, user.Role, "activities", "write")
	if err != nil {
		slog.Error("authz eval", "error", err)
	}
	flash := r.URL.Query().Get("flash")
	flashType := r.URL.Query().Get("type")

	users, err := h.q.ListUsers(r.Context())
	if err != nil {
		slog.Warn("list users for activity detail", "error", err)
	}
	auditLog, err := h.q.ListAuditLogForActivity(r.Context(), id.String())
	if err != nil {
		slog.Warn("list audit log for activity", "activity_id", id.String(), "error", err) // #nosec G706
	}
	linkedRisks, err := h.q.ListRisksForActivity(r.Context(), id)
	if err != nil {
		slog.Warn("list risks for activity", "activity_id", id.String(), "error", err) // #nosec G706
	}

	activityTitle := a.Title
	if a.RefNum.Valid {
		activityTitle = fmt.Sprintf("A-%03d · %s", a.RefNum.Int32, a.Title)
	}
	httputil.Render(w, r, layout.Layout(activityTitle, a.MeasureName, "activities", user,
		activitiestemplates.ActivityDetail(a, canEdit, flash, flashType, users, auditLog, linkedRisks),
	))
}

// Edit renders the activity edit form.
func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	a, ok := activityFromContext(r.Context())
	if !ok {
		httputil.InternalServerError(w, r)
		return
	}

	measures, err := h.q.ListMeasures(r.Context())
	if err != nil {
		slog.Error("list measures for edit", "error", err)
		http.Error(w, "failed to load measures", http.StatusInternalServerError)
		return
	}

	users, err := h.q.ListUsers(r.Context())
	if err != nil {
		slog.Warn("list users for edit activity", "error", err)
	}
	auditLog, err := h.q.ListAuditLogForActivity(r.Context(), a.ID.String())
	if err != nil {
		slog.Warn("list audit log for activity edit", "activity_id", a.ID.String(), "error", err) // #nosec G706
	}
	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("Edit activity", a.Title, "activities", user,
		activitiestemplates.ActivityForm(measures, users, &a, "", "", "", auditLog),
	))
}

// Update handles the activity edit form submission.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	a, ok := activityFromContext(r.Context())
	if !ok {
		httputil.InternalServerError(w, r)
		return
	}
	id := a.ID

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	actType, recurrence, priority, kind, ok := parseActivityEnums(w,
		strings.TrimSpace(r.FormValue("activity_type")),
		strings.TrimSpace(r.FormValue("recurrence")),
		strings.TrimSpace(r.FormValue("priority")),
		strings.TrimSpace(r.FormValue("kind")),
	)
	if !ok {
		return
	}

	owner, assigneeID := parseActivityAssignee(r, h.q)
	_, err := h.q.UpdateActivity(r.Context(), db.UpdateActivityParams{
		ID:           id,
		Title:        strings.TrimSpace(r.FormValue("title")),
		Description:  strings.TrimSpace(r.FormValue("description")),
		ActivityType: actType,
		Recurrence:   recurrence,
		Priority:     priority,
		Kind:         kind,
		Owner:        owner,
		AssigneeID:   assigneeID,
		DueDate:      parseDateInput(r.FormValue("due_date")),
	})
	if err != nil {
		slog.Error("update activity", "error", err)
		httputil.RedirectWithFlash(w, r, "/activities/"+id.String()+"/edit", "Failed to save changes.", "error")
		return
	}

	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "activities.activity.update",
		Attrs: map[string]any{"activity_id": id.String()},
	})
	httputil.RedirectWithFlash(w, r, "/activities/"+id.String(), "Activity updated.", "success")
}

// Complete marks an activity as completed, updates measure staleness, and
// auto-creates the next instance for recurring activities.
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
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

	evidenceURL := strings.TrimSpace(r.FormValue("evidence_url"))
	if evidenceURL != "" {
		if urlErr := httputil.ValidateURL(evidenceURL); urlErr != nil {
			httputil.RedirectWithFlash(w, r, "/activities/"+id.String(), "Evidence URL must use http or https.", "error")
			return
		}
	}
	completed, err := h.q.CompleteActivity(r.Context(), db.CompleteActivityParams{
		ID:          id,
		CompletedBy: strings.TrimSpace(r.FormValue("completed_by")),
		Notes:       strings.TrimSpace(r.FormValue("notes")),
		EvidenceUrl: evidenceURL,
	})
	if err != nil {
		slog.Error("complete activity", "error", err)
		httputil.RedirectWithFlash(w, r, "/activities/"+id.String(), "Failed to complete activity.", "error")
		return
	}

	// Update parent measure's last_verified_at if linked.
	if completed.MeasureID.Valid {
		if err := h.q.UpdateMeasureLastVerified(r.Context(), completed.MeasureID.UUID); err != nil {
			slog.Error("update measure last verified", "error", err)
		}
	}

	h.maybeCreateNextRecurring(r.Context(), completed)

	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "activities.activity.complete",
		Attrs: map[string]any{"activity_id": id.String(), "completed_by": strings.TrimSpace(r.FormValue("completed_by"))},
	})
	httputil.RedirectWithFlash(w, r, "/activities/"+id.String(), "Activity completed.", "success")
}

// Reopen resets a completed activity back to planned.
func (h *Handler) Reopen(w http.ResponseWriter, r *http.Request) { //nolint:dupl
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, err = h.q.ReopenActivity(r.Context(), id); err != nil {
		slog.Error("reopen activity", "error", err)
		httputil.RedirectWithFlash(w, r, "/activities/"+id.String(), "Failed to reopen activity.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "activities.activity.reopen",
		Attrs: map[string]any{"activity_id": id.String()},
	})
	httputil.RedirectWithFlash(w, r, "/activities/"+id.String(), "Activity reopened.", "success")
}

func (h *Handler) maybeCreateNextRecurring(ctx context.Context, completed db.Activity) {
	if completed.ActivityType != "recurring" || completed.Recurrence == "none" {
		return
	}
	nextDue := nextDueDate(completed.Recurrence, time.Now())
	if !nextDue.Valid {
		return
	}
	_, err := h.q.CreateActivity(ctx, db.CreateActivityParams{
		MeasureID:    completed.MeasureID,
		Title:        completed.Title,
		Description:  completed.Description,
		ActivityType: completed.ActivityType,
		Recurrence:   completed.Recurrence,
		Priority:     completed.Priority,
		Kind:         completed.Kind,
		Owner:        completed.Owner,
		AssigneeID:   completed.AssigneeID,
		DueDate:      nextDue,
	})
	if err != nil {
		slog.Error("create next recurring activity", "error", err)
	}
}

// StartProgress moves an activity to in_progress.
func (h *Handler) StartProgress(w http.ResponseWriter, r *http.Request) { //nolint:dupl
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if _, err := h.q.MarkActivityInProgress(r.Context(), id); err != nil {
		slog.Error("start activity", "error", err)
		httputil.RedirectWithFlash(w, r, "/activities/"+id.String(), "Failed to start activity.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "activities.activity.started",
		Attrs: map[string]any{"activity_id": id.String()},
	})
	httputil.RedirectWithFlash(w, r, "/activities/"+id.String(), "Activity started.", "success")
}

// Delete removes an activity.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.q.DeleteActivity(r.Context(), id); err != nil {
		slog.Error("delete activity", "error", err)
		httputil.RedirectWithFlash(w, r, "/activities/"+id.String(), "Failed to delete activity.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "activities.activity.delete",
		Attrs: map[string]any{"activity_id": id.String()},
	})
	httputil.RedirectWithFlash(w, r, "/activities", "Activity deleted.", "success")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func parseDateInput(s string) sql.NullTime {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullTime{}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

func nextDueDate(recurrence string, from time.Time) sql.NullTime {
	var next time.Time
	switch recurrence {
	case "monthly":
		next = from.AddDate(0, 1, 0)
	case "tertially":
		next = from.AddDate(0, 4, 0)
	case "annual":
		next = from.AddDate(1, 0, 0)
	default:
		return sql.NullTime{}
	}
	return sql.NullTime{Time: next, Valid: true}
}

func (h *Handler) renderFormError(w http.ResponseWriter, r *http.Request, a *db.GetActivityRow, preselected, msg string) {
	measures, err := h.q.ListMeasures(r.Context())
	if err != nil {
		slog.Warn("list measures for form error", "error", err)
	}
	users, err := h.q.ListUsers(r.Context())
	if err != nil {
		slog.Warn("list users for form error", "error", err)
	}
	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("Add activity", "Create a new compliance activity", "activities", user,
		activitiestemplates.ActivityForm(measures, users, a, preselected, msg, "error", nil),
	))
}

// parseActivityAssignee resolves the assignee_id/owner pair from the form.
func parseActivityAssignee(r *http.Request, q db.Querier) (owner string, assigneeID uuid.NullUUID) {
	owner = strings.TrimSpace(r.FormValue("owner"))
	rawUID := strings.TrimSpace(r.FormValue("assignee_id"))
	if rawUID == "" {
		return owner, uuid.NullUUID{}
	}
	uid, err := uuid.Parse(rawUID)
	if err != nil {
		return owner, uuid.NullUUID{}
	}
	assigneeID = uuid.NullUUID{UUID: uid, Valid: true}
	if u, err := q.GetUserByID(r.Context(), uid); err == nil {
		owner = u.Name
	}
	return owner, assigneeID
}

// rejectEnum writes a 400 and returns true if raw is non-empty and not in allowed.
// Callers must return immediately when true is returned.
func rejectEnum(w http.ResponseWriter, field, raw, options string, allowed map[string]bool) bool {
	if raw == "" || allowed[raw] {
		return false
	}
	http.Error(w, "invalid "+field+": must be one of "+options, http.StatusBadRequest)
	return true
}

// parseActivityEnums validates and resolves the activity_type, recurrence, priority,
// and kind fields. Returns ok=false (and writes 400) on invalid input.
func parseActivityEnums(w http.ResponseWriter, actType, recurrence, priority, kind string) (string, string, string, string, bool) {
	if rejectEnum(w, "activity_type", actType, "one_off, recurring", allowedActivityType) {
		return "", "", "", "", false
	}
	if actType == "" {
		actType = "one_off"
	}
	if rejectEnum(w, "recurrence", recurrence, "none, monthly, quarterly, annual, ad_hoc", allowedRecurrence) {
		return "", "", "", "", false
	}
	if recurrence == "" {
		recurrence = "none"
	}
	if rejectEnum(w, "priority", priority, "low, medium, high", allowedPriority) {
		return "", "", "", "", false
	}
	if priority == "" {
		priority = "medium"
	}
	if rejectEnum(w, "kind", kind, "review, training, exercise, assessment, audit, remediation", allowedKind) {
		return "", "", "", "", false
	}
	if kind == "" {
		kind = "review"
	}
	return actType, recurrence, priority, kind, true
}
