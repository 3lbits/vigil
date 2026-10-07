package risktemplates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/google/uuid"
	"github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/locale"
)

func requireContains(t *testing.T, html string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
}

func requireAbsent(t *testing.T, html string, unwanted ...string) {
	t.Helper()
	for _, item := range unwanted {
		if strings.Contains(html, item) {
			t.Errorf("rendered HTML unexpectedly contains %q", item)
		}
	}
}

func TestRiskNativeControlShape(t *testing.T) {
	bundle, err := locale.NewBundle()
	if err != nil {
		t.Fatal(err)
	}
	ctx := locale.SetLocalizer(context.Background(), i18n.NewLocalizer(bundle, locale.LangEN), locale.LangEN)
	assessment := db.RiskAssessment{ID: uuid.New(), Name: "Test assessment", ThreatAssessmentEnabled: true}
	risk := db.Risk{ID: uuid.New(), Name: "Test risk", RiskDecision: "document", ReviewNeeded: true}

	for _, tc := range []struct {
		name      string
		component templ.Component
		contains  []string
	}{
		{"framework", WizardStep1(WizardStep1VM{Assessment: assessment}), []string{`name="_csrf"`, `name="name"`, `required`, `name="org_id"`, `name="risk_owner_id"`, `name="scope"`, `name="security_objectives"`, `name="threat_assessment_enabled"`}},
		{"threat", WizardStepThreat(WizardStepThreatVM{Assessment: assessment}), []string{`name="threat_app_description"`, `name="threat_information_flow"`, `x-ref="src"`, `@click="render()"`, `name="_csrf"`}},
		{"scoring", WizardStep3(WizardStep3VM{Assessment: assessment, Risks: []db.Risk{risk}}), []string{`name="current_likelihood_` + risk.ID.String() + `"`, `x-model="l"`, `x-model="c"`, `name="likelihood_reasoning_` + risk.ID.String() + `"`, `name="consequence_reasoning_` + risk.ID.String() + `"`}},
		{"evaluation", WizardEvaluate(EvaluationVM{Assessment: assessment, Risks: []db.Risk{risk}}), []string{`name="decision_` + risk.ID.String() + `"`, `name="notes_` + risk.ID.String() + `"`, `name="_csrf"`}},
		{"decision", WizardDecision(DecisionVM{Assessment: assessment, Risk: risk, Risks: []db.Risk{risk}}), []string{`name="decision"`, `name="decision_notes"`, `name="next" value="0"`, `name="submit_assessment" value="1"`, `x-model="decision"`}},
		{"treatment", WizardStep4(WizardStep4VM{Assessment: assessment, Risks: []db.Risk{risk}}), []string{`id="finalise-form"`, `name="target_likelihood_` + risk.ID.String() + `"`, `name="target_consequence_` + risk.ID.String() + `"`, `form="finalise-form"`, `x-model="tl"`, `x-model="tc"`, `name="_csrf"`}},
		{"risk detail", RiskDetail(RiskDetailVM{Assessment: assessment, Risk: risk}), []string{`name="assessed_likelihood"`, `name="assessed_consequence"`, `name="assessment_rationale"`, `name="asset_q"`, `name="_csrf"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := tc.component.Render(ctx, &out); err != nil {
				t.Fatal(err)
			}
			requireContains(t, out.String(), tc.contains...)
		})
	}
}

func TestRiskUI2PagesAndFragments(t *testing.T) {
	bundle, err := locale.NewBundle()
	if err != nil {
		t.Fatal(err)
	}
	ctx := locale.SetLocalizer(context.Background(), i18n.NewLocalizer(bundle, locale.LangEN), locale.LangEN)
	render := func(component templ.Component) string {
		t.Helper()
		var out bytes.Buffer
		if err := component.Render(ctx, &out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	hasPage := func(html string) bool {
		return strings.Contains(html, `<div class="ui2 ui2-risk">`)
	}

	assessmentID := uuid.New()
	riskID := uuid.New()
	list := AssessmentListPageVM{
		Assessments: []AssessmentListVM{{Assessment: db.RiskAssessment{ID: assessmentID, Name: "Test <assessment>", Status: "active", CurrentStep: 2}, RiskCount: 1}},
		Q:           "test",
	}
	page := render(RiskPage(AssessmentList(list)))
	if !hasPage(page) {
		t.Error("assessment list missing UI2 page wrapper")
	}
	requireContains(t, page, `class="ui2-table"`, `role="search"`, `name="q"`, `value="test"`, `hx-target="#assessment-list"`, `hx-include="closest form"`, `hx-trigger="input changed delay:300ms"`, `hx-on::after-request=`, `href="/risks/`+assessmentID.String()+`"`, `Test &lt;assessment&gt;`, `<progress`)
	fragment := render(AssessmentItems(list))
	if hasPage(fragment) || !strings.Contains(fragment, `class="ui2-table"`) {
		t.Error("assessment htmx fragment must have a table but no page wrapper")
	}
	register := render(RiskPage(RiskRegisterPage([]db.ListAllRisksRow{{ID: riskID, AssessmentID: assessmentID, Name: "Test risk", RiskDecision: "treat"}}, "risk", 5, 12)))
	requireContains(t, register, `hx-target="#risk-register-table"`, `hx-on::after-request=`, `value="risk"`, `href="/risks/`+assessmentID.String()+`/risks/`+riskID.String()+`"`, `Test risk`, `class="ui2-badge"`)

	wizard := render(RiskPage(WizardStep2(WizardStep2VM{
		Assessment: db.RiskAssessment{ID: assessmentID, Name: "Wizard"},
		Risks:      []db.Risk{{ID: riskID, Name: "Test risk"}},
	})))
	if !hasPage(wizard) {
		t.Error("wizard step missing UI2 page wrapper")
	}
	requireContains(t, wizard, `aria-current="step"`, `id="risk-rows"`, `id="risk-row-`+riskID.String()+`"`, `name="name_`+riskID.String()+`"`, `name="_csrf"`, `hx-target="#risk-rows"`, `name="step2_action"`, `data-tone="danger"`)
	row := render(NewRiskRow(db.Risk{ID: riskID, Name: "New risk"}, assessmentID.String()))
	if !strings.Contains(row, `<tr id="risk-row-`) || hasPage(row) {
		t.Error("new-risk htmx response must remain a standalone table row")
	}
	linkedMeasure := render(LinkedMeasureChip(db.Measure{ID: uuid.New(), Name: "Control"}, riskID.String(), assessmentID.String()))
	linkedActivity := render(LinkedActivityChip(db.Activity{ID: uuid.New(), Title: "Work"}, riskID.String(), assessmentID.String()))
	if !strings.Contains(linkedMeasure, `data-tone="danger"`) || !strings.Contains(linkedActivity, `data-tone="danger"`) {
		t.Error("unlink buttons must retain destructive affordance in htmx fragments")
	}
}
