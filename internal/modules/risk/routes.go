package risk

import (
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/modregistry"
)

type riskModule struct{}

func New() modregistry.Module {
	return riskModule{}
}

func (riskModule) Name() string {
	return "risk"
}

func (riskModule) Register(deps modregistry.Dependencies, r *modregistry.Registrar) error {
	h := NewHandler(deps.Queries, deps.Authz)
	moduleGuard := middleware.RequireModule("risk")
	readPolicy := modregistry.Policy{Resource: "risk", Action: "read"}
	writePolicy := modregistry.Policy{Resource: "risk", Action: "write"}
	deletePolicy := modregistry.Policy{Resource: "risk", Action: "delete"}
	acceptPolicy := modregistry.Policy{Resource: "risks", Action: "accept"}
	declinePolicy := modregistry.Policy{Resource: "risks", Action: "decline"}
	assessmentReadScoped := RequireRiskReadScoped(deps.Queries, deps.Authz)
	assessmentUpdateOwn := RequireRiskUpdateOwn(deps.Queries, deps.Authz)
	assessmentAcceptOwn := RequireRiskOwnerDecision("accept", deps.Queries, deps.Authz)
	assessmentDeclineOwn := RequireRiskOwnerDecision("decline", deps.Queries, deps.Authz)

	// Risk register (flat list of all risks)
	r.Guarded("GET /risk-register", readPolicy, h.RiskRegister, moduleGuard)

	// Risk assessments
	r.Guarded("GET /risks", readPolicy, h.List, moduleGuard)
	r.Guarded("GET /risks/new", writePolicy, h.NewAssessment, moduleGuard)
	r.Guarded("POST /risks", writePolicy, h.CreateAssessment, moduleGuard)
	r.Guarded("GET /risks/{id}", readPolicy, h.Detail, moduleGuard, assessmentReadScoped)
	r.Guarded("POST /risks/{id}/delete", deletePolicy, h.DeleteAssessment, moduleGuard, assessmentReadScoped)
	r.Guarded("POST /risks/{id}/toggle-public", writePolicy, h.TogglePublic, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)

	// Wizard steps (new assessment flow)
	r.Guarded("GET /risks/{id}/step/2", writePolicy, h.Step2, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/step/2", writePolicy, h.SaveStep2, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("GET /risks/{id}/step/3", writePolicy, h.Step3, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/step/3", writePolicy, h.SaveStep3, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("GET /risks/{id}/step/3/evaluate", writePolicy, h.Evaluate, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/step/3/evaluate", writePolicy, h.SaveEvaluate, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("GET /risks/{id}/step/4", writePolicy, h.Step4, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/step/4", writePolicy, h.SaveStep4, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("GET /risks/{id}/decision", writePolicy, h.DecisionStart, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("GET /risks/{id}/decision/{rid}", writePolicy, h.DecisionRisk, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/decision/{rid}", writePolicy, h.SaveDecisionRisk, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)

	// Individual risk detail
	r.Guarded("GET /risks/{id}/risks/{rid}", readPolicy, h.RiskDetail, moduleGuard, assessmentReadScoped)
	r.Guarded("GET /risks/{id}/risks/{rid}/reassess", readPolicy, h.ReassessRisk, moduleGuard, assessmentReadScoped)
	r.Guarded("POST /risks/{id}/risks/{rid}/reassess", writePolicy, h.SubmitRiskReassessment, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)

	// Risk CRUD within an assessment (HTMX)
	r.Guarded("POST /risks/{id}/risks", writePolicy, h.AddRisk, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("DELETE /risks/{id}/risks/{rid}", writePolicy, h.DeleteRisk, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/risks/{rid}/delete", writePolicy, h.DeleteRiskAndRedirect, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)

	// SoA linkage (HTMX)
	r.Guarded("GET /risks/{id}/risks/{rid}/measures/search", readPolicy, h.SearchMeasures, moduleGuard, assessmentReadScoped)
	r.Guarded("POST /risks/{id}/risks/{rid}/measures/new", writePolicy, h.QuickCreateMeasure, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/risks/{rid}/measures/{mid}", writePolicy, h.LinkMeasure, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("DELETE /risks/{id}/risks/{rid}/measures/{mid}", writePolicy, h.UnlinkMeasure, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/risks/{rid}/measures/{mid}/delete", writePolicy, h.UnlinkMeasureAndRedirect, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)

	// Activity linkage (HTMX)
	r.Guarded("GET /risks/{id}/risks/{rid}/activities/search", readPolicy, h.SearchActivities, moduleGuard, assessmentReadScoped)
	r.Guarded("POST /risks/{id}/risks/{rid}/activities/{aid}", writePolicy, h.LinkActivity, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("DELETE /risks/{id}/risks/{rid}/activities/{aid}", writePolicy, h.UnlinkActivity, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/risks/{rid}/activities/{aid}/delete", writePolicy, h.UnlinkActivityAndRedirect, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/risks/{rid}/assets/{aid}/add", writePolicy, h.AddRiskAsset, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/risks/{rid}/assets/{aid}/remove", writePolicy, h.RemoveRiskAsset, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)

	// Acceptance loop
	r.Guarded("POST /risks/{id}/accept", acceptPolicy, h.AcceptAssessment, moduleGuard, assessmentAcceptOwn)
	r.Guarded("POST /risks/{id}/decline", declinePolicy, h.DeclineAssessment, moduleGuard, assessmentDeclineOwn)

	// Review mode
	r.Guarded("GET /risks/{id}/review", readPolicy, h.ReviewStart, moduleGuard, assessmentReadScoped)
	r.Guarded("GET /risks/{id}/review/step/1", writePolicy, h.ReviewStep1, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/review/step/1", writePolicy, h.SaveReviewStep1, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/participants/{uid}/add", writePolicy, h.AddAssessmentParticipant, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/participants/{uid}/remove", writePolicy, h.RemoveAssessmentParticipant, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/assets/{aid}/add", writePolicy, h.AddAssessmentAsset, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/assets/{aid}/remove", writePolicy, h.RemoveAssessmentAsset, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("GET /risks/{id}/review/step/2", writePolicy, h.ReviewStep2, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/review/step/2", writePolicy, h.SaveStep2, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("GET /risks/{id}/review/step/3", writePolicy, h.ReviewStep3, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/review/step/3", writePolicy, h.SaveStep3, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("GET /risks/{id}/review/step/3/evaluate", writePolicy, h.ReviewEvaluate, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/review/step/3/evaluate", writePolicy, h.SaveEvaluate, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("GET /risks/{id}/review/step/4", writePolicy, h.ReviewStep4, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/review/step/4", writePolicy, h.SaveReviewStep4, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("GET /risks/{id}/review/decision", writePolicy, h.ReviewDecisionStart, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("GET /risks/{id}/review/decision/{rid}", writePolicy, h.ReviewDecisionRisk, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	r.Guarded("POST /risks/{id}/review/decision/{rid}", writePolicy, h.SaveReviewDecisionRisk, moduleGuard, assessmentReadScoped, assessmentUpdateOwn)
	return nil
}
