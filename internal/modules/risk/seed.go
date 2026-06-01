package risk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/modregistry"
)

var (
	errNoDevUsersForRisk = errors.New("no dev stub users found; run stub-user seeding first")
	errNoOrgsForRisk     = errors.New("no organizations found; run organization seeding first")
)

var assessmentSeeds = []struct {
	Name           string
	Scope          string
	AnalysisObject string
	SecurityGoal   string
	BusinessGoal   string
	Type           string
}{
	{
		Name:           "External API Exposure Assessment",
		Scope:          "Internet-facing APIs and authentication controls.",
		AnalysisObject: "Public API platform and gateway integrations.",
		SecurityGoal:   "Preserve confidentiality and integrity of customer data in transit.",
		BusinessGoal:   "Maintain reliable API delivery and contractual security commitments.",
		Type:           "initial",
	},
	{
		Name:           "Identity and Access Management Assessment",
		Scope:          "SSO, privileged access, and joiner-mover-leaver workflows.",
		AnalysisObject: "Identity provider and account lifecycle process.",
		SecurityGoal:   "Prevent unauthorized privilege escalation and stale access.",
		BusinessGoal:   "Support fast onboarding without lowering access assurance.",
		Type:           "periodic",
	},
	{
		Name:           "Endpoint Security Baseline Assessment",
		Scope:          "Corporate endpoints, patching, and endpoint detection coverage.",
		AnalysisObject: "Managed laptop fleet and endpoint tooling.",
		SecurityGoal:   "Reduce likelihood of malware and credential theft compromise.",
		BusinessGoal:   "Keep workforce productivity high while meeting compliance targets.",
		Type:           "periodic",
	},
	{
		Name:           "Third-Party Vendor Integration Assessment",
		Scope:          "Critical SaaS and processing vendor data paths.",
		AnalysisObject: "Vendor integrations and data exchange contracts.",
		SecurityGoal:   "Limit data exfiltration and vendor-originated security incidents.",
		BusinessGoal:   "Enable safe partner operations and continuous delivery.",
		Type:           "initial",
	},
	{
		Name:           "Business Continuity and Recovery Assessment",
		Scope:          "Backup integrity, restore paths, and service failover readiness.",
		AnalysisObject: "Disaster recovery procedures and platform dependencies.",
		SecurityGoal:   "Ensure availability and timely recovery of critical services.",
		BusinessGoal:   "Minimize outage impact on customers and regulatory obligations.",
		Type:           "periodic",
	},
	{
		Name:           "Enterprise Revenue Concentration Assessment",
		Scope:          "Revenue dependence on top accounts, renewal timing, and channel diversity.",
		AnalysisObject: "Sales portfolio concentration and contract pipeline resilience.",
		SecurityGoal:   "Protect key customer records and forecasting integrity for executive decisions.",
		BusinessGoal:   "Reduce single-customer concentration risk and improve revenue predictability.",
		Type:           "periodic",
	},
	{
		Name:           "Supply Chain and Vendor Resilience Assessment",
		Scope:          "Critical suppliers, subcontractor continuity, and operational fallback options.",
		AnalysisObject: "Procurement dependencies and vendor continuity commitments.",
		SecurityGoal:   "Preserve integrity and availability of supplier-delivered services and data.",
		BusinessGoal:   "Limit operational disruption and margin impact from vendor outages.",
		Type:           "initial",
	},
}

var riskScenarioSeeds = []struct {
	Name        string
	Description string
}{
	{Name: "Third-party vendor service outage", Description: "A critical external provider outage blocks key operational workflows and customer commitments."},
	{Name: "Revenue concentration in top customers", Description: "Contract churn in a small number of large accounts causes material revenue shortfall."},
	{Name: "Regulatory compliance deadline slippage", Description: "Delayed completion of mandated controls triggers penalties and reputational impact."},
	{Name: "Key-person dependency in operations", Description: "Single points of failure in specialist roles disrupt decision-making and delivery."},
	{Name: "Cash-flow pressure from delayed receivables", Description: "Large payment delays compress operating runway and constrain investment decisions."},
}

var riskScaleLabelSeeds = []db.UpsertRiskScaleLabelParams{
	{Scale: "probability", Level: 1, Label: "Very unlikely", Description: "Rare in normal conditions (roughly less than 5% annual probability)."},
	{Scale: "probability", Level: 2, Label: "Unlikely", Description: "Could occur, but not expected in most years (about 5-20% annual probability)."},
	{Scale: "probability", Level: 3, Label: "Possible", Description: "May occur under realistic conditions (about 20-50% annual probability)."},
	{Scale: "probability", Level: 4, Label: "Likely", Description: "Expected to occur in many scenarios (about 50-80% annual probability)."},
	{Scale: "probability", Level: 5, Label: "Very likely", Description: "Expected frequently unless controls improve (roughly over 80% annual probability)."},
	{Scale: "consequence", Level: 1, Label: "Negligible", Description: consequenceSeedDescription(
		"No measurable disruption; handled within routine operations.",
		"No sensitive data exposed; no customer impact.",
		"No legal/regulatory breach expected.",
		"Minimal visibility outside the team.",
	)},
	{Scale: "consequence", Level: 2, Label: "Minor", Description: consequenceSeedDescription(
		"Short-lived disruption with low cost and local impact.",
		"Limited internal data exposure with low sensitivity.",
		"Minor non-conformance; simple remediation required.",
		"Limited stakeholder concern.",
	)},
	{Scale: "consequence", Level: 3, Label: "Moderate", Description: consequenceSeedDescription(
		"Noticeable service impact, rework, or medium financial loss.",
		"Exposure of internal or customer data requiring response.",
		"Reportable non-compliance with potential supervisory follow-up.",
		"Negative coverage or customer trust impact in key segments.",
	)},
	{Scale: "consequence", Level: 4, Label: "Severe", Description: consequenceSeedDescription(
		"Major service interruption or significant financial/operational loss.",
		"Sensitive data compromise with broad customer impact.",
		"Serious legal/regulatory breach with likely sanctions.",
		"Sustained reputational damage and measurable customer churn risk.",
	)},
	{Scale: "consequence", Level: 5, Label: "Critical", Description: consequenceSeedDescription(
		"Prolonged outage or existential financial/operational impact.",
		"Large-scale compromise of highly sensitive data.",
		"Major legal/regulatory enforcement with severe penalties.",
		"Severe long-term trust erosion with strategic business impact.",
	)},
}

func (riskModule) DevSeed(ctx context.Context, deps modregistry.Dependencies) error {
	if err := ensureRiskScaleLabels(ctx, deps.Queries); err != nil {
		return err
	}
	users, orgs, assets, measures, activities, err := loadSeedContext(ctx, deps.Queries)
	if err != nil {
		return err
	}
	assessments, err := ensureAssessments(ctx, deps.Queries, users, orgs)
	if err != nil {
		return err
	}
	if err := enrichAssessments(ctx, deps.Queries, users, assets, assessments); err != nil {
		return err
	}
	if err := ensureAssessmentRisks(ctx, deps.Queries, assessments, measures, activities, assets); err != nil {
		return err
	}
	return nil
}

func ensureRiskScaleLabels(ctx context.Context, q db.Querier) error {
	existing, err := q.ListRiskScaleLabels(ctx)
	if err != nil {
		return fmt.Errorf("list risk scale labels: %w", err)
	}
	existingKeys := make(map[string]struct{}, len(existing))
	for _, label := range existing {
		existingKeys[scaleLabelKey(label.Scale, label.Level)] = struct{}{}
	}
	for _, seed := range riskScaleLabelSeeds {
		if _, found := existingKeys[scaleLabelKey(seed.Scale, seed.Level)]; found {
			continue
		}
		if err := q.UpsertRiskScaleLabel(ctx, seed); err != nil {
			return fmt.Errorf("upsert risk scale label %s %d: %w", seed.Scale, seed.Level, err)
		}
	}
	return nil
}

func scaleLabelKey(scale string, level int32) string {
	return fmt.Sprintf("%s:%d", scale, level)
}

func consequenceSeedDescription(finOps, confData, regLegal, repTrust string) string {
	lines := []string{
		"Financial / Operational: " + finOps,
		"Confidentiality / Data: " + confData,
		"Regulatory / Legal: " + regLegal,
		"Reputation / Trust: " + repTrust,
	}
	return strings.Join(lines, "\n")
}

func loadSeedContext(
	ctx context.Context,
	q db.Querier,
) ([]db.User, []db.Organization, []db.Asset, []db.Measure, []db.ListActivitiesRow, error) {
	users, err := q.ListDevStubUsers(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("list dev stub users: %w", err)
	}
	if len(users) == 0 {
		return nil, nil, nil, nil, nil, errNoDevUsersForRisk
	}
	orgs, err := q.ListOrganizations(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("list organizations: %w", err)
	}
	if len(orgs) == 0 {
		return nil, nil, nil, nil, nil, errNoOrgsForRisk
	}
	assets, err := q.ListAssets(ctx, db.ListAssetsParams{Q: "", Status: "", PageOffset: 0, PageSize: 1000})
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("list assets: %w", err)
	}
	measures, err := q.ListMeasures(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("list measures: %w", err)
	}
	activities, err := q.ListActivities(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("list activities: %w", err)
	}
	return users, orgs, assets, measures, activities, nil
}

func ensureAssessments(
	ctx context.Context,
	q db.Querier,
	users []db.User,
	orgs []db.Organization,
) ([]db.RiskAssessment, error) {
	existingAssessments, err := q.ListRiskAssessments(ctx)
	if err != nil {
		return nil, fmt.Errorf("list risk assessments: %w", err)
	}
	byName := make(map[string]db.RiskAssessment, len(existingAssessments))
	for _, assessment := range existingAssessments {
		byName[assessment.Name] = assessment
	}

	out := make([]db.RiskAssessment, 0, len(assessmentSeeds))
	for i := 0; i < len(assessmentSeeds); i++ {
		assessment, ensureErr := ensureSingleAssessment(ctx, q, byName, users, orgs, i)
		if ensureErr != nil {
			return nil, ensureErr
		}
		out = append(out, assessment)
	}
	return out, nil
}

func ensureSingleAssessment(
	ctx context.Context,
	q db.Querier,
	byName map[string]db.RiskAssessment,
	users []db.User,
	orgs []db.Organization,
	idx int,
) (db.RiskAssessment, error) {
	seed := assessmentSeeds[idx]
	if existing, ok := byName[seed.Name]; ok {
		return existing, nil
	}
	owner := users[idx%len(users)]
	org := orgs[idx%len(orgs)]
	created, err := q.CreateRiskAssessment(ctx, db.CreateRiskAssessmentParams{
		Name:               seed.Name,
		Scope:              seed.Scope,
		AnalysisObject:     seed.AnalysisObject,
		SecurityObjectives: seed.SecurityGoal,
		BusinessObjectives: seed.BusinessGoal,
		Type:               seed.Type,
		RiskOwnerID:        uuid.NullUUID{UUID: owner.ID, Valid: true},
		OrgID:              uuid.NullUUID{UUID: org.ID, Valid: true},
		CreatedBy:          uuid.NullUUID{UUID: users[0].ID, Valid: true},
	})
	if err != nil {
		return db.RiskAssessment{}, fmt.Errorf("create risk assessment %q: %w", seed.Name, err)
	}
	return created, nil
}

func enrichAssessments(
	ctx context.Context,
	q db.Querier,
	users []db.User,
	assets []db.Asset,
	assessments []db.RiskAssessment,
) error {
	for idx, assessment := range assessments {
		if err := setAssessmentStep(ctx, q, assessment, idx); err != nil {
			return err
		}
		if err := addAllParticipants(ctx, q, assessment, users); err != nil {
			return err
		}
		if err := addAssessmentAssets(ctx, q, assessment, assets, idx); err != nil {
			return err
		}
		if err := applyAssessmentStatusMix(ctx, q, assessment, idx); err != nil {
			return err
		}
	}
	return nil
}

func setAssessmentStep(ctx context.Context, q db.Querier, assessment db.RiskAssessment, idx int) error {
	step := int32((idx % 4) + 1)
	status := "draft"
	if idx == 2 || idx == 4 {
		status = "pending_acceptance"
	}
	if err := q.UpdateRiskAssessmentStep(ctx, db.UpdateRiskAssessmentStepParams{
		ID:          assessment.ID,
		CurrentStep: step,
		Status:      status,
	}); err != nil {
		return fmt.Errorf("update risk assessment step for %s: %w", assessment.Name, err)
	}
	return nil
}

func addAllParticipants(ctx context.Context, q db.Querier, assessment db.RiskAssessment, users []db.User) error {
	for _, user := range users {
		if err := q.AddAssessmentParticipant(ctx, db.AddAssessmentParticipantParams{
			AssessmentID: assessment.ID,
			UserID:       user.ID,
		}); err != nil {
			return fmt.Errorf("add participant %s to %s: %w", user.ProviderID, assessment.Name, err)
		}
	}
	return nil
}

func addAssessmentAssets(ctx context.Context, q db.Querier, assessment db.RiskAssessment, assets []db.Asset, idx int) error {
	if len(assets) == 0 {
		return nil
	}
	for a := 0; a < len(assets) && a < 2; a++ {
		asset := assets[(idx+a)%len(assets)]
		if err := q.AddAssetToAssessment(ctx, db.AddAssetToAssessmentParams{
			AssessmentID: assessment.ID,
			AssetID:      asset.ID,
		}); err != nil {
			return fmt.Errorf("add asset %s to %s: %w", asset.Name, assessment.Name, err)
		}
	}
	return nil
}

func applyAssessmentStatusMix(ctx context.Context, q db.Querier, assessment db.RiskAssessment, idx int) error {
	switch idx {
	case 2:
		if _, err := q.AcceptAssessment(ctx, assessment.ID); err != nil {
			return fmt.Errorf("accept assessment %s: %w", assessment.Name, err)
		}
	case 3:
		if _, err := q.DeclineAssessment(ctx, db.DeclineAssessmentParams{
			ID:             assessment.ID,
			AcceptanceNote: "Needs clearer treatment rationale in development seed.",
		}); err != nil {
			return fmt.Errorf("decline assessment %s: %w", assessment.Name, err)
		}
	case 4:
		if err := q.UpdateRiskAssessmentReviewed(ctx, assessment.ID); err != nil {
			return fmt.Errorf("mark reviewed %s: %w", assessment.Name, err)
		}
	}
	return nil
}

func ensureAssessmentRisks(
	ctx context.Context,
	q db.Querier,
	assessments []db.RiskAssessment,
	measures []db.Measure,
	activities []db.ListActivitiesRow,
	assets []db.Asset,
) error {
	for idx, assessment := range assessments {
		if err := ensureRisksForAssessment(ctx, q, assessment, idx, measures, activities, assets); err != nil {
			return err
		}
	}
	return nil
}

func ensureRisksForAssessment(
	ctx context.Context,
	q db.Querier,
	assessment db.RiskAssessment,
	assessmentIdx int,
	measures []db.Measure,
	activities []db.ListActivitiesRow,
	assets []db.Asset,
) error {
	existingRisks, err := q.ListRisksForAssessment(ctx, assessment.ID)
	if err != nil {
		return fmt.Errorf("list risks for assessment %s: %w", assessment.Name, err)
	}
	byName := make(map[string]db.Risk, len(existingRisks))
	for _, risk := range existingRisks {
		byName[risk.Name] = risk
	}
	for i := 0; i < len(riskScenarioSeeds); i++ {
		risk, ensureErr := ensureSingleRisk(ctx, q, byName, assessment, i)
		if ensureErr != nil {
			return ensureErr
		}
		if err := updateRiskScoringAndDecision(ctx, q, risk, assessmentIdx, i+1); err != nil {
			return err
		}
		if err := linkRiskArtifacts(ctx, q, risk, assessmentIdx+i, measures, activities, assets); err != nil {
			return err
		}
	}
	return nil
}

func ensureSingleRisk(
	ctx context.Context,
	q db.Querier,
	byName map[string]db.Risk,
	assessment db.RiskAssessment,
	idx int,
) (db.Risk, error) {
	seed := riskScenarioSeeds[idx]
	riskName := fmt.Sprintf("%s: %s", assessment.Name, seed.Name)
	if risk, ok := byName[riskName]; ok {
		return risk, nil
	}
	created, err := q.CreateRisk(ctx, db.CreateRiskParams{
		AssessmentID: assessment.ID,
		Name:         riskName,
		Description:  seed.Description,
	})
	if err != nil {
		return db.Risk{}, fmt.Errorf("create risk %s: %w", riskName, err)
	}
	return created, nil
}

func updateRiskScoringAndDecision(ctx context.Context, q db.Querier, risk db.Risk, assessmentIdx, i int) error {
	likelihood := int32(((assessmentIdx + i) % 5) + 1)
	consequence := int32(((assessmentIdx*2 + i) % 5) + 1)
	targetLikelihood := max(1, likelihood-1)
	targetConsequence := max(1, consequence-1)
	if _, err := q.UpdateRiskCurrentScores(ctx, db.UpdateRiskCurrentScoresParams{
		ID:                   risk.ID,
		LikelihoodCurrent:    sql.NullInt32{Int32: likelihood, Valid: true},
		ConsequenceCurrent:   sql.NullInt32{Int32: consequence, Valid: true},
		LikelihoodReasoning:  "Seeded baseline likelihood assessment.",
		ConsequenceReasoning: "Seeded baseline consequence assessment.",
	}); err != nil {
		return fmt.Errorf("update risk current scores for %s: %w", risk.Name, err)
	}
	if err := q.UpdateRiskTargetScore(ctx, db.UpdateRiskTargetScoreParams{
		ID:                risk.ID,
		LikelihoodTarget:  sql.NullInt32{Int32: targetLikelihood, Valid: true},
		ConsequenceTarget: sql.NullInt32{Int32: targetConsequence, Valid: true},
	}); err != nil {
		return fmt.Errorf("update risk target scores for %s: %w", risk.Name, err)
	}
	decision := decisionForSeed(likelihood, consequence, i)
	if err := q.UpdateRiskDecision(ctx, db.UpdateRiskDecisionParams{
		ID:            risk.ID,
		RiskDecision:  decision,
		DecisionNotes: "Development seed decision rationale.",
	}); err != nil {
		return fmt.Errorf("update risk decision for %s: %w", risk.Name, err)
	}
	return nil
}

func decisionForSeed(likelihood, consequence int32, i int) string {
	decisionCycle := []string{"accept", "treat", "document", "treat", "document"}
	decision := decisionCycle[(i-1)%len(decisionCycle)]
	if decision == "accept" && (likelihood*consequence) > 6 {
		return "treat"
	}
	return decision
}

func linkRiskArtifacts(
	ctx context.Context,
	q db.Querier,
	risk db.Risk,
	seedOffset int,
	measures []db.Measure,
	activities []db.ListActivitiesRow,
	assets []db.Asset,
) error {
	if len(measures) > 0 {
		measure := measures[seedOffset%len(measures)]
		if err := q.LinkRiskToMeasure(ctx, db.LinkRiskToMeasureParams{
			RiskID:    risk.ID,
			MeasureID: measure.ID,
			Note:      "Seeded mitigation link.",
		}); err != nil {
			return fmt.Errorf("link risk %s to measure %s: %w", risk.Name, measure.Name, err)
		}
	}
	if len(activities) > 0 {
		activity := activities[seedOffset%len(activities)]
		if err := q.LinkRiskToActivity(ctx, db.LinkRiskToActivityParams{
			RiskID:     risk.ID,
			ActivityID: activity.ID,
		}); err != nil {
			return fmt.Errorf("link risk %s to activity %s: %w", risk.Name, activity.Title, err)
		}
	}
	if len(assets) > 0 {
		asset := assets[seedOffset%len(assets)]
		if err := q.LinkRiskToAsset(ctx, db.LinkRiskToAssetParams{
			RiskID:  risk.ID,
			AssetID: asset.ID,
		}); err != nil {
			return fmt.Errorf("link risk %s to asset %s: %w", risk.Name, asset.Name, err)
		}
	}
	return nil
}
