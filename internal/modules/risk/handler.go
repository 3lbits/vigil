// Package risk provides NS 5814-based risk management with wizard flows.
package risk

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/audit"
	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/middleware"
	risktemplates "github.com/3lbits/vigil/internal/modules/risk/templates"
	"github.com/3lbits/vigil/internal/ui/layout"
)

type Handler struct {
	q db.Querier
}

func NewHandler(q db.Querier, engine *authz.Engine) *Handler {
	_ = engine
	return &Handler{q: q}
}

// ── Assessment list ──────────────────────────────────────────────────────────

func (h *Handler) listAssessments(r *http.Request, user middleware.SessionUser) ([]db.RiskAssessment, error) {
	if user.Role == "admin" || user.Role == "editor" {
		rows, err := h.q.ListRiskAssessments(r.Context())
		return rows, err //nolint:wrapcheck
	}
	userID, _ := uuid.Parse(user.ID)
	rows, err := h.q.ListRiskAssessmentsForUser(r.Context(), uuid.NullUUID{UUID: userID, Valid: true})
	return rows, err //nolint:wrapcheck
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.FromContext(r.Context())
	flash := r.URL.Query().Get("flash")
	flashType := r.URL.Query().Get("type")

	assessments, err := h.listAssessments(r, user)
	if err != nil {
		slog.Error("list risk assessments", "error", err)
		http.Error(w, "failed to load assessments", http.StatusInternalServerError)
		return
	}

	vms := make([]risktemplates.AssessmentListVM, 0, len(assessments))
	for _, a := range assessments {
		risks, _ := h.q.ListRisksForAssessment(r.Context(), a.ID)
		var red int64
		for _, ri := range risks {
			if ri.LikelihoodCurrent.Valid && ri.ConsequenceCurrent.Valid {
				if int(ri.LikelihoodCurrent.Int32)*int(ri.ConsequenceCurrent.Int32) >= 12 {
					red++
				}
			}
		}
		vms = append(vms, risktemplates.AssessmentListVM{
			Assessment: a,
			RiskCount:  int64(len(risks)),
			RedCount:   red,
		})
	}

	httputil.Render(w, r, layout.Layout("Risk assessments", "NS 5814 risk assessments", "risk", user,
		risktemplates.AssessmentList(risktemplates.AssessmentListPageVM{
			Assessments:   vms,
			Flash:         flash,
			FlashType:     flashType,
			CurrentUserID: user.ID,
		}),
	))
}

func (h *Handler) RiskRegister(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.FromContext(r.Context())
	gs, _ := h.q.GetRiskGlobalSettings(r.Context())
	lowMax, highMin := thresholdDefaults(int(gs.LowMax), int(gs.HighMin))

	var risks []db.ListAllRisksRow
	if user.Role == "admin" || user.Role == "editor" {
		var err error
		risks, err = h.q.ListAllRisks(r.Context())
		if err != nil {
			slog.Error("list all risks", "error", err)
			http.Error(w, "failed to load risks", http.StatusInternalServerError)
			return
		}
	} else {
		userID, _ := uuid.Parse(user.ID)
		rows, err := h.q.ListAllRisksForUser(r.Context(), userID)
		if err != nil {
			slog.Error("list risks for user", "error", err)
			http.Error(w, "failed to load risks", http.StatusInternalServerError)
			return
		}
		risks = make([]db.ListAllRisksRow, len(rows))
		for i, row := range rows {
			risks[i] = db.ListAllRisksRow(row)
		}
	}

	httputil.Render(w, r, layout.Layout("Risk register", "All risks across assessments", "risk-register", user,
		risktemplates.RiskRegister(risks, lowMax, highMin),
	))
}

// ── Wizard Step 1: Framework ─────────────────────────────────────────────────

func (h *Handler) NewAssessment(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.FromContext(r.Context())
	users, _ := h.q.ListUsers(r.Context())
	orgs, _ := h.q.ListOrganizations(r.Context())
	gs, _ := h.q.GetRiskGlobalSettings(r.Context())
	httputil.Render(w, r, layout.Layout("New risk assessment", "Step 1 — Framework", "risk", user,
		risktemplates.WizardStep1(risktemplates.WizardStep1VM{
			Assessment:         db.RiskAssessment{Type: "security"},
			Users:              users,
			Orgs:               orgs,
			IsNew:              true,
			AcceptanceCriteria: gs.AcceptanceCriteria,
		}),
	))
}

func (h *Handler) CreateAssessment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := middleware.FromContext(r.Context())

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		h.renderStep1Error(w, r, user, db.RiskAssessment{}, true, "Assessment name is required.")
		return
	}

	var createdBy uuid.NullUUID
	if uid, err := uuid.Parse(user.ID); err == nil {
		createdBy = uuid.NullUUID{UUID: uid, Valid: true}
	}

	assessment, err := h.q.CreateRiskAssessment(r.Context(), db.CreateRiskAssessmentParams{
		Name:               name,
		Scope:              strings.TrimSpace(r.FormValue("scope")),
		SecurityObjectives: strings.TrimSpace(r.FormValue("security_objectives")),
		Type:               "security",
		RiskOwnerID:        nullUUID(r.FormValue("risk_owner_id")),
		OrgID:              nullUUID(r.FormValue("org_id")),
		CreatedBy:          createdBy,
	})
	if err != nil {
		slog.Error("create risk assessment", "error", err)
		h.renderStep1Error(w, r, user, db.RiskAssessment{Name: name}, true, "Failed to create assessment.")
		return
	}

	h.saveParticipants(r, assessment.ID)

	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.create",
		Attrs: map[string]any{"assessment_id": assessment.ID.String(), "name": name},
	})

	http.Redirect(w, r, "/risks/"+assessment.ID.String()+"/step/2", http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

func (h *Handler) saveParticipants(r *http.Request, assessmentID uuid.UUID) {
	if err := h.q.ClearAssessmentParticipants(r.Context(), assessmentID); err != nil {
		slog.Warn("clear assessment participants", "assessment_id", assessmentID, "error", err)
	}
	for _, idStr := range r.Form["participant_ids"] {
		if uid, err := uuid.Parse(idStr); err == nil {
			if err := h.q.AddAssessmentParticipant(r.Context(), db.AddAssessmentParticipantParams{
				AssessmentID: assessmentID,
				UserID:       uid,
			}); err != nil {
				slog.Warn("add assessment participant", "assessment_id", assessmentID, "user_id", uid.String(), "error", err) //nolint:gosec // uid.String() is always hex+hyphens from uuid.Parse; no injection risk
			}
		}
	}
}

func (h *Handler) renderStep1Error(w http.ResponseWriter, r *http.Request, user middleware.SessionUser, a db.RiskAssessment, isNew bool, msg string) {
	users, _ := h.q.ListUsers(r.Context())
	orgs, _ := h.q.ListOrganizations(r.Context())
	gs, _ := h.q.GetRiskGlobalSettings(r.Context())
	httputil.Render(w, r, layout.Layout("New risk assessment", "Step 1 — Framework", "risk", user,
		risktemplates.WizardStep1(risktemplates.WizardStep1VM{
			Assessment:         a,
			Users:              users,
			Orgs:               orgs,
			IsNew:              isNew,
			Flash:              msg,
			AcceptanceCriteria: gs.AcceptanceCriteria,
		}),
	))
}

// ── Wizard Step 2: Identify ──────────────────────────────────────────────────

func (h *Handler) Step2(w http.ResponseWriter, r *http.Request) {
	a, risks, ok := h.loadAssessmentWithRisks(w, r, "step2")
	if !ok {
		return
	}
	h.renderStep2(w, r, a, risks, false)
}

func (h *Handler) SaveStep2(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	review := strings.Contains(r.URL.Path, "/review/")
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	risks, err := h.q.ListRisksForAssessment(r.Context(), assessment.ID)
	if err != nil {
		slog.Error("list risks for step2 save", "error", err)
		http.Error(w, "failed to load risks", http.StatusInternalServerError)
		return
	}
	h.updateStep2Risks(r, risks)
	_ = h.q.UpdateRiskAssessmentStep(r.Context(), db.UpdateRiskAssessmentStepParams{
		ID:          assessment.ID,
		CurrentStep: maxStep(assessment.CurrentStep, 3),
		Status:      assessment.Status,
	})
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.step2_saved",
		Attrs: map[string]any{"assessment_id": assessment.ID.String(), "name": assessment.Name},
	})
	step2Action := strings.TrimSpace(r.FormValue("step2_action"))
	http.Redirect(w, r, step2Path(assessment.ID, review, step2Action == "continue"), http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

func (h *Handler) updateStep2Risks(r *http.Request, risks []db.Risk) {
	for _, risk := range risks {
		id := risk.ID.String()
		name := strings.TrimSpace(r.FormValue("name_" + id))
		if name == "" {
			name = risk.Name
		}
		if _, err := h.q.UpdateRiskIdentification(r.Context(), db.UpdateRiskIdentificationParams{
			ID:          risk.ID,
			Name:        name,
			Description: strings.TrimSpace(r.FormValue("description_" + id)),
		}); err != nil {
			slog.Error("update risk identification", "risk_id", risk.ID, "error", err)
		}
		h.syncStep2RiskAssets(r, risk, id)
	}
}

func (h *Handler) syncStep2RiskAssets(r *http.Request, risk db.Risk, id string) {
	rawAssetIDs, ok := r.Form["asset_ids_"+id]
	if !ok {
		return
	}
	if err := h.q.ClearRiskAssets(r.Context(), risk.ID); err != nil {
		slog.Error("clear risk assets", "risk_id", risk.ID, "error", err)
	}
	for _, assetID := range parseUUIDs(rawAssetIDs) {
		if err := h.q.LinkRiskToAsset(r.Context(), db.LinkRiskToAssetParams{
			RiskID:  risk.ID,
			AssetID: assetID,
		}); err != nil {
			slog.Error("link asset to risk", "risk_id", risk.ID, "asset_id", assetID, "error", err)
		}
	}
}

// ── Risk CRUD (HTMX) ─────────────────────────────────────────────────────────

func (h *Handler) AddRisk(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	risk, err := h.q.CreateRisk(r.Context(), db.CreateRiskParams{
		AssessmentID: assessment.ID,
		Name:         name,
		Description:  description,
	})
	if err != nil {
		slog.Error("create risk", "error", err)
		http.Error(w, "failed to create risk", http.StatusInternalServerError)
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.risk.create",
		Attrs: map[string]any{"risk_id": risk.ID.String(), "name": name, "assessment_id": assessment.ID.String()},
	})
	if r.Header.Get("HX-Request") != "true" {
		httputil.RedirectWithFlash(w, r, "/risks/"+assessment.ID.String()+"/step/2", "Risk added.", "success")
		return
	}
	httputil.Render(w, r, risktemplates.NewRiskRow(risk, assessment.ID.String()))
}

func (h *Handler) DeleteRisk(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risk, ok := h.loadRisk(w, r, assessment)
	if !ok {
		return
	}
	if err := h.q.DeleteRisk(r.Context(), risk.ID); err != nil {
		slog.Error("delete risk", "error", err)
		http.Error(w, "failed to delete risk", http.StatusInternalServerError)
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.risk.delete",
		Attrs: map[string]any{"risk_id": risk.ID.String(), "name": risk.Name, "assessment_id": assessment.ID.String()},
	})
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) DeleteRiskAndRedirect(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risk, ok := h.loadRisk(w, r, assessment)
	if !ok {
		return
	}
	if err := h.q.DeleteRisk(r.Context(), risk.ID); err != nil {
		slog.Error("delete risk", "error", err)
		httputil.RedirectWithFlash(w, r, "/risks/"+assessment.ID.String(), "Failed to delete risk.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.risk.delete",
		Attrs: map[string]any{"risk_id": risk.ID.String(), "name": risk.Name, "assessment_id": assessment.ID.String()},
	})
	httputil.RedirectWithFlash(w, r, "/risks/"+assessment.ID.String(), "Risk deleted.", "success")
}

// ── Wizard Step 3: Analyse ───────────────────────────────────────────────────

func (h *Handler) Step3(w http.ResponseWriter, r *http.Request) {
	a, risks, ok := h.loadAssessmentWithRisks(w, r, "step3")
	if !ok {
		return
	}
	gs, _ := h.q.GetRiskGlobalSettings(r.Context())
	lowMax, highMin := thresholdDefaults(int(gs.LowMax), int(gs.HighMin))
	scaleLabels, _ := h.q.ListRiskScaleLabels(r.Context())
	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("Risk analysis", "Step 3 — Analyse", "risk", user,
		risktemplates.WizardStep3(risktemplates.WizardStep3VM{Assessment: a, Risks: risks, LowMax: lowMax, HighMin: highMin, ScaleLabels: scaleLabels}),
	))
}

func (h *Handler) SaveStep3(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	risks, err := h.q.ListRisksForAssessment(r.Context(), assessment.ID)
	if err != nil {
		slog.Error("list risks for step3 save", "error", err)
		http.Error(w, "failed to load risks", http.StatusInternalServerError)
		return
	}
	for _, risk := range risks {
		id := risk.ID.String()
		if _, err := h.q.UpdateRiskCurrentScores(r.Context(), db.UpdateRiskCurrentScoresParams{
			ID:                   risk.ID,
			LikelihoodCurrent:    nullInt32(r.FormValue("current_likelihood_" + id)),
			ConsequenceCurrent:   nullInt32(r.FormValue("current_consequence_" + id)),
			LikelihoodReasoning:  strings.TrimSpace(r.FormValue("likelihood_reasoning_" + id)),
			ConsequenceReasoning: strings.TrimSpace(r.FormValue("consequence_reasoning_" + id)),
		}); err != nil {
			slog.Error("update risk current scores", "risk_id", risk.ID, "error", err)
		}
	}
	_ = h.q.UpdateRiskAssessmentStep(r.Context(), db.UpdateRiskAssessmentStepParams{
		ID:          assessment.ID,
		CurrentStep: maxStep(assessment.CurrentStep, 4),
		Status:      assessment.Status,
	})
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.step3_saved",
		Attrs: map[string]any{"assessment_id": assessment.ID.String(), "name": assessment.Name},
	})
	http.Redirect(w, r, decisionStartPath(assessment.ID, strings.Contains(r.URL.Path, "/review/")), http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

// ── Evaluation gate ──────────────────────────────────────────────────────────

func (h *Handler) Evaluate(w http.ResponseWriter, r *http.Request) {
	h.redirectDecisionStart(w, r, false)
}

func (h *Handler) SaveEvaluate(w http.ResponseWriter, r *http.Request) {
	h.redirectDecisionStart(w, r, strings.Contains(r.URL.Path, "/review/"))
}

// ── Wizard Step 4: Treat ─────────────────────────────────────────────────────

func (h *Handler) Step4(w http.ResponseWriter, r *http.Request) {
	h.redirectDecisionStart(w, r, false)
}

func (h *Handler) SaveStep4(w http.ResponseWriter, r *http.Request) {
	h.redirectDecisionStart(w, r, false)
}

// ── Acceptance loop ──────────────────────────────────────────────────────────

func (h *Handler) AcceptAssessment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	if assessment.Status != "pending_acceptance" {
		http.Error(w, "assessment is not pending acceptance", http.StatusBadRequest)
		return
	}
	if _, err := h.q.AcceptAssessment(ctx, assessment.ID); err != nil {
		slog.Error("accept assessment", "error", err)
		http.Error(w, "failed to accept", http.StatusInternalServerError)
		return
	}
	audit.RecordOrWarn(ctx, h.q, audit.Event{
		Event: "risk.assessment.accepted",
		Attrs: map[string]any{"id": assessment.ID.String(), "name": assessment.Name},
	})
	httputil.RedirectWithFlash(w, r, "/risks/"+assessment.ID.String(), "Assessment accepted and marked active.", "success")
}

func (h *Handler) DeclineAssessment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	if assessment.Status != "pending_acceptance" {
		http.Error(w, "assessment is not pending acceptance", http.StatusBadRequest)
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if _, err := h.q.DeclineAssessment(ctx, db.DeclineAssessmentParams{
		ID:             assessment.ID,
		AcceptanceNote: note,
	}); err != nil {
		slog.Error("decline assessment", "error", err)
		http.Error(w, "failed to decline", http.StatusInternalServerError)
		return
	}
	audit.RecordOrWarn(ctx, h.q, audit.Event{
		Event: "risk.assessment.declined",
		Attrs: map[string]any{"id": assessment.ID.String(), "name": assessment.Name, "note": note},
	})
	httputil.RedirectWithFlash(w, r, "/risks/"+assessment.ID.String(), "Assessment returned to draft.", "warning")
}

// ── SoA linkage (HTMX) ───────────────────────────────────────────────────────

func (h *Handler) SearchMeasures(w http.ResponseWriter, r *http.Request) {
	assessmentIDStr := r.PathValue("id")
	if _, err := uuid.Parse(assessmentIDStr); err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	riskIDStr := r.PathValue("rid")
	q := "%" + strings.TrimSpace(r.URL.Query().Get("q")) + "%"
	measures, err := h.q.SearchMeasures(r.Context(), q)
	if err != nil {
		slog.Error("search measures", "error", err)
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}
	httputil.Render(w, r, risktemplates.MeasureSearchResults(measures, riskIDStr, r.PathValue("id")))
}

func (h *Handler) LinkMeasure(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risk, ok := h.loadRisk(w, r, assessment)
	if !ok {
		return
	}
	measureID, err := uuid.Parse(r.PathValue("mid"))
	if err != nil {
		http.Error(w, "invalid measure id", http.StatusBadRequest)
		return
	}
	if linkErr := h.q.LinkRiskToMeasure(r.Context(), db.LinkRiskToMeasureParams{
		RiskID:    risk.ID,
		MeasureID: measureID,
		Note:      "",
	}); linkErr != nil {
		slog.Error("link measure to risk", "error", linkErr)
		http.Error(w, "link failed", http.StatusInternalServerError)
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.measure_linked",
		Attrs: map[string]any{"risk_id": risk.ID.String(), "measure_id": measureID.String(), "assessment_id": assessment.ID.String()},
	})
	if r.Header.Get("HX-Request") != "true" {
		httputil.RedirectWithFlash(w, r, decisionRiskPathFromRequest(r, assessment.ID, risk.ID), "Measure linked.", "success")
		return
	}
	m, err := h.q.GetMeasure(r.Context(), measureID)
	if err != nil {
		http.Error(w, "not found", http.StatusInternalServerError)
		return
	}
	httputil.Render(w, r, risktemplates.LinkedMeasureChip(m, risk.ID.String(), assessment.ID.String()))
}

func (h *Handler) UnlinkMeasure(w http.ResponseWriter, r *http.Request) { //nolint:dupl
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risk, ok := h.loadRisk(w, r, assessment)
	if !ok {
		return
	}
	measureID, err := uuid.Parse(r.PathValue("mid"))
	if err != nil {
		http.Error(w, "invalid measure id", http.StatusBadRequest)
		return
	}
	_ = h.q.UnlinkRiskFromMeasure(r.Context(), db.UnlinkRiskFromMeasureParams{
		RiskID:    risk.ID,
		MeasureID: measureID,
	})
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.measure_unlinked",
		Attrs: map[string]any{"risk_id": risk.ID.String(), "measure_id": measureID.String(), "assessment_id": assessment.ID.String()},
	})
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) UnlinkMeasureAndRedirect(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risk, ok := h.loadRisk(w, r, assessment)
	if !ok {
		return
	}
	measureID, err := uuid.Parse(r.PathValue("mid"))
	if err != nil {
		http.Error(w, "invalid measure id", http.StatusBadRequest)
		return
	}
	_ = h.q.UnlinkRiskFromMeasure(r.Context(), db.UnlinkRiskFromMeasureParams{
		RiskID:    risk.ID,
		MeasureID: measureID,
	})
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.measure_unlinked",
		Attrs: map[string]any{"risk_id": risk.ID.String(), "measure_id": measureID.String(), "assessment_id": assessment.ID.String()},
	})
	httputil.RedirectWithFlash(w, r, decisionRiskPathFromRequest(r, assessment.ID, risk.ID), "Measure unlinked.", "success")
}

func (h *Handler) SearchActivities(w http.ResponseWriter, r *http.Request) {
	riskIDStr := r.PathValue("rid")
	q := "%" + strings.TrimSpace(r.URL.Query().Get("q")) + "%"
	activities, err := h.q.SearchActivities(r.Context(), q)
	if err != nil {
		slog.Error("search activities", "error", err)
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}
	httputil.Render(w, r, risktemplates.ActivitySearchResults(activities, riskIDStr, r.PathValue("id")))
}

func (h *Handler) LinkActivity(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risk, ok := h.loadRisk(w, r, assessment)
	if !ok {
		return
	}
	activityID, err := uuid.Parse(r.PathValue("aid"))
	if err != nil {
		http.Error(w, "invalid activity id", http.StatusBadRequest)
		return
	}
	if linkErr := h.q.LinkRiskToActivity(r.Context(), db.LinkRiskToActivityParams{
		RiskID:     risk.ID,
		ActivityID: activityID,
	}); linkErr != nil {
		slog.Error("link activity to risk", "error", linkErr)
		http.Error(w, "link failed", http.StatusInternalServerError)
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.activity_linked",
		Attrs: map[string]any{"risk_id": risk.ID.String(), "activity_id": activityID.String(), "assessment_id": assessment.ID.String()},
	})
	if r.Header.Get("HX-Request") != "true" {
		httputil.RedirectWithFlash(w, r, decisionRiskPathFromRequest(r, assessment.ID, risk.ID), "Activity linked.", "success")
		return
	}
	row, err := h.q.GetActivity(r.Context(), activityID)
	if err != nil {
		http.Error(w, "not found", http.StatusInternalServerError)
		return
	}
	a := db.Activity{ID: row.ID, Title: row.Title, ActivityType: row.ActivityType}
	httputil.Render(w, r, risktemplates.LinkedActivityChip(a, risk.ID.String(), assessment.ID.String()))
}

func (h *Handler) UnlinkActivity(w http.ResponseWriter, r *http.Request) { //nolint:dupl
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risk, ok := h.loadRisk(w, r, assessment)
	if !ok {
		return
	}
	activityID, err := uuid.Parse(r.PathValue("aid"))
	if err != nil {
		http.Error(w, "invalid activity id", http.StatusBadRequest)
		return
	}
	_ = h.q.UnlinkRiskFromActivity(r.Context(), db.UnlinkRiskFromActivityParams{
		RiskID:     risk.ID,
		ActivityID: activityID,
	})
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.activity_unlinked",
		Attrs: map[string]any{"risk_id": risk.ID.String(), "activity_id": activityID.String(), "assessment_id": assessment.ID.String()},
	})
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) UnlinkActivityAndRedirect(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risk, ok := h.loadRisk(w, r, assessment)
	if !ok {
		return
	}
	activityID, err := uuid.Parse(r.PathValue("aid"))
	if err != nil {
		http.Error(w, "invalid activity id", http.StatusBadRequest)
		return
	}
	_ = h.q.UnlinkRiskFromActivity(r.Context(), db.UnlinkRiskFromActivityParams{
		RiskID:     risk.ID,
		ActivityID: activityID,
	})
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.activity_unlinked",
		Attrs: map[string]any{"risk_id": risk.ID.String(), "activity_id": activityID.String(), "assessment_id": assessment.ID.String()},
	})
	httputil.RedirectWithFlash(w, r, decisionRiskPathFromRequest(r, assessment.ID, risk.ID), "Activity unlinked.", "success")
}

func (h *Handler) QuickCreateMeasure(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risk, ok := h.loadRisk(w, r, assessment)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if parseErr := r.ParseForm(); parseErr != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	owner, assigneeID := resolveMeasureAssignee(r, h.q)
	var dueDate sql.NullTime
	if d := strings.TrimSpace(r.FormValue("due_date")); d != "" {
		if t, parseErr := time.Parse("2006-01-02", d); parseErr == nil {
			dueDate = sql.NullTime{Time: t, Valid: true}
		}
	}
	m, err := h.q.CreateMeasure(r.Context(), db.CreateMeasureParams{
		Name:        name,
		Description: strings.TrimSpace(r.FormValue("description")),
		Category:    strings.TrimSpace(r.FormValue("category")),
		Owner:       owner,
		AssigneeID:  assigneeID,
		Status:      "planned",
	})
	if err != nil {
		slog.Error("quick create measure", "error", err)
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}
	if linkErr := h.q.LinkRiskToMeasure(r.Context(), db.LinkRiskToMeasureParams{
		RiskID:    risk.ID,
		MeasureID: m.ID,
	}); linkErr != nil {
		slog.Error("link measure to risk", "error", linkErr)
		http.Error(w, "link failed", http.StatusInternalServerError)
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.measure_created",
		Attrs: map[string]any{"measure_id": m.ID.String(), "name": name, "risk_id": risk.ID.String(), "assessment_id": assessment.ID.String()},
	})
	var linkedActivity *db.Activity
	if dueDate.Valid {
		a, actErr := h.q.CreateActivity(r.Context(), db.CreateActivityParams{
			MeasureID:    uuid.NullUUID{UUID: m.ID, Valid: true},
			Title:        "Implement: " + name,
			ActivityType: "one_off",
			Recurrence:   "none",
			Priority:     "medium",
			Kind:         "task",
			Owner:        owner,
			DueDate:      dueDate,
		})
		if actErr != nil {
			slog.Error("quick create activity for measure", "error", actErr)
		} else {
			linkedActivity = &a
			_ = h.q.LinkRiskToActivity(r.Context(), db.LinkRiskToActivityParams{
				RiskID:     risk.ID,
				ActivityID: a.ID,
			})
		}
	}
	h.renderQuickMeasureResponse(w, r, assessment.ID, risk.ID, m, linkedActivity)
}

func (h *Handler) renderQuickMeasureResponse(
	w http.ResponseWriter,
	r *http.Request,
	assessmentID uuid.UUID,
	riskID uuid.UUID,
	measure db.Measure,
	activity *db.Activity,
) {
	if r.Header.Get("HX-Request") != "true" {
		httputil.RedirectWithFlash(w, r, decisionRiskPathFromRequest(r, assessmentID, riskID), "Measure created and linked.", "success")
		return
	}
	if activity != nil {
		httputil.Render(w, r, risktemplates.LinkedMeasureWithActivity(measure, *activity, riskID.String(), assessmentID.String()))
		return
	}
	httputil.Render(w, r, risktemplates.LinkedMeasureChip(measure, riskID.String(), assessmentID.String()))
}

// ── Assessment detail ────────────────────────────────────────────────────────

type detailData struct {
	currentCounts  map[[2]int]int
	targetCounts   map[[2]int]int
	riskMeasures   map[uuid.UUID][]db.Measure
	riskActivities map[uuid.UUID][]db.Activity
}

func (h *Handler) buildDetailData(r *http.Request, risks []db.Risk) detailData {
	d := detailData{
		currentCounts:  make(map[[2]int]int),
		targetCounts:   make(map[[2]int]int),
		riskMeasures:   make(map[uuid.UUID][]db.Measure),
		riskActivities: make(map[uuid.UUID][]db.Activity),
	}
	for _, ri := range risks {
		if ri.LikelihoodCurrent.Valid && ri.ConsequenceCurrent.Valid {
			d.currentCounts[[2]int{int(ri.LikelihoodCurrent.Int32), int(ri.ConsequenceCurrent.Int32)}]++
		}
		if ri.LikelihoodTarget.Valid && ri.ConsequenceTarget.Valid {
			d.targetCounts[[2]int{int(ri.LikelihoodTarget.Int32), int(ri.ConsequenceTarget.Int32)}]++
		}
		d.riskMeasures[ri.ID], _ = h.q.ListMeasuresForRisk(r.Context(), ri.ID)
		d.riskActivities[ri.ID], _ = h.q.ListActivitiesForRisk(r.Context(), ri.ID)
	}
	return d
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risks, err := h.q.ListRisksForAssessment(r.Context(), assessment.ID)
	if err != nil {
		slog.Error("list risks for detail", "error", err)
		http.Error(w, "failed to load risks", http.StatusInternalServerError)
		return
	}
	d := h.buildDetailData(r, risks)
	var ownerName, orgName string
	if assessment.RiskOwnerID.Valid {
		if u, err := h.q.GetUserByID(r.Context(), assessment.RiskOwnerID.UUID); err == nil {
			ownerName = u.Name
		}
	}
	if assessment.OrgID.Valid {
		if o, err := h.q.GetOrganization(r.Context(), assessment.OrgID.UUID); err == nil {
			orgName = o.Name
		}
	}
	gs, _ := h.q.GetRiskGlobalSettings(r.Context())
	lowMax, highMin := thresholdDefaults(int(gs.LowMax), int(gs.HighMin))
	thresholds := layout.RiskThresholds{LowMax: lowMax, HighMin: highMin}
	participants, _ := h.q.ListParticipantsForAssessment(r.Context(), assessment.ID)
	auditLog, _ := h.q.ListAuditLogForAssessment(r.Context(), assessment.ID.String())
	flash := r.URL.Query().Get("flash")
	flashType := r.URL.Query().Get("type")
	user, _ := middleware.FromContext(r.Context())
	canToggle := user.Role == "admin" || assessment.RiskOwnerID.UUID.String() == user.ID || assessment.CreatedBy.UUID.String() == user.ID
	httputil.Render(w, r, layout.Layout(assessmentPageTitle(assessment), "Risk assessment", "risk", user,
		risktemplates.AssessmentDetail(risktemplates.AssessmentDetailVM{
			Assessment:      assessment,
			Risks:           risks,
			CurrentMatrix:   layout.BuildRiskMatrixCellsT(d.currentCounts, thresholds),
			TargetMatrix:    layout.BuildRiskMatrixCellsT(d.targetCounts, thresholds),
			RiskMeasures:    d.riskMeasures,
			RiskActivities:  d.riskActivities,
			OwnerName:       ownerName,
			OrgName:         orgName,
			Participants:    participants,
			Flash:           flash,
			FlashType:       flashType,
			LowMax:          lowMax,
			HighMin:         highMin,
			CurrentUserID:   user.ID,
			CanTogglePublic: canToggle,
			AuditLog:        auditLog,
		}),
	))
}

func (h *Handler) RiskDetail(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	riskID, err := uuid.Parse(r.PathValue("rid"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	risk, err := h.q.GetRisk(r.Context(), riskID)
	if err != nil || risk.AssessmentID != assessment.ID {
		http.NotFound(w, r)
		return
	}
	measures, _ := h.q.ListMeasuresForRisk(r.Context(), risk.ID)
	activities, _ := h.q.ListActivitiesForRisk(r.Context(), risk.ID)
	assets, _ := h.q.ListAssetsForRisk(r.Context(), risk.ID)
	reviewEvents, _ := h.q.ListRiskReassessmentEvents(r.Context(), db.ListRiskReassessmentEventsParams{
		RiskID:  risk.ID,
		SinceAt: risk.AssessedAt,
	})
	auditLog, _ := h.q.ListAuditLogForRisk(r.Context(), risk.ID.String())
	assessedByName := ""
	if risk.AssessedBy.Valid {
		if u, userErr := h.q.GetUserByID(r.Context(), risk.AssessedBy.UUID); userErr == nil {
			assessedByName = u.Name
		}
	}
	assetQuery := strings.TrimSpace(r.URL.Query().Get("asset_q"))
	assetResults, _ := h.q.SearchAssetsForRisk(r.Context(), db.SearchAssetsForRiskParams{
		Q:          assetQuery,
		RiskID:     risk.ID,
		LimitCount: 20,
	})
	if assetQuery == "" {
		assetResults = nil
	}
	gs, _ := h.q.GetRiskGlobalSettings(r.Context())
	lowMax, highMin := thresholdDefaults(int(gs.LowMax), int(gs.HighMin))
	user, _ := middleware.FromContext(r.Context())
	reviewRationale := risk.AssessmentRationale
	reassessMessage := r.URL.Query().Get("reassess_flash")
	reassessError := r.URL.Query().Get("reassess_type") == "error"
	httputil.Render(w, r, layout.Layout(risk.Name, "Risk", "risk", user,
		risktemplates.RiskDetail(risktemplates.RiskDetailVM{
			Assessment:          assessment,
			Risk:                risk,
			Measures:            measures,
			Activities:          activities,
			Assets:              assets,
			AssetQuery:          assetQuery,
			AssetResults:        assetResults,
			LowMax:              lowMax,
			HighMin:             highMin,
			ReviewEvents:        reviewEvents,
			AuditLog:            auditLog,
			AssessedByName:      assessedByName,
			ReviewRationale:     reviewRationale,
			ReassessmentMessage: reassessMessage,
			ReassessmentError:   reassessError,
		}),
	))
}

func (h *Handler) ReassessRisk(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risk, ok := h.loadRisk(w, r, assessment)
	if !ok {
		return
	}
	http.Redirect(w, r, "/risks/"+assessment.ID.String()+"/risks/"+risk.ID.String(), http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

func (h *Handler) SubmitRiskReassessment(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risk, ok := h.loadRisk(w, r, assessment)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	likelihood := nullInt32(r.FormValue("assessed_likelihood"))
	consequence := nullInt32(r.FormValue("assessed_consequence"))
	if !likelihood.Valid || !consequence.Valid {
		http.Redirect(w, r, "/risks/"+assessment.ID.String()+"/risks/"+risk.ID.String()+"?reassess_flash=Reassessment+requires+both+likelihood+and+consequence+scores.&reassess_type=error", http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
		return
	}
	user, _ := middleware.FromContext(r.Context())
	assessedBy := uuid.NullUUID{}
	if uid, err := uuid.Parse(user.ID); err == nil {
		assessedBy = uuid.NullUUID{UUID: uid, Valid: true}
	}
	rationale := strings.TrimSpace(r.FormValue("assessment_rationale"))
	updated, err := h.q.ReassessRiskCurrentScores(r.Context(), db.ReassessRiskCurrentScoresParams{
		ID:                  risk.ID,
		LikelihoodCurrent:   likelihood,
		ConsequenceCurrent:  consequence,
		AssessmentRationale: rationale,
		AssessedBy:          assessedBy,
	})
	if err != nil {
		slog.Error("reassess risk current scores", "risk_id", risk.ID, "error", err)
		http.Redirect(w, r, "/risks/"+assessment.ID.String()+"/risks/"+risk.ID.String()+"?reassess_flash=Failed+to+save+reassessment.&reassess_type=error", http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.risk.reassessed",
		Attrs: map[string]any{
			"assessment_id": assessment.ID.String(),
			"risk_id":       risk.ID.String(),
			"likelihood":    updated.LikelihoodCurrent.Int32,
			"consequence":   updated.ConsequenceCurrent.Int32,
		},
	})
	http.Redirect(w, r, "/risks/"+assessment.ID.String()+"/risks/"+risk.ID.String()+"?reassess_flash=Risk+reassessment+saved.&reassess_type=success", http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

func (h *Handler) AddRiskAsset(w http.ResponseWriter, r *http.Request) {
	h.updateRiskAsset(w, r, true)
}

func (h *Handler) RemoveRiskAsset(w http.ResponseWriter, r *http.Request) {
	h.updateRiskAsset(w, r, false)
}

func (h *Handler) updateRiskAsset(w http.ResponseWriter, r *http.Request, add bool) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risk, ok := h.loadRisk(w, r, assessment)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	assetID, err := uuid.Parse(r.PathValue("aid"))
	if err != nil {
		http.Error(w, "invalid asset id", http.StatusBadRequest)
		return
	}
	action := "linked"
	opErr := h.q.LinkRiskToAsset(r.Context(), db.LinkRiskToAssetParams{
		RiskID:  risk.ID,
		AssetID: assetID,
	})
	if !add {
		action = "unlinked"
		opErr = h.q.UnlinkRiskFromAsset(r.Context(), db.UnlinkRiskFromAssetParams{
			RiskID:  risk.ID,
			AssetID: assetID,
		})
	}
	if opErr != nil {
		slog.Error("update risk asset link", "error", opErr)
		http.Error(w, "failed to update risk asset link", http.StatusInternalServerError)
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.risk.asset_" + action,
		Attrs: map[string]any{"assessment_id": assessment.ID.String(), "risk_id": risk.ID.String(), "asset_id": assetID.String()},
	})
	target := "/risks/" + assessment.ID.String() + "/risks/" + risk.ID.String()
	if strings.TrimSpace(r.FormValue("return_to")) == "step2" {
		review := r.FormValue("review") == "1" || strings.Contains(r.Header.Get("Referer"), "/review/step/2")
		target = step2Path(assessment.ID, review, false)
		values := url.Values{}
		if q := strings.TrimSpace(r.FormValue("asset_q")); q != "" {
			values.Set("asset_q", q)
		}
		if rid := strings.TrimSpace(r.FormValue("asset_risk_id")); rid != "" {
			values.Set("asset_risk_id", rid)
		}
		if encoded := values.Encode(); encoded != "" {
			target += "?" + encoded
		}
		http.Redirect(w, r, target, http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
		return
	}
	if q := strings.TrimSpace(r.FormValue("asset_q")); q != "" {
		target += "?asset_q=" + url.QueryEscape(q)
	}
	http.Redirect(w, r, target, http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

func (h *Handler) DeleteAssessment(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	if err := h.q.DeleteRiskAssessment(r.Context(), assessment.ID); err != nil {
		slog.Error("delete risk assessment", "error", err)
		httputil.RedirectWithFlash(w, r, "/risks", "Failed to delete assessment.", "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.delete",
		Attrs: map[string]any{"id": assessment.ID.String(), "name": assessment.Name},
	})
	httputil.RedirectWithFlash(w, r, "/risks", "Assessment deleted.", "success")
}

func (h *Handler) TogglePublic(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	user, _ := middleware.FromContext(r.Context())
	if !canToggleAssessmentVisibility(assessment, user) {
		httputil.Forbidden(w, r)
		return
	}
	updated, err := h.q.ToggleRiskAssessmentPublic(r.Context(), assessment.ID)
	if err != nil {
		slog.Error("toggle public", "error", err)
		httputil.RedirectWithFlash(w, r, "/risks/"+assessment.ID.String(), "Failed to update visibility.", "error")
		return
	}
	vis := "private"
	if updated.IsPublic {
		vis = "public"
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.visibility",
		Attrs: map[string]any{"id": assessment.ID.String(), "visibility": vis},
	})
	httputil.RedirectWithFlash(w, r, "/risks/"+assessment.ID.String(), "Visibility updated.", "success")
}

// ── Review mode ──────────────────────────────────────────────────────────────

func (h *Handler) ReviewStart(w http.ResponseWriter, r *http.Request) {
	a, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	http.Redirect(w, r, "/risks/"+a.ID.String()+"/review/step/1", http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

func (h *Handler) ReviewStep1(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	users, _ := h.q.ListUsers(r.Context())
	orgs, _ := h.q.ListOrganizations(r.Context())
	gs, _ := h.q.GetRiskGlobalSettings(r.Context())
	participants, _ := h.q.ListParticipantsForAssessment(r.Context(), assessment.ID)
	participantQuery := strings.TrimSpace(r.URL.Query().Get("participant_q"))
	assetQuery := strings.TrimSpace(r.URL.Query().Get("asset_q"))
	assets, _ := h.q.ListAssetsForAssessment(r.Context(), assessment.ID)
	assetResults, _ := h.q.SearchAssetsToLink(r.Context(), db.SearchAssetsToLinkParams{
		AssessmentID: assessment.ID,
		Q:            assetQuery,
		LimitCount:   20,
	})
	if assetQuery == "" {
		assetResults = nil
	}
	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("Review: Framework", "Review step 1", "risk", user,
		risktemplates.WizardStep1(risktemplates.WizardStep1VM{
			Assessment:         assessment,
			Users:              users,
			Orgs:               orgs,
			Participants:       participants,
			ParticipantQuery:   participantQuery,
			ParticipantResults: findParticipantResults(users, participants, participantQuery, 20),
			Assets:             assets,
			AssetQuery:         assetQuery,
			AssetResults:       assetResults,
			IsNew:              false,
			Flash:              r.URL.Query().Get("flash"),
			AcceptanceCriteria: gs.AcceptanceCriteria,
		}),
	))
}

func (h *Handler) SaveReviewStep1(w http.ResponseWriter, r *http.Request) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/risks/"+assessment.ID.String()+"/review/step/1?flash=Name+required", http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
		return
	}
	_, err := h.q.UpdateRiskAssessmentStep1(r.Context(), db.UpdateRiskAssessmentStep1Params{
		ID:                 assessment.ID,
		Name:               name,
		Scope:              strings.TrimSpace(r.FormValue("scope")),
		SecurityObjectives: strings.TrimSpace(r.FormValue("security_objectives")),
		Type:               assessment.Type,
		RiskOwnerID:        nullUUID(r.FormValue("risk_owner_id")),
		OrgID:              nullUUID(r.FormValue("org_id")),
	})
	if err != nil {
		slog.Error("update risk assessment step1", "error", err)
		http.Redirect(w, r, "/risks/"+assessment.ID.String()+"/review/step/1?flash=Save+failed", http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.review_updated",
		Attrs: map[string]any{"assessment_id": assessment.ID.String(), "name": name},
	})
	http.Redirect(w, r, "/risks/"+assessment.ID.String()+"/step/2", http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

func (h *Handler) AddAssessmentParticipant(w http.ResponseWriter, r *http.Request) {
	h.updateAssessmentParticipant(w, r, true)
}

func (h *Handler) RemoveAssessmentParticipant(w http.ResponseWriter, r *http.Request) {
	h.updateAssessmentParticipant(w, r, false)
}

func (h *Handler) updateAssessmentParticipant(w http.ResponseWriter, r *http.Request, add bool) { //nolint:dupl
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	userID, err := uuid.Parse(r.PathValue("uid"))
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	if add {
		if err := h.q.AddAssessmentParticipant(r.Context(), db.AddAssessmentParticipantParams{
			AssessmentID: assessment.ID,
			UserID:       userID,
		}); err != nil {
			slog.Error("add assessment participant", "assessment_id", assessment.ID, "error", err)
			http.Error(w, "failed to add participant", http.StatusInternalServerError)
			return
		}
		audit.RecordOrWarn(r.Context(), h.q, audit.Event{
			Event: "risk.assessment.participant_added",
			Attrs: map[string]any{"assessment_id": assessment.ID.String(), "user_id": userID.String()},
		})
	} else {
		if err := h.q.RemoveAssessmentParticipant(r.Context(), db.RemoveAssessmentParticipantParams{
			AssessmentID: assessment.ID,
			UserID:       userID,
		}); err != nil {
			slog.Error("remove assessment participant", "assessment_id", assessment.ID, "error", err)
			http.Error(w, "failed to remove participant", http.StatusInternalServerError)
			return
		}
		audit.RecordOrWarn(r.Context(), h.q, audit.Event{
			Event: "risk.assessment.participant_removed",
			Attrs: map[string]any{"assessment_id": assessment.ID.String(), "user_id": userID.String()},
		})
	}
	http.Redirect(w, r, reviewStep1URLWithQuery(assessment.ID, r.FormValue("participant_q"), r.FormValue("asset_q")), http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

func findParticipantResults(users, participants []db.User, query string, limit int) []db.User {
	if query == "" || limit <= 0 {
		return nil
	}
	selected := make(map[uuid.UUID]struct{}, len(participants))
	for _, p := range participants {
		selected[p.ID] = struct{}{}
	}
	q := strings.ToLower(query)
	results := make([]db.User, 0, limit)
	for _, u := range users {
		if _, exists := selected[u.ID]; exists {
			continue
		}
		if strings.Contains(strings.ToLower(u.Name), q) || strings.Contains(strings.ToLower(u.Email), q) {
			results = append(results, u)
			if len(results) >= limit {
				break
			}
		}
	}
	return results
}

func (h *Handler) AddAssessmentAsset(w http.ResponseWriter, r *http.Request) {
	h.updateAssessmentAsset(w, r, true)
}

func (h *Handler) RemoveAssessmentAsset(w http.ResponseWriter, r *http.Request) {
	h.updateAssessmentAsset(w, r, false)
}

func (h *Handler) updateAssessmentAsset(w http.ResponseWriter, r *http.Request, add bool) { //nolint:dupl
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	assetID, err := uuid.Parse(r.PathValue("aid"))
	if err != nil {
		http.Error(w, "invalid asset id", http.StatusBadRequest)
		return
	}
	if add {
		if err := h.q.AddAssetToAssessment(r.Context(), db.AddAssetToAssessmentParams{
			AssessmentID: assessment.ID,
			AssetID:      assetID,
		}); err != nil {
			slog.Error("add assessment asset", "assessment_id", assessment.ID, "error", err)
			http.Error(w, "failed to add asset", http.StatusInternalServerError)
			return
		}
		audit.RecordOrWarn(r.Context(), h.q, audit.Event{
			Event: "risk.assessment.asset_added",
			Attrs: map[string]any{"assessment_id": assessment.ID.String(), "asset_id": assetID.String()},
		})
	} else {
		if err := h.q.RemoveAssetFromAssessment(r.Context(), db.RemoveAssetFromAssessmentParams{
			AssessmentID: assessment.ID,
			AssetID:      assetID,
		}); err != nil {
			slog.Error("remove assessment asset", "assessment_id", assessment.ID, "error", err)
			http.Error(w, "failed to remove asset", http.StatusInternalServerError)
			return
		}
		audit.RecordOrWarn(r.Context(), h.q, audit.Event{
			Event: "risk.assessment.asset_removed",
			Attrs: map[string]any{"assessment_id": assessment.ID.String(), "asset_id": assetID.String()},
		})
	}
	http.Redirect(w, r, reviewStep1URLWithQuery(assessment.ID, r.FormValue("participant_q"), r.FormValue("asset_q")), http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

func reviewStep1URLWithQuery(assessmentID uuid.UUID, participantQ, assetQ string) string {
	base := "/risks/" + assessmentID.String() + "/review/step/1"
	values := url.Values{}
	if q := strings.TrimSpace(participantQ); q != "" {
		values.Set("participant_q", q)
	}
	if q := strings.TrimSpace(assetQ); q != "" {
		values.Set("asset_q", q)
	}
	if len(values) == 0 {
		return base
	}
	return base + "?" + values.Encode()
}

func parseUUIDs(raw []string) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(strings.TrimSpace(s))
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}

func buildRiskAssets(risks []db.Risk, listFn func(uuid.UUID) ([]db.Asset, error)) map[uuid.UUID][]db.Asset {
	out := make(map[uuid.UUID][]db.Asset, len(risks))
	for _, risk := range risks {
		assets, err := listFn(risk.ID)
		if err != nil {
			continue
		}
		out[risk.ID] = assets
	}
	return out
}

func (h *Handler) ReviewStep2(w http.ResponseWriter, r *http.Request) {
	a, risks, ok := h.loadAssessmentWithRisks(w, r, "review-step2")
	if !ok {
		return
	}
	h.renderStep2(w, r, a, risks, true)
}

func (h *Handler) step2AssetSearch(r *http.Request) (map[uuid.UUID]string, map[uuid.UUID][]db.Asset) {
	queries := map[uuid.UUID]string{}
	results := map[uuid.UUID][]db.Asset{}

	q := strings.TrimSpace(r.URL.Query().Get("asset_q"))
	riskIDRaw := strings.TrimSpace(r.URL.Query().Get("asset_risk_id"))
	if q == "" || riskIDRaw == "" {
		return queries, results
	}
	riskID, err := uuid.Parse(riskIDRaw)
	if err != nil {
		return queries, results
	}
	queries[riskID] = q
	assets, err := h.q.SearchAssetsForRisk(r.Context(), db.SearchAssetsForRiskParams{
		Q:          q,
		RiskID:     riskID,
		LimitCount: 25,
	})
	if err != nil {
		return queries, results
	}
	results[riskID] = assets
	return queries, results
}

func (h *Handler) renderStep2(w http.ResponseWriter, r *http.Request, assessment db.RiskAssessment, risks []db.Risk, isReview bool) {
	users, _ := h.q.ListUsers(r.Context())
	linkedAssets := buildRiskAssets(risks, func(riskID uuid.UUID) ([]db.Asset, error) {
		return h.q.ListAssetsForRisk(r.Context(), riskID)
	})
	assetQueries, assetResults := h.step2AssetSearch(r)
	if r.Header.Get("HX-Request") == "true" && r.URL.Query().Get("asset_search_partial") == "1" {
		riskID, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("asset_risk_id")))
		if err != nil {
			http.Error(w, "invalid risk id", http.StatusBadRequest)
			return
		}
		httputil.Render(w, r, risktemplates.Step2AssetSearchResults(
			assessment.ID.String(),
			riskID.String(),
			isReview,
			assetQueries[riskID],
			assetResults[riskID],
		))
		return
	}
	user, _ := middleware.FromContext(r.Context())
	title := "Risk identification"
	subtitle := "Step 2 — Identify"
	if isReview {
		title = "Review: Identify"
		subtitle = "Review step 2"
	}
	httputil.Render(w, r, layout.Layout(title, subtitle, "risk", user,
		risktemplates.WizardStep2(risktemplates.WizardStep2VM{
			Assessment:   assessment,
			Risks:        risks,
			Users:        users,
			LinkedAssets: linkedAssets,
			AssetQueries: assetQueries,
			AssetResults: assetResults,
			IsReview:     isReview,
		}),
	))
}

func (h *Handler) ReviewStep3(w http.ResponseWriter, r *http.Request) {
	a, risks, ok := h.loadAssessmentWithRisks(w, r, "review-step3")
	if !ok {
		return
	}
	gs, _ := h.q.GetRiskGlobalSettings(r.Context())
	lowMax, highMin := thresholdDefaults(int(gs.LowMax), int(gs.HighMin))
	scaleLabels, _ := h.q.ListRiskScaleLabels(r.Context())
	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("Review: Analyse", "Review step 3", "risk", user,
		risktemplates.WizardStep3(risktemplates.WizardStep3VM{Assessment: a, Risks: risks, IsReview: true, LowMax: lowMax, HighMin: highMin, ScaleLabels: scaleLabels}),
	))
}

func (h *Handler) ReviewEvaluate(w http.ResponseWriter, r *http.Request) {
	h.redirectDecisionStart(w, r, true)
}

func (h *Handler) ReviewStep4(w http.ResponseWriter, r *http.Request) {
	h.redirectDecisionStart(w, r, true)
}

func (h *Handler) SaveReviewStep4(w http.ResponseWriter, r *http.Request) {
	h.redirectDecisionStart(w, r, true)
}

func (h *Handler) DecisionStart(w http.ResponseWriter, r *http.Request) {
	h.redirectDecisionStart(w, r, false)
}

func (h *Handler) ReviewDecisionStart(w http.ResponseWriter, r *http.Request) {
	h.redirectDecisionStart(w, r, true)
}

func (h *Handler) DecisionRisk(w http.ResponseWriter, r *http.Request) {
	h.renderDecisionRisk(w, r, false)
}

func (h *Handler) ReviewDecisionRisk(w http.ResponseWriter, r *http.Request) {
	h.renderDecisionRisk(w, r, true)
}

func (h *Handler) SaveDecisionRisk(w http.ResponseWriter, r *http.Request) {
	h.saveDecisionRisk(w, r, false)
}

func (h *Handler) SaveReviewDecisionRisk(w http.ResponseWriter, r *http.Request) {
	h.saveDecisionRisk(w, r, true)
}

func (h *Handler) redirectDecisionStart(w http.ResponseWriter, r *http.Request, review bool) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return
	}
	risks, err := h.q.ListRisksForAssessment(r.Context(), assessment.ID)
	if err != nil {
		slog.Error("list risks for decision start", "error", err)
		http.Error(w, "failed to load risks", http.StatusInternalServerError)
		return
	}
	if len(risks) == 0 {
		httputil.RedirectWithFlash(w, r, "/risks/"+assessment.ID.String()+"/step/2", "No risks found. Add risks first.", "warning")
		return
	}
	decisionRisks := decisionEligibleRisks(risks)
	if len(decisionRisks) == 0 {
		httputil.RedirectWithFlash(w, r, "/risks/"+assessment.ID.String(), "No scored risks to treat. Add current scores in analysis first.", "warning")
		return
	}
	http.Redirect(w, r, decisionRiskPath(assessment.ID, decisionRisks[0].ID, review), http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

func (h *Handler) renderDecisionRisk(w http.ResponseWriter, r *http.Request, review bool) {
	assessment, risk, idx, allRisks, ok := h.loadDecisionContext(w, r)
	if !ok {
		return
	}
	measures, _ := h.q.ListMeasuresForRisk(r.Context(), risk.ID)
	activities, _ := h.q.ListActivitiesForRisk(r.Context(), risk.ID)
	assets, _ := h.q.ListAssetsForRisk(r.Context(), risk.ID)
	users, _ := h.q.ListUsers(r.Context())
	gs, _ := h.q.GetRiskGlobalSettings(r.Context())
	lowMax, highMin := thresholdDefaults(int(gs.LowMax), int(gs.HighMin))
	if risk.RiskDecision == "" {
		risk.RiskDecision = defaultDecisionForRisk(risk, lowMax)
	}
	if riskCanBeAccepted(risk, lowMax) {
		risk.RiskDecision = "accept"
	}
	overdue := false
	for _, a := range activities {
		if a.Status != "done" && a.DueDate.Valid && a.DueDate.Time.Before(time.Now()) {
			overdue = true
			break
		}
	}
	var prevID uuid.NullUUID
	if idx > 0 {
		prevID = uuid.NullUUID{UUID: allRisks[idx-1].ID, Valid: true}
	}
	var nextID uuid.NullUUID
	if idx+1 < len(allRisks) {
		nextID = uuid.NullUUID{UUID: allRisks[idx+1].ID, Valid: true}
	}
	user, _ := middleware.FromContext(r.Context())
	title := "Risk decision"
	subtitle := "Treat one risk at a time"
	if review {
		title = "Review: Risk decision"
		subtitle = "Review decisions one risk at a time"
	}
	ownerName := ""
	if assessment.RiskOwnerID.Valid {
		if u, err := h.q.GetUserByID(r.Context(), assessment.RiskOwnerID.UUID); err == nil {
			ownerName = u.Name
		}
	}
	httputil.Render(w, r, layout.Layout(title, subtitle, "risk", user,
		risktemplates.WizardDecision(risktemplates.DecisionVM{
			Assessment:         assessment,
			Risk:               risk,
			Risks:              allRisks,
			Assets:             assets,
			Measures:           measures,
			Activities:         activities,
			Users:              users,
			Overdue:            overdue,
			AcceptanceCriteria: gs.AcceptanceCriteria,
			LowMax:             lowMax,
			HighMin:            highMin,
			Position:           idx + 1,
			Total:              len(allRisks),
			PrevRiskID:         prevID,
			NextRiskID:         nextID,
			IsReview:           review,
			CanAccept:          riskCanBeAccepted(risk, lowMax),
			OwnerName:          ownerName,
		}),
	))
}

func (h *Handler) saveDecisionRisk(w http.ResponseWriter, r *http.Request, review bool) {
	assessment, risk, idx, allRisks, ok := h.loadDecisionContext(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	decision := strings.TrimSpace(r.FormValue("decision"))
	decisionNotes := strings.TrimSpace(r.FormValue("decision_notes"))
	targetLikelihood := nullInt32(r.FormValue("target_likelihood_" + risk.ID.String()))
	targetConsequence := nullInt32(r.FormValue("target_consequence_" + risk.ID.String()))
	gs, _ := h.q.GetRiskGlobalSettings(r.Context())
	lowMax, _ := thresholdDefaults(int(gs.LowMax), int(gs.HighMin))
	if decision == "" {
		decision = defaultDecisionForRisk(risk, lowMax)
	}
	isAdvance := r.FormValue("next") == "1" || r.FormValue("submit_assessment") == "1"
	if !h.validateDecisionChoice(w, r, assessment.ID, risk, decision, decisionNotes, review, isAdvance) {
		return
	}
	if err := h.persistDecision(w, r, risk, decision, decisionNotes, targetLikelihood, targetConsequence); err != nil {
		return
	}
	if r.FormValue("submit_assessment") == "1" {
		h.completeDecisionFlow(w, r, assessment, review)
		return
	}
	if r.FormValue("next") != "1" {
		http.Redirect(w, r, decisionRiskPath(assessment.ID, risk.ID, review), http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
		return
	}
	if idx+1 < len(allRisks) {
		http.Redirect(w, r, decisionRiskPath(assessment.ID, allRisks[idx+1].ID, review), http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
		return
	}
	http.Redirect(w, r, decisionRiskPath(assessment.ID, risk.ID, review), http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

func (h *Handler) validateDecisionChoice(
	w http.ResponseWriter,
	r *http.Request,
	assessmentID uuid.UUID,
	risk db.Risk,
	decision string,
	decisionNotes string,
	review bool,
	isAdvance bool,
) bool {
	if decision == "document" && decisionNotes == "" {
		httputil.RedirectWithFlash(w, r, decisionRiskPath(assessmentID, risk.ID, review), "Documentation decision requires notes.", "error")
		return false
	}
	if decision == "accept" {
		gs, _ := h.q.GetRiskGlobalSettings(r.Context())
		lowMax, _ := thresholdDefaults(int(gs.LowMax), int(gs.HighMin))
		if !riskCanBeAccepted(risk, lowMax) {
			httputil.RedirectWithFlash(w, r, decisionRiskPath(assessmentID, risk.ID, review), "Accept is only allowed when current risk is low (green).", "error")
			return false
		}
	}
	if riskCanBeAccepted(risk, lowMaxFromContext(r, h.q)) && decision != "accept" {
		httputil.RedirectWithFlash(w, r, decisionRiskPath(assessmentID, risk.ID, review), "Green risks can only be accepted.", "error")
		return false
	}
	if decision != "treat" || !isAdvance {
		return true
	}
	measures, err := h.q.ListMeasuresForRisk(r.Context(), risk.ID)
	if err != nil {
		slog.Error("list measures for decision validation", "error", err)
		http.Error(w, "failed to validate decision", http.StatusInternalServerError)
		return false
	}
	if len(measures) == 0 {
		httputil.RedirectWithFlash(w, r, decisionRiskPath(assessmentID, risk.ID, review), "Treat requires at least one linked measure before continuing.", "error")
		return false
	}
	return true
}

func (h *Handler) persistDecision(
	w http.ResponseWriter,
	r *http.Request,
	risk db.Risk,
	decision string,
	decisionNotes string,
	targetLikelihood sql.NullInt32,
	targetConsequence sql.NullInt32,
) error {
	if decision != "accept" && decision != "treat" && decision != "document" {
		return nil
	}
	if err := h.q.UpdateRiskDecision(r.Context(), db.UpdateRiskDecisionParams{
		ID:            risk.ID,
		RiskDecision:  decision,
		DecisionNotes: decisionNotes,
	}); err != nil {
		slog.Error("update risk decision", "error", err)
		http.Error(w, "failed to update decision", http.StatusInternalServerError)
		return fmt.Errorf("update risk decision: %w", err)
	}
	if decision == "accept" || decision == "document" {
		if err := h.q.UpdateRiskTargetScore(r.Context(), db.UpdateRiskTargetScoreParams{
			ID:                risk.ID,
			LikelihoodTarget:  risk.LikelihoodCurrent,
			ConsequenceTarget: risk.ConsequenceCurrent,
		}); err != nil {
			slog.Error("set target score to current", "error", err)
			http.Error(w, "failed to update target score", http.StatusInternalServerError)
			return fmt.Errorf("set target score to current: %w", err)
		}
		return nil
	}
	if decision != "treat" {
		return nil
	}
	if !targetLikelihood.Valid || !targetConsequence.Valid {
		return nil
	}
	if err := h.q.UpdateRiskTargetScore(r.Context(), db.UpdateRiskTargetScoreParams{
		ID:                risk.ID,
		LikelihoodTarget:  targetLikelihood,
		ConsequenceTarget: targetConsequence,
	}); err != nil {
		slog.Error("update risk target score", "error", err)
		http.Error(w, "failed to update target score", http.StatusInternalServerError)
		return fmt.Errorf("update risk target score: %w", err)
	}
	return nil
}

func (h *Handler) completeDecisionFlow(w http.ResponseWriter, r *http.Request, assessment db.RiskAssessment, review bool) {
	if review {
		_ = h.q.UpdateRiskAssessmentReviewed(r.Context(), assessment.ID)
		audit.RecordOrWarn(r.Context(), h.q, audit.Event{
			Event: "risk.assessment.reviewed",
			Attrs: map[string]any{"id": assessment.ID.String(), "name": assessment.Name},
		})
		httputil.RedirectWithFlash(w, r, "/risks/"+assessment.ID.String(), "Review complete.", "success")
		return
	}
	_ = h.q.UpdateRiskAssessmentStep(r.Context(), db.UpdateRiskAssessmentStepParams{
		ID:          assessment.ID,
		CurrentStep: 4,
		Status:      "pending_acceptance",
	})
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "risk.assessment.submitted_for_acceptance",
		Attrs: map[string]any{"id": assessment.ID.String(), "name": assessment.Name},
	})
	httputil.RedirectWithFlash(w, r, "/risks/"+assessment.ID.String(), "Assessment submitted — awaiting risk owner acceptance.", "success")
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func decisionStartPath(assessmentID uuid.UUID, review bool) string {
	if review {
		return "/risks/" + assessmentID.String() + "/review/decision"
	}
	return "/risks/" + assessmentID.String() + "/decision"
}

func decisionRiskPath(assessmentID, riskID uuid.UUID, review bool) string {
	if review {
		return "/risks/" + assessmentID.String() + "/review/decision/" + riskID.String()
	}
	return "/risks/" + assessmentID.String() + "/decision/" + riskID.String()
}

func step2Path(assessmentID uuid.UUID, review, shouldContinue bool) string {
	if shouldContinue {
		if review {
			return "/risks/" + assessmentID.String() + "/review/step/3"
		}
		return "/risks/" + assessmentID.String() + "/step/3"
	}
	if review {
		return "/risks/" + assessmentID.String() + "/review/step/2"
	}
	return "/risks/" + assessmentID.String() + "/step/2"
}

func decisionRiskPathFromRequest(r *http.Request, assessmentID, riskID uuid.UUID) string {
	ref := r.Header.Get("Referer")
	if ref != "" {
		if u, err := url.Parse(ref); err == nil && strings.Contains(u.Path, "/review/decision/") {
			return decisionRiskPath(assessmentID, riskID, true)
		}
	}
	return decisionRiskPath(assessmentID, riskID, false)
}

func (h *Handler) loadDecisionContext(w http.ResponseWriter, r *http.Request) (db.RiskAssessment, db.Risk, int, []db.Risk, bool) {
	assessment, ok := h.loadAssessment(w, r)
	if !ok {
		return db.RiskAssessment{}, db.Risk{}, 0, nil, false
	}
	riskID, err := uuid.Parse(r.PathValue("rid"))
	if err != nil {
		http.Error(w, "invalid risk id", http.StatusBadRequest)
		return db.RiskAssessment{}, db.Risk{}, 0, nil, false
	}
	risks, err := h.q.ListRisksForAssessment(r.Context(), assessment.ID)
	if err != nil {
		slog.Error("list risks for decision context", "error", err)
		http.Error(w, "failed to load risks", http.StatusInternalServerError)
		return db.RiskAssessment{}, db.Risk{}, 0, nil, false
	}
	decisionRisks := decisionEligibleRisks(risks)
	if len(decisionRisks) == 0 {
		httputil.RedirectWithFlash(w, r, "/risks/"+assessment.ID.String(), "No scored risks to treat. Add current scores in analysis first.", "warning")
		return db.RiskAssessment{}, db.Risk{}, 0, nil, false
	}
	for i, risk := range decisionRisks {
		if risk.ID == riskID {
			return assessment, risk, i, decisionRisks, true
		}
	}
	http.Redirect(w, r, decisionRiskPath(assessment.ID, decisionRisks[0].ID, strings.Contains(r.URL.Path, "/review/")), http.StatusSeeOther) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
	return db.RiskAssessment{}, db.Risk{}, 0, nil, false
}

func (h *Handler) loadAssessmentWithRisks(w http.ResponseWriter, r *http.Request, errCtx string) (db.RiskAssessment, []db.Risk, bool) {
	a, ok := h.loadAssessment(w, r)
	if !ok {
		return db.RiskAssessment{}, nil, false
	}
	risks, err := h.q.ListRisksForAssessment(r.Context(), a.ID)
	if err != nil {
		slog.Error("list risks for "+errCtx, "error", err)
		http.Error(w, "failed to load risks", http.StatusInternalServerError)
		return db.RiskAssessment{}, nil, false
	}
	return a, risks, true
}

func (h *Handler) loadAssessment(w http.ResponseWriter, r *http.Request) (db.RiskAssessment, bool) {
	if a, ok := assessmentFromContext(r.Context()); ok {
		return a, true
	}
	id, ok := assessmentIDFromPath(w, r)
	if !ok {
		return db.RiskAssessment{}, false
	}
	a, err := h.q.GetRiskAssessment(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return db.RiskAssessment{}, false
	}
	return a, true
}

func assessmentIDFromPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid assessment id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

// loadRisk fetches the risk by {rid} and asserts it belongs to the given assessment.
// Returns 404 if the risk doesn't exist or belongs to a different assessment.
func (h *Handler) loadRisk(w http.ResponseWriter, r *http.Request, assessment db.RiskAssessment) (db.Risk, bool) {
	rid, err := uuid.Parse(r.PathValue("rid"))
	if err != nil {
		http.Error(w, "invalid risk id", http.StatusBadRequest)
		return db.Risk{}, false
	}
	risk, err := h.q.GetRisk(r.Context(), rid)
	if err != nil || risk.AssessmentID != assessment.ID {
		http.NotFound(w, r)
		return db.Risk{}, false
	}
	return risk, true
}

func nullUUID(s string) uuid.NullUUID {
	if id, err := uuid.Parse(strings.TrimSpace(s)); err == nil {
		return uuid.NullUUID{UUID: id, Valid: true}
	}
	return uuid.NullUUID{}
}

func nullInt32(s string) sql.NullInt32 {
	if s == "" {
		return sql.NullInt32{}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 5 {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(n), Valid: true} //nolint:gosec // n is range-checked to 1-5 above
}

func maxStep(current, next int32) int32 {
	if next > current {
		return next
	}
	return current
}

// thresholdDefaults returns low/high with NS 5814 fallback when DB returns zeroes.
func thresholdDefaults(lowMax, highMin int) (int, int) {
	if lowMax == 0 {
		return 5, 12
	}
	return lowMax, highMin
}

func canToggleAssessmentVisibility(assessment db.RiskAssessment, user middleware.SessionUser) bool {
	if user.Role == "admin" {
		return true
	}
	userID, err := uuid.Parse(user.ID)
	if err != nil {
		return false
	}
	return (assessment.RiskOwnerID.Valid && assessment.RiskOwnerID.UUID == userID) ||
		(assessment.CreatedBy.Valid && assessment.CreatedBy.UUID == userID)
}

func riskCanBeAccepted(risk db.Risk, lowMax int) bool {
	if !risk.LikelihoodCurrent.Valid || !risk.ConsequenceCurrent.Valid {
		return false
	}
	score := int(risk.LikelihoodCurrent.Int32) * int(risk.ConsequenceCurrent.Int32)
	return score <= lowMax
}

func defaultDecisionForRisk(risk db.Risk, lowMax int) string {
	if riskCanBeAccepted(risk, lowMax) {
		return "accept"
	}
	return "treat"
}

func decisionEligibleRisks(risks []db.Risk) []db.Risk {
	filtered := make([]db.Risk, 0, len(risks))
	for _, risk := range risks {
		if riskHasCurrentScore(risk) {
			filtered = append(filtered, risk)
		}
	}
	return filtered
}

func riskHasCurrentScore(risk db.Risk) bool {
	return risk.LikelihoodCurrent.Valid && risk.ConsequenceCurrent.Valid
}

func lowMaxFromContext(r *http.Request, q db.Querier) int {
	gs, _ := q.GetRiskGlobalSettings(r.Context())
	lowMax, _ := thresholdDefaults(int(gs.LowMax), int(gs.HighMin))
	return lowMax
}

func resolveMeasureAssignee(r *http.Request, q db.Querier) (owner string, assigneeID uuid.NullUUID) {
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

func assessmentPageTitle(a db.RiskAssessment) string {
	if a.RefNum.Valid {
		return fmt.Sprintf("RA-%03d · %s", a.RefNum.Int32, a.Name)
	}
	return a.Name
}
