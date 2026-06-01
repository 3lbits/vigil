package avvik

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/middleware"
	avviktemplates "github.com/3lbits/vigil/internal/modules/avvik/templates"
	"github.com/3lbits/vigil/internal/ui/layout"
)

type Handler struct {
	q      db.Querier
	sqlDB  *sql.DB
	engine *authz.Engine
}

func NewHandler(q db.Querier, sqlDB *sql.DB, engine *authz.Engine) *Handler {
	return &Handler{q: q, sqlDB: sqlDB, engine: engine}
}

var (
	allowedRiskLevels   = map[string]bool{"low": true, "medium": true, "high": true}
	allowedAvvikStatus  = map[string]bool{"new": true, "triaging": true, "investigating": true, "mitigating": true, "closed": true}
	allowedRelationship = map[string]bool{"corrective": true, "preventive": true, "compensating": true}
	allowedAudience     = map[string]bool{
		"reporter": true, "management": true, "organisation": true,
		"exposed_employees": true, "datatilsynet": true, "nve": true,
		"finanstilsynet": true, "bors": true,
	}
	allowedActivityType = map[string]bool{"one_off": true, "recurring": true}
	allowedRecurrence   = map[string]bool{"none": true, "monthly": true, "quarterly": true, "annual": true, "ad_hoc": true}
	allowedPriority     = map[string]bool{"low": true, "medium": true, "high": true}
	allowedKind         = map[string]bool{"review": true, "training": true, "exercise": true, "assessment": true, "audit": true, "remediation": true}
)

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.FromContext(r.Context())
	args := db.ListAvvikParams{
		Status:          nullString(r.URL.Query().Get("status")),
		RiskLevel:       nullString(r.URL.Query().Get("risk")),
		PersonalData:    nullBoolFromQuery(r.URL.Query().Get("personal_data")),
		Ksi:             nullBoolFromQuery(r.URL.Query().Get("ksi")),
		MarketSensitive: nullBoolFromQuery(r.URL.Query().Get("market_sensitive")),
		OrgUnitID:       parseNullUUID(r.URL.Query().Get("org_unit")),
		Mine:            r.URL.Query().Get("mine") == "on",
		PageOffset:      0,
		PageSize:        100,
	}
	if uid, err := uuid.Parse(user.ID); err == nil {
		args.AssigneeID = uid
	}

	items, err := h.q.ListAvvik(r.Context(), args)
	if err != nil {
		http.Error(w, "failed to load avvik", http.StatusInternalServerError)
		return
	}
	if user.Role == "viewer" {
		for i := range items {
			items[i].ReporterName = "—"
			items[i].ReporterEmail = "—"
		}
	}
	orgs, _ := h.q.ListOrganizations(r.Context())
	if r.Header.Get("HX-Request") == "true" {
		httputil.Render(w, r, avviktemplates.AvvikTable(items))
		return
	}
	httputil.Render(w, r, layout.Layout("Avvik", "Avvik — nonconformities and security events", "avvik", user,
		avviktemplates.AvvikList(items, orgs, r.URL.Query().Get("status"), r.URL.Query().Get("risk"), r.URL.Query().Get("org_unit"), args.Mine),
	))
}

func (h *Handler) New(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.FromContext(r.Context())
	orgs, _ := h.q.ListOrganizations(r.Context())
	users, _ := h.q.ListUsers(r.Context())
	httputil.Render(w, r, layout.Layout("New Avvik", "Create avvik", "avvik", user, avviktemplates.NewAvvikForm(orgs, users)))
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if parseErr := r.ParseForm(); parseErr != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}
	discoveredAt := parseDateOrNow(r.FormValue("discovered_at"))
	reporterName, reporterEmail := h.resolveReporter(r.Context(), strings.TrimSpace(r.FormValue("reporter_name")), strings.TrimSpace(r.FormValue("reporter_email")))
	personalData := r.FormValue("personal_data") == "on"

	rawRiskLevel := strings.TrimSpace(r.FormValue("risk_level"))
	if rejectEnum(w, "risk_level", rawRiskLevel, "low, medium, high", allowedRiskLevels) {
		return
	}
	riskLevel := defaultIfEmpty(rawRiskLevel, "medium")
	deadline := sql.NullTime{}
	if personalData {
		deadline = sql.NullTime{Time: discoveredAt.Add(72 * time.Hour), Valid: true}
	}

	var created db.Avvik
	err := h.withTx(r.Context(), func(qtx db.Querier) error {
		var createErr error
		created, createErr = qtx.CreateAvvik(r.Context(), db.CreateAvvikParams{
			Title:                title,
			Description:          strings.TrimSpace(r.FormValue("description")),
			DiscoveredAt:         discoveredAt,
			ReportedAt:           parseNullTimeRFC3339(r.FormValue("reported_at")),
			ReporterName:         reporterName,
			ReporterEmail:        reporterEmail,
			AssignedTo:           parseNullUUID(r.FormValue("assigned_to")),
			OrgUnitID:            parseNullUUID(r.FormValue("org_unit_id")),
			RiskLevel:            riskLevel,
			Status:               "new",
			PersonalData:         personalData,
			Ksi:                  r.FormValue("ksi") == "on",
			KsiInformationOwner:  strings.TrimSpace(r.FormValue("ksi_information_owner")),
			MarketSensitive:      r.FormValue("market_sensitive") == "on",
			MarketAssessmentNote: strings.TrimSpace(r.FormValue("market_assessment_note")),
			GdprDeadlineAt:       deadline,
			RealisedRiskID:       parseNullUUID(r.FormValue("realised_risk_id")),
		})
		if createErr != nil {
			return fmt.Errorf("create avvik: %w", createErr)
		}
		return h.addEventTx(r.Context(), qtx, created.ID, "created", map[string]any{"title": created.Title})
	})
	if err != nil {
		http.Error(w, "failed to create avvik", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Location", "/avvik/"+created.ID.String())
	w.WriteHeader(http.StatusSeeOther)
}

func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a, err := h.q.GetAvvik(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	user, _ := middleware.FromContext(r.Context())
	if user.Role == "viewer" {
		a.ReporterName = "—"
		a.ReporterEmail = "—"
	}
	tab := defaultIfEmpty(r.URL.Query().Get("tab"), "details")
	events, _ := h.q.ListAvvikEvents(r.Context(), id)
	measures, _ := h.q.ListAvvikMeasures(r.Context(), id)
	activities, _ := h.q.ListAvvikActivities(r.Context(), id)
	attachments, _ := h.q.ListAvvikAttachments(r.Context(), id)
	notifications, _ := h.q.ListAvvikNotifications(r.Context(), id)
	allMeasures, _ := h.q.ListMeasures(r.Context())
	allActivities, _ := h.q.ListActivities(r.Context())
	allUsers, _ := h.q.ListUsers(r.Context())
	canManage := user.Role == "admin"
	canContribute := canManage || h.canReporterOrCreatorAccess(r.Context(), a, user)

	canClose := canCloseAvvik(a)
	data := avviktemplates.DetailData{
		Avvik:         a,
		Tab:           tab,
		Events:        events,
		Measures:      measures,
		Activities:    activities,
		Attachments:   attachments,
		Notifications: notifications,
		AllMeasures:   allMeasures,
		AllActivities: allActivities,
		AllUsers:      allUsers,
		CanClose:      canClose,
		CanManage:     canManage,
		CanContribute: canContribute,
	}
	if r.Header.Get("HX-Request") == "true" {
		httputil.Render(w, r, avviktemplates.AvvikTabPanel(data))
		return
	}
	httputil.Render(w, r, layout.Layout("Avvik", a.Title, "avvik", user, avviktemplates.AvvikDetail(data)))
}

func (h *Handler) Triage(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(w, r, "id")
	if !ok {
		return
	}
	if parseErr := r.ParseForm(); parseErr != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	current, err := h.q.GetAvvik(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	user, _ := middleware.FromContext(r.Context())
	nextPersonal := r.FormValue("personal_data") == "on"
	deadline := current.GdprDeadlineAt
	if !current.PersonalData && nextPersonal && !current.GdprDeadlineAt.Valid {
		deadline = sql.NullTime{Time: current.DiscoveredAt.Add(72 * time.Hour), Valid: true}
	}
	if current.PersonalData && !nextPersonal && user.Role != "admin" {
		httputil.Forbidden(w, r)
		return
	}

	rawRiskLevel := strings.TrimSpace(r.FormValue("risk_level"))
	if rejectEnum(w, "risk_level", rawRiskLevel, "low, medium, high", allowedRiskLevels) {
		return
	}
	riskLevel := defaultIfEmpty(rawRiskLevel, current.RiskLevel)

	err = h.withTx(r.Context(), func(qtx db.Querier) error {
		updated, updateErr := qtx.UpdateAvvikTriage(r.Context(), db.UpdateAvvikTriageParams{
			ID:                   id,
			RiskLevel:            riskLevel,
			PersonalData:         nextPersonal,
			Ksi:                  r.FormValue("ksi") == "on",
			KsiInformationOwner:  strings.TrimSpace(r.FormValue("ksi_information_owner")),
			MarketSensitive:      r.FormValue("market_sensitive") == "on",
			MarketAssessmentNote: strings.TrimSpace(r.FormValue("market_assessment_note")),
			GdprDeadlineAt:       deadline,
			OrgUnitID:            parseNullUUID(r.FormValue("org_unit_id")),
		})
		if updateErr != nil {
			return fmt.Errorf("update triage: %w", updateErr)
		}
		return h.addEventTx(r.Context(), qtx, id, "triaged", map[string]any{
			"risk_level":       updated.RiskLevel,
			"personal_data":    updated.PersonalData,
			"market_sensitive": updated.MarketSensitive,
		})
	})
	if err != nil {
		http.Error(w, "triage update failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/avvik/"+id.String()+"?tab=details", http.StatusSeeOther)
}

func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(w, r, "id")
	if !ok {
		return
	}
	if parseErr := r.ParseForm(); parseErr != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		http.Error(w, "status is required", http.StatusBadRequest)
		return
	}
	if status == "closed" {
		http.Error(w, "use /close endpoint", http.StatusBadRequest)
		return
	}
	if rejectEnum(w, "status", status, "new, triaging, investigating, mitigating", allowedAvvikStatus) {
		return
	}
	err := h.withTx(r.Context(), func(qtx db.Querier) error {
		if _, updateErr := qtx.UpdateAvvikStatus(r.Context(), db.UpdateAvvikStatusParams{
			ID:     id,
			Status: status,
		}); updateErr != nil {
			return fmt.Errorf("update status: %w", updateErr)
		}
		return h.addEventTx(r.Context(), qtx, id, "status_changed", map[string]any{"status": status})
	})
	if err != nil {
		http.Error(w, "status update failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/avvik/"+id.String()+"?tab=timeline", http.StatusSeeOther)
}

func (h *Handler) AddNote(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(w, r, "id")
	if !ok {
		return
	}
	current, err := h.q.GetAvvik(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	user, _ := middleware.FromContext(r.Context())
	if !h.canReporterOrCreatorAccess(r.Context(), current, user) && user.Role != "admin" {
		httputil.Forbidden(w, r)
		return
	}
	if parseErr := r.ParseForm(); parseErr != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if note == "" {
		http.Redirect(w, r, "/avvik/"+id.String()+"?tab=timeline", http.StatusSeeOther)
		return
	}
	if err := h.withTx(r.Context(), func(qtx db.Querier) error {
		return h.addEventTx(r.Context(), qtx, id, "note_added", map[string]any{"note": note})
	}); err != nil {
		http.Error(w, "note failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/avvik/"+id.String()+"?tab=timeline", http.StatusSeeOther)
}

func (h *Handler) LinkMeasure(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(w, r, "id")
	if !ok {
		return
	}
	if parseErr := r.ParseForm(); parseErr != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	rawRelationship := strings.TrimSpace(r.FormValue("relationship"))
	if rejectEnum(w, "relationship", rawRelationship, "corrective, preventive, compensating", allowedRelationship) {
		return
	}
	relationship := defaultIfEmpty(rawRelationship, "corrective")
	var linkedID uuid.UUID
	err := h.withTx(r.Context(), func(qtx db.Querier) error {
		if mid := strings.TrimSpace(r.FormValue("measure_id")); mid != "" {
			parsed, parseErr := uuid.Parse(mid)
			if parseErr != nil {
				return fmt.Errorf("parse measure_id: %w", parseErr)
			}
			linkedID = parsed
		} else {
			owner := strings.TrimSpace(r.FormValue("owner"))
			assigneeID := h.resolveUserLookupAssignee(r.Context(), strings.TrimSpace(r.FormValue("assignee_id")), strings.TrimSpace(r.FormValue("assignee_lookup")))
			m, createErr := qtx.CreateMeasure(r.Context(), db.CreateMeasureParams{
				Name:        defaultIfEmpty(strings.TrimSpace(r.FormValue("name")), "Avvik follow-up"),
				Description: strings.TrimSpace(r.FormValue("description")),
				Category:    strings.TrimSpace(r.FormValue("category")),
				Owner:       owner,
				AssigneeID:  assigneeID,
				Status:      "planned",
			})
			if createErr != nil {
				return fmt.Errorf("create measure: %w", createErr)
			}
			linkedID = m.ID
		}
		if linkErr := qtx.LinkAvvikMeasure(r.Context(), db.LinkAvvikMeasureParams{AvvikID: id, MeasureID: linkedID, Relationship: relationship}); linkErr != nil {
			return fmt.Errorf("link measure: %w", linkErr)
		}
		return h.addEventTx(r.Context(), qtx, id, "measure_linked", map[string]any{"measure_id": linkedID.String(), "relationship": relationship})
	})
	if err != nil {
		http.Error(w, "failed to link measure", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/avvik/"+id.String()+"?tab=measures", http.StatusSeeOther)
}

func (h *Handler) LinkActivity(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// Validate enum fields only when creating a new activity (not when linking an existing one).
	if strings.TrimSpace(r.FormValue("activity_id")) == "" {
		if _, _, _, _, ok := parseActivityEnums(w,
			strings.TrimSpace(r.FormValue("activity_type")),
			strings.TrimSpace(r.FormValue("recurrence")),
			strings.TrimSpace(r.FormValue("priority")),
			strings.TrimSpace(r.FormValue("kind")),
		); !ok {
			return
		}
	}
	var linkedID uuid.UUID
	err := h.withTx(r.Context(), func(qtx db.Querier) error {
		resolvedID, resolveErr := h.resolveActivityToLink(r.Context(), qtx, id, r)
		if resolveErr != nil {
			return resolveErr
		}
		linkedID = resolvedID
		if linkErr := qtx.LinkAvvikActivity(r.Context(), db.LinkAvvikActivityParams{AvvikID: id, ActivityID: linkedID}); linkErr != nil {
			return fmt.Errorf("link activity: %w", linkErr)
		}
		return h.addEventTx(r.Context(), qtx, id, "activity_linked", map[string]any{"activity_id": linkedID.String()})
	})
	if err != nil {
		http.Error(w, "failed to link activity", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/avvik/"+id.String()+"?tab=activities", http.StatusSeeOther)
}

func (h *Handler) resolveActivityToLink(ctx context.Context, qtx db.Querier, avvikID uuid.UUID, r *http.Request) (uuid.UUID, error) {
	if aid := strings.TrimSpace(r.FormValue("activity_id")); aid != "" {
		parsed, parseErr := uuid.Parse(aid)
		if parseErr != nil {
			return uuid.UUID{}, fmt.Errorf("parse activity_id: %w", parseErr)
		}
		return parsed, nil
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = "Oppfølging avvik #" + avvikID.String()[:8]
	}

	owner := strings.TrimSpace(r.FormValue("owner"))
	assigneeID := parseNullUUID(r.FormValue("assignee_id"))
	if assigneeID.Valid {
		if u, userErr := qtx.GetUserByID(ctx, assigneeID.UUID); userErr == nil {
			owner = u.Name
		}
	}

	created, createErr := qtx.CreateActivity(ctx, db.CreateActivityParams{
		MeasureID:    parseNullUUID(r.FormValue("measure_id")),
		Title:        title,
		Description:  strings.TrimSpace(r.FormValue("description")),
		ActivityType: defaultIfEmpty(strings.TrimSpace(r.FormValue("activity_type")), "one_off"),
		Recurrence:   defaultIfEmpty(strings.TrimSpace(r.FormValue("recurrence")), "none"),
		Priority:     defaultIfEmpty(strings.TrimSpace(r.FormValue("priority")), "medium"),
		Kind:         defaultIfEmpty(strings.TrimSpace(r.FormValue("kind")), "review"),
		Owner:        owner,
		AssigneeID:   assigneeID,
		DueDate:      parseNullTimeDate(r.FormValue("due_date")),
	})
	if createErr != nil {
		return uuid.UUID{}, fmt.Errorf("create activity: %w", createErr)
	}
	return created.ID, nil
}

func (h *Handler) UnlinkActivity(w http.ResponseWriter, r *http.Request) {
	h.unlinkByPath(w, r, "aid", "activity", func(qtx db.Querier, avvikID, targetID uuid.UUID) error {
		if unlinkErr := qtx.UnlinkAvvikActivity(r.Context(), db.UnlinkAvvikActivityParams{AvvikID: avvikID, ActivityID: targetID}); unlinkErr != nil {
			return fmt.Errorf("unlink activity: %w", unlinkErr)
		}
		return nil
	})
}

func (h *Handler) UnlinkMeasure(w http.ResponseWriter, r *http.Request) {
	h.unlinkByPath(w, r, "mid", "measure", func(qtx db.Querier, avvikID, targetID uuid.UUID) error {
		if unlinkErr := qtx.UnlinkAvvikMeasure(r.Context(), db.UnlinkAvvikMeasureParams{AvvikID: avvikID, MeasureID: targetID}); unlinkErr != nil {
			return fmt.Errorf("unlink measure: %w", unlinkErr)
		}
		return nil
	})
}

func (h *Handler) AddNotification(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	rawAudience := strings.TrimSpace(r.FormValue("audience"))
	if rejectEnum(w, "audience", rawAudience, "reporter, management, organisation, exposed_employees, datatilsynet, nve, finanstilsynet, bors", allowedAudience) {
		return
	}
	audience := defaultIfEmpty(rawAudience, "organisation")
	err := h.withTx(r.Context(), func(qtx db.Querier) error {
		userID, _ := currentUser(r.Context())
		if _, addErr := qtx.AddAvvikNotification(r.Context(), db.AddAvvikNotificationParams{
			AvvikID:  id,
			Audience: audience,
			SentAt:   time.Now().UTC(),
			SentBy:   userID,
			Notes:    strings.TrimSpace(r.FormValue("notes")),
		}); addErr != nil {
			return fmt.Errorf("add notification: %w", addErr)
		}
		return h.addEventTx(r.Context(), qtx, id, "notification_sent", map[string]any{"audience": audience})
	})
	if err != nil {
		http.Error(w, "failed to add notification", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/avvik/"+id.String()+"?tab=notifications", http.StatusSeeOther)
}

func (h *Handler) AddAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(w, r, "id")
	if !ok {
		return
	}
	current, err := h.q.GetAvvik(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	user, _ := middleware.FromContext(r.Context())
	if !h.canReporterOrCreatorAccess(r.Context(), current, user) && user.Role != "admin" {
		httputil.Forbidden(w, r)
		return
	}
	if parseErr := r.ParseForm(); parseErr != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	filename := strings.TrimSpace(r.FormValue("filename"))
	link := strings.TrimSpace(r.FormValue("storage_key"))
	if filename == "" || link == "" {
		http.Error(w, "filename and link required", http.StatusBadRequest)
		return
	}
	if urlErr := httputil.ValidateURL(link); urlErr != nil {
		http.Error(w, "invalid URL: "+urlErr.Error(), http.StatusBadRequest)
		return
	}
	err = h.withTx(r.Context(), func(qtx db.Querier) error {
		userID, _ := currentUser(r.Context())
		if _, addErr := qtx.AddAvvikAttachment(r.Context(), db.AddAvvikAttachmentParams{
			AvvikID:    id,
			Filename:   filename,
			StorageKey: link,
			UploadedBy: userID,
		}); addErr != nil {
			return fmt.Errorf("add attachment: %w", addErr)
		}
		return h.addEventTx(r.Context(), qtx, id, "evidence_added", map[string]any{"filename": filename, "storage_key": link})
	})
	if err != nil {
		http.Error(w, "failed to add attachment", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/avvik/"+id.String()+"?tab=evidence", http.StatusSeeOther)
}

func (h *Handler) UpdateClosureFlags(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	params := db.UpdateAvvikClosureFlagsParams{
		ID:                        id,
		LogQaDone:                 r.FormValue("log_qa_done") == "on",
		FollowupsDelegated:        r.FormValue("followups_delegated") == "on",
		ReporterInformed:          r.FormValue("reporter_informed") == "on",
		OrgInformed:               r.FormValue("org_informed") == "on",
		MgmtInformed:              r.FormValue("mgmt_informed") == "on",
		DecisionsAnchored:         r.FormValue("decisions_anchored") == "on",
		ImplementationDeadlineSet: r.FormValue("implementation_deadline_set") == "on",
	}
	err := h.withTx(r.Context(), func(qtx db.Querier) error {
		if _, updateErr := qtx.UpdateAvvikClosureFlags(r.Context(), params); updateErr != nil {
			return fmt.Errorf("update closure flags: %w", updateErr)
		}
		return h.addEventTx(r.Context(), qtx, id, "status_changed", map[string]any{"checklist_updated": true})
	})
	if err != nil {
		http.Error(w, "failed to update closure flags", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/avvik/"+id.String()+"?tab=closure", http.StatusSeeOther)
}

func (h *Handler) Close(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	a, err := h.q.GetAvvik(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !canCloseAvvik(a) {
		http.Error(w, "closure checklist incomplete", http.StatusBadRequest)
		return
	}
	err = h.withTx(r.Context(), func(qtx db.Querier) error {
		if _, updateErr := qtx.UpdateAvvikStatus(r.Context(), db.UpdateAvvikStatusParams{
			ID:             id,
			Status:         "closed",
			ClosedAt:       sql.NullTime{Time: time.Now().UTC(), Valid: true},
			ClosureSummary: strings.TrimSpace(r.FormValue("closure_summary")),
			RootCause:      strings.TrimSpace(r.FormValue("root_cause")),
			LessonsLearned: strings.TrimSpace(r.FormValue("lessons_learned")),
		}); updateErr != nil {
			return fmt.Errorf("close avvik: %w", updateErr)
		}
		return h.addEventTx(r.Context(), qtx, id, "closed", map[string]any{"closed": true})
	})
	if err != nil {
		http.Error(w, "failed to close avvik", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/avvik/"+id.String()+"?tab=closure", http.StatusSeeOther)
}

func (h *Handler) withTx(ctx context.Context, fn func(db.Querier) error) error {
	tx, err := h.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	qtx := db.New(tx)
	if err := fn(qtx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (h *Handler) addEventTx(ctx context.Context, qtx db.Querier, avvikID uuid.UUID, eventType string, payload map[string]any) error {
	userID, userName := currentUser(ctx)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}
	_, err = qtx.AddAvvikEvent(ctx, db.AddAvvikEventParams{
		AvvikID:    avvikID,
		ActorID:    userID,
		ActorLabel: userName,
		EventType:  eventType,
		Payload:    body,
		OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

func (h *Handler) unlinkRelationship(ctx context.Context, avvikID, targetID uuid.UUID, kind string, unlinkFn func(db.Querier) error) error {
	return h.withTx(ctx, func(qtx db.Querier) error {
		if err := unlinkFn(qtx); err != nil {
			return err
		}
		eventType := "measure_linked"
		payloadKey := "measure_id"
		if kind == "activity" {
			eventType = "activity_linked"
			payloadKey = "activity_id"
		}
		return h.addEventTx(ctx, qtx, avvikID, eventType, map[string]any{payloadKey: targetID.String(), "action": "unlinked"})
	})
}

func (h *Handler) unlinkByPath(
	w http.ResponseWriter,
	r *http.Request,
	targetPathKey, kind string,
	unlinkFn func(db.Querier, uuid.UUID, uuid.UUID) error,
) {
	avvikID, ok := h.pathUUID(w, r, "id")
	if !ok {
		return
	}
	targetID, ok := h.pathUUID(w, r, targetPathKey)
	if !ok {
		return
	}
	err := h.unlinkRelationship(r.Context(), avvikID, targetID, kind, func(qtx db.Querier) error {
		return unlinkFn(qtx, avvikID, targetID)
	})
	if err != nil {
		http.Error(w, "failed to unlink "+kind, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func currentUser(ctx context.Context) (uuid.NullUUID, string) {
	u, ok := middleware.FromContext(ctx)
	if !ok {
		return uuid.NullUUID{}, ""
	}
	id, err := uuid.Parse(u.ID)
	if err != nil {
		return uuid.NullUUID{}, u.Name
	}
	return uuid.NullUUID{UUID: id, Valid: true}, u.Name
}

func (h *Handler) pathUUID(w http.ResponseWriter, r *http.Request, key string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(key))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return uuid.UUID{}, false
	}
	return id, true
}

func nullString(v string) sql.NullString {
	v = strings.TrimSpace(v)
	return sql.NullString{String: v, Valid: v != ""}
}

func nullBoolFromQuery(v string) sql.NullBool {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "true", "1", "yes", "on":
		return sql.NullBool{Bool: true, Valid: true}
	case "false", "0", "no", "off":
		return sql.NullBool{Bool: false, Valid: true}
	default:
		return sql.NullBool{}
	}
}

func parseNullUUID(v string) uuid.NullUUID {
	id, err := uuid.Parse(strings.TrimSpace(v))
	if err != nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: id, Valid: true}
}

func parseDateOrNow(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Now().UTC()
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t.UTC()
	}
	slog.Warn("invalid date input, using now")
	return time.Now().UTC()
}

func parseNullTimeRFC3339(v string) sql.NullTime {
	v = strings.TrimSpace(v)
	if v == "" {
		return sql.NullTime{}
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}

func parseNullTimeDate(v string) sql.NullTime {
	v = strings.TrimSpace(v)
	if v == "" {
		return sql.NullTime{}
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}

func (h *Handler) resolveReporter(ctx context.Context, name, email string) (string, string) {
	if name == "" || email != "" {
		return name, email
	}
	users, err := h.q.ListUsers(ctx)
	if err != nil {
		return name, email
	}
	for _, u := range users {
		if strings.EqualFold(name, u.Name) || strings.EqualFold(name, u.Email) || strings.EqualFold(name, u.Name+" ("+u.Email+")") {
			return u.Name, u.Email
		}
	}
	return name, email
}

func (h *Handler) resolveUserLookupAssignee(ctx context.Context, rawID, lookup string) uuid.NullUUID {
	if parsed := parseNullUUID(rawID); parsed.Valid {
		return parsed
	}
	lookup = strings.TrimSpace(lookup)
	if lookup == "" {
		return uuid.NullUUID{}
	}
	users, err := h.q.ListUsers(ctx)
	if err != nil {
		return uuid.NullUUID{}
	}
	for _, u := range users {
		display := strings.TrimSpace(u.Name + " (" + u.Email + ")")
		if strings.EqualFold(lookup, display) || strings.EqualFold(lookup, u.Name) || strings.EqualFold(lookup, u.Email) {
			return uuid.NullUUID{UUID: u.ID, Valid: true}
		}
	}
	return uuid.NullUUID{}
}

func (h *Handler) canReporterOrCreatorAccess(ctx context.Context, a db.Avvik, user middleware.SessionUser) bool {
	if user.Role == "admin" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(a.ReporterEmail), strings.TrimSpace(user.Email)) && strings.TrimSpace(user.Email) != "" {
		return true
	}
	uid, err := uuid.Parse(user.ID)
	if err != nil {
		return false
	}
	events, listErr := h.q.ListAvvikEvents(ctx, a.ID)
	if listErr != nil {
		return false
	}
	for _, e := range events {
		if e.EventType == "created" && e.ActorID.Valid && e.ActorID.UUID == uid {
			return true
		}
	}
	return false
}

func defaultIfEmpty(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
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

func canCloseAvvik(a db.Avvik) bool {
	ok := a.LogQaDone &&
		a.FollowupsDelegated &&
		a.ReporterInformed &&
		a.OrgInformed &&
		a.DecisionsAnchored &&
		a.ImplementationDeadlineSet
	if !ok {
		return false
	}
	if a.RiskLevel == "high" {
		return a.MgmtInformed
	}
	return true
}
