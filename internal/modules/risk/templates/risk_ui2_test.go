package risktemplates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
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

func renderRiskTest(t *testing.T, ctx context.Context, component templ.Component) string {
	t.Helper()
	var out bytes.Buffer
	if err := component.Render(ctx, &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func checkRiskAttr(t *testing.T, html, selector, attr, want string) {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	control := doc.Find(selector)
	if control.Length() != 1 {
		t.Errorf("%s: got %d controls, want 1", selector, control.Length())
	} else if got, ok := control.Attr(attr); !ok || got != want {
		t.Errorf("%s %s: got %q (present: %v), want %q", selector, attr, got, ok, want)
	}
}

func checkRiskText(t *testing.T, html, selector, want string) {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Find(selector).Text(); got != want {
		t.Errorf("%s text: got %q, want %q", selector, got, want)
	}
}

func TestRiskControlShape(t *testing.T) {
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
		{"decision", WizardDecision(DecisionVM{Assessment: assessment, Risk: risk, Risks: []db.Risk{risk}}), []string{`name="decision"`, `name="decision_notes"`, `x-model="decision"`}},
		{"treatment", WizardStep4(WizardStep4VM{Assessment: assessment, Risks: []db.Risk{risk}}), []string{`id="finalise-form"`, `name="target_likelihood_` + risk.ID.String() + `"`, `name="target_consequence_` + risk.ID.String() + `"`, `form="finalise-form"`, `x-model="tl"`, `x-model="tc"`, `name="_csrf"`}},
		{"risk detail", RiskDetail(RiskDetailVM{Assessment: assessment, Risk: risk}), []string{`name="assessed_likelihood"`, `name="assessed_consequence"`, `name="assessment_rationale"`, `name="asset_q"`, `name="_csrf"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireContains(t, renderRiskTest(t, ctx, tc.component), tc.contains...)
		})
	}
	decision := renderRiskTest(t, ctx, WizardDecision(DecisionVM{Assessment: assessment, Risk: risk, Risks: []db.Risk{risk}}))
	checkRiskAttr(t, decision, `button[name="next"][value="0"]`, "type", "submit")
	checkRiskAttr(t, decision, `button[name="submit_assessment"][value="1"]`, "type", "submit")
}

func TestRiskUI2PagesAndFragments(t *testing.T) {
	bundle, err := locale.NewBundle()
	if err != nil {
		t.Fatal(err)
	}
	ctx := locale.SetLocalizer(context.Background(), i18n.NewLocalizer(bundle, locale.LangEN), locale.LangEN)
	render := func(component templ.Component) string {
		return renderRiskTest(t, ctx, component)
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
		Risks:      []db.Risk{{ID: riskID, Name: "Test risk", Description: "Detail < &"}},
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

func TestRiskSharedControlAttributes(t *testing.T) {
	bundle, err := locale.NewBundle()
	if err != nil {
		t.Fatal(err)
	}
	ctx := locale.SetLocalizer(context.Background(), i18n.NewLocalizer(bundle, locale.LangEN), locale.LangEN)
	render := func(component templ.Component) string {
		return renderRiskTest(t, ctx, component)
	}
	assessmentID, riskID := uuid.New(), uuid.New()
	page := render(RiskPage(AssessmentList(AssessmentListPageVM{Q: "test"})))
	checkRiskAttr(t, page, `#assessment-search`, "hx-on::after-request", "if(event.detail.successful){document.querySelector('#assessment-list [role=region]')?.focus()}")
	wizard := render(WizardStep2(WizardStep2VM{
		Assessment: db.RiskAssessment{ID: assessmentID},
		Risks:      []db.Risk{{ID: riskID, Name: "Test risk", Description: "Detail < &"}},
	}))
	checkRiskAttr(t, wizard, `#asset-q-`+riskID.String(), "hx-get", "/risks/"+assessmentID.String()+"/step/2")
	checkRiskAttr(t, wizard, `#asset-q-`+riskID.String(), "hx-include", "#risk-asset-search-controls-"+riskID.String())
	checkRiskAttr(t, wizard, `#risk-row-`+riskID.String()+` button.risk-delete-button`, "hx-on::after-request", "document.getElementById('risk-step2-status').textContent='Risk deleted'; document.getElementById('risk_name')?.focus()")
	checkRiskAttr(t, wizard, `#risk-name-`+riskID.String(), "value", "Test risk")
	checkRiskText(t, wizard, `#risk-description-`+riskID.String(), "Detail < &")
	linkedMeasure := render(LinkedMeasureChip(db.Measure{ID: uuid.New(), Name: "Control"}, riskID.String(), assessmentID.String()))
	checkRiskAttr(t, linkedMeasure, `button.risk-chip-remove`, "hx-on::after-request", "this.closest('[x-data]')?.querySelector('[id^=risk-status-]')?.replaceChildren(document.createTextNode('Measure unlinked')); this.closest('[x-data]')?.querySelector('[id^=risk-heading-]')?.focus()")
	userID, assetID := uuid.New(), uuid.New()
	framework := render(WizardStep1(WizardStep1VM{
		Assessment:    db.RiskAssessment{ID: assessmentID, Name: "Assessment", Scope: "Scope < &"},
		ThreatEnabled: true,
		Users:         []db.User{{ID: userID, Name: "Alice"}},
		Participants:  []db.User{{ID: userID, Name: "Alice"}},
		Assets:        []db.Asset{{ID: assetID, Name: "Server"}},
	}))
	checkRiskAttr(t, framework, `button.risk-participant-chip`, "formaction", "/risks/"+assessmentID.String()+"/participants/"+userID.String()+"/remove")
	checkRiskAttr(t, framework, `button.risk-asset-chip`, "formaction", "/risks/"+assessmentID.String()+"/assets/"+assetID.String()+"/remove")
	checkRiskAttr(t, framework, `button.risk-participant-chip`, "formnovalidate", "")
	checkRiskAttr(t, framework, `#threat_assessment_enabled`, "checked", "")
	checkRiskText(t, framework, "#scope", "Scope < &")
	decision := render(WizardDecision(DecisionVM{Assessment: db.RiskAssessment{ID: assessmentID}, Risk: db.Risk{ID: riskID, RiskDecision: "document"}, Risks: []db.Risk{{ID: riskID}}}))
	checkRiskAttr(t, decision, `input[name="decision"][value="document"]`, "checked", "")
	checkRiskAttr(t, decision, `input[name="decision"][value="accept"]`, "disabled", "")
	checkRiskAttr(t, decision, `a.risk-decision-selected`, "href", "/risks/"+assessmentID.String()+"/decision/"+riskID.String())
	threat := render(WizardStepThreat(WizardStepThreatVM{Assessment: db.RiskAssessment{ID: assessmentID}}))
	checkRiskAttr(t, threat, `button.risk-secondary-button`, "@click", "render()")
	checkRiskAttr(t, threat, `button.risk-secondary-button`, ":disabled", "rendering")
	checkRiskAttr(t, render(WizardStep3(WizardStep3VM{Assessment: db.RiskAssessment{ID: assessmentID}, Risks: []db.Risk{{ID: riskID}}})), `button.risk-split`, ":aria-expanded", "expanded.toString()")
	checkRiskAttr(t, render(WizardStep4(WizardStep4VM{Assessment: db.RiskAssessment{ID: assessmentID}, Risks: []db.Risk{{ID: riskID}}})), `select[name="target_likelihood_`+riskID.String()+`"]`, "form", "finalise-form")
}
