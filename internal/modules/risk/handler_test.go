package risk

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/modregistry"
	"github.com/3lbits/vigil/internal/testutil"
)

func TestStep2Path(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	tests := []struct {
		name           string
		review         bool
		shouldContinue bool
		want           string
	}{
		{name: "standard save", review: false, shouldContinue: false, want: "/risks/11111111-1111-1111-1111-111111111111/step/2"},
		{name: "standard continue", review: false, shouldContinue: true, want: "/risks/11111111-1111-1111-1111-111111111111/step/3"},
		{name: "review save", review: true, shouldContinue: false, want: "/risks/11111111-1111-1111-1111-111111111111/review/step/2"},
		{name: "review continue", review: true, shouldContinue: true, want: "/risks/11111111-1111-1111-1111-111111111111/review/step/3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := step2Path(id, tc.review, tc.shouldContinue); got != tc.want {
				t.Fatalf("step2Path(%v,%v)=%q, want %q", tc.review, tc.shouldContinue, got, tc.want)
			}
		})
	}
}

type reassessQ struct {
	testutil.StubQuerier
	assessment     db.RiskAssessment
	risk           db.Risk
	reassessCalled bool
	reassessArg    db.ReassessRiskCurrentScoresParams
}

func (q *reassessQ) GetRiskAssessment(_ context.Context, _ uuid.UUID) (db.RiskAssessment, error) {
	return q.assessment, nil
}

func (q *reassessQ) GetRisk(_ context.Context, _ uuid.UUID) (db.Risk, error) {
	return q.risk, nil
}

func (q *reassessQ) ReassessRiskCurrentScores(_ context.Context, arg db.ReassessRiskCurrentScoresParams) (db.Risk, error) {
	q.reassessCalled = true
	q.reassessArg = arg
	return db.Risk{
		ID:                 arg.ID,
		LikelihoodCurrent:  arg.LikelihoodCurrent,
		ConsequenceCurrent: arg.ConsequenceCurrent,
	}, nil
}

type acceptAuditQ struct {
	testutil.StubQuerier
	assessment db.RiskAssessment
	auditRows  []db.InsertAuditLogParams
}

func (q *acceptAuditQ) GetRiskAssessment(_ context.Context, _ uuid.UUID) (db.RiskAssessment, error) {
	return q.assessment, nil
}

func (q *acceptAuditQ) AcceptAssessment(_ context.Context, id uuid.UUID) (db.RiskAssessment, error) {
	q.assessment.Status = "active"
	return q.assessment, nil
}

func (q *acceptAuditQ) InsertAuditLog(_ context.Context, arg db.InsertAuditLogParams) error {
	q.auditRows = append(q.auditRows, arg)
	return nil
}

func withUser(r *http.Request) *http.Request {
	ctx := middleware.SetUser(r.Context(), middleware.SessionUser{
		ID:   "00000000-0000-0000-0000-000000000010",
		Name: "Reviewer",
		Role: "admin",
	})
	return r.WithContext(ctx)
}

func TestSubmitRiskReassessment_UpdatesCurrentRisk(t *testing.T) {
	assessmentID := uuid.New()
	riskID := uuid.New()
	q := &reassessQ{
		assessment: db.RiskAssessment{ID: assessmentID},
		risk:       db.Risk{ID: riskID, AssessmentID: assessmentID},
	}
	h := NewHandler(q, nil)

	form := url.Values{
		"assessed_likelihood":  {"3"},
		"assessed_consequence": {"2"},
		"assessment_rationale": {"validated after implementation"},
	}
	r := httptest.NewRequest(http.MethodPost, "/risks/"+assessmentID.String()+"/risks/"+riskID.String()+"/reassess", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", assessmentID.String())
	r.SetPathValue("rid", riskID.String())
	r = withUser(r)
	w := httptest.NewRecorder()

	h.SubmitRiskReassessment(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if !q.reassessCalled {
		t.Fatal("expected reassessment query to be called")
	}
	if q.reassessArg.ID != riskID {
		t.Fatalf("expected risk id %s, got %s", riskID, q.reassessArg.ID)
	}
	if !q.reassessArg.LikelihoodCurrent.Valid || q.reassessArg.LikelihoodCurrent.Int32 != 3 {
		t.Fatalf("expected likelihood 3, got %+v", q.reassessArg.LikelihoodCurrent)
	}
	if !q.reassessArg.ConsequenceCurrent.Valid || q.reassessArg.ConsequenceCurrent.Int32 != 2 {
		t.Fatalf("expected consequence 2, got %+v", q.reassessArg.ConsequenceCurrent)
	}
	if q.reassessArg.AssessmentRationale != "validated after implementation" {
		t.Fatalf("unexpected rationale: %q", q.reassessArg.AssessmentRationale)
	}
	if !q.reassessArg.AssessedBy.Valid {
		t.Fatal("expected assessed_by to be set for authenticated user")
	}
}

func TestSubmitRiskReassessment_RequiresBothScores(t *testing.T) {
	assessmentID := uuid.New()
	riskID := uuid.New()
	q := &reassessQ{
		assessment: db.RiskAssessment{ID: assessmentID},
		risk:       db.Risk{ID: riskID, AssessmentID: assessmentID},
	}
	h := NewHandler(q, nil)

	form := url.Values{
		"assessed_likelihood": {"3"},
	}
	r := httptest.NewRequest(http.MethodPost, "/risks/"+assessmentID.String()+"/risks/"+riskID.String()+"/reassess", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", assessmentID.String())
	r.SetPathValue("rid", riskID.String())
	r = withUser(r)
	w := httptest.NewRecorder()

	h.SubmitRiskReassessment(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if q.reassessCalled {
		t.Fatal("did not expect reassessment query call when consequence is missing")
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "reassess_type=error") {
		t.Fatalf("expected error redirect, got %q", loc)
	}
}

func TestSubmitRiskReassessment_StubAuthSetsAssessedBy(t *testing.T) {
	assessmentID := uuid.New()
	riskID := uuid.New()
	q := &reassessQ{
		assessment: db.RiskAssessment{ID: assessmentID},
		risk:       db.Risk{ID: riskID, AssessmentID: assessmentID},
	}
	h := NewHandler(q, nil)

	form := url.Values{
		"assessed_likelihood":  {"3"},
		"assessed_consequence": {"2"},
	}
	r := httptest.NewRequest(http.MethodPost, "/risks/"+assessmentID.String()+"/risks/"+riskID.String()+"/reassess", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", assessmentID.String())
	r.SetPathValue("rid", riskID.String())
	w := httptest.NewRecorder()

	middleware.StubMiddleware(http.HandlerFunc(h.SubmitRiskReassessment)).ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if !q.reassessCalled {
		t.Fatal("expected reassessment query to be called")
	}
	if !q.reassessArg.AssessedBy.Valid {
		t.Fatal("expected assessed_by to be set under stub auth")
	}
}

func TestAcceptAssessment_RecordsAuditWithAssessmentID(t *testing.T) {
	assessmentID := uuid.New()
	q := &acceptAuditQ{
		assessment: db.RiskAssessment{
			ID:     assessmentID,
			Name:   "Vendor onboarding",
			Status: "pending_acceptance",
		},
	}
	h := NewHandler(q, nil)

	r := httptest.NewRequest(http.MethodPost, "/risks/"+assessmentID.String()+"/accept", nil)
	r.SetPathValue("id", assessmentID.String())
	r = withUser(r)
	w := httptest.NewRecorder()

	h.AcceptAssessment(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if len(q.auditRows) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(q.auditRows))
	}
	if q.auditRows[0].Event != "risk.assessment.accepted" {
		t.Fatalf("unexpected audit event: %s", q.auditRows[0].Event)
	}
	attrs := string(q.auditRows[0].Attrs)
	if !strings.Contains(attrs, `"assessment_id":"`+assessmentID.String()+`"`) {
		t.Fatalf("expected assessment_id in attrs, got %s", attrs)
	}
}

func TestRiskCanBeAccepted(t *testing.T) {
	risk := db.Risk{
		LikelihoodCurrent:  sql.NullInt32{Int32: 2, Valid: true},
		ConsequenceCurrent: sql.NullInt32{Int32: 2, Valid: true},
	}
	if !riskCanBeAccepted(risk, 5) {
		t.Fatal("expected low risk to be acceptable")
	}
}

func TestDecisionEligibleRisks_FiltersUnscored(t *testing.T) {
	scored := db.Risk{
		ID:                 uuid.New(),
		LikelihoodCurrent:  sql.NullInt32{Int32: 2, Valid: true},
		ConsequenceCurrent: sql.NullInt32{Int32: 3, Valid: true},
	}
	unscoredLikelihood := db.Risk{
		ID:                 uuid.New(),
		ConsequenceCurrent: sql.NullInt32{Int32: 3, Valid: true},
	}
	unscoredConsequence := db.Risk{
		ID:                uuid.New(),
		LikelihoodCurrent: sql.NullInt32{Int32: 2, Valid: true},
	}

	got := decisionEligibleRisks([]db.Risk{unscoredLikelihood, scored, unscoredConsequence})
	if len(got) != 1 {
		t.Fatalf("expected 1 eligible risk, got %d", len(got))
	}
	if got[0].ID != scored.ID {
		t.Fatalf("expected scored risk %s, got %s", scored.ID, got[0].ID)
	}
}

// ── riskCanBeAccepted (table-driven) ─────────────────────────────────────────

func TestRiskCanBeAccepted_Table(t *testing.T) {
	const lowMax = 6 // 2x3 threshold

	tests := []struct {
		name        string
		likelihood  int32
		consequence int32
		validScores bool
		want        bool
	}{
		{"score below threshold", 2, 3, true, true},  // 6 <= 6
		{"score equals threshold", 3, 2, true, true}, // 6 <= 6
		{"score above threshold", 2, 4, true, false}, // 8 > 6
		{"high risk", 4, 5, true, false},             // 20 > 6
		{"missing likelihood", 0, 3, false, false},   // NullInt32 not valid
		{"missing consequence", 2, 0, false, false},  // NullInt32 not valid
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			risk := db.Risk{}
			if tc.validScores && tc.likelihood != 0 {
				risk.LikelihoodCurrent.Valid = true
				risk.LikelihoodCurrent.Int32 = tc.likelihood
			}
			if tc.validScores && tc.consequence != 0 {
				risk.ConsequenceCurrent.Valid = true
				risk.ConsequenceCurrent.Int32 = tc.consequence
			}
			got := riskCanBeAccepted(risk, lowMax)
			if got != tc.want {
				t.Errorf("riskCanBeAccepted(l=%d,c=%d,valid=%v) = %v, want %v",
					tc.likelihood, tc.consequence, tc.validScores, got, tc.want)
			}
		})
	}
}

// ── assessmentIDFromPath ──────────────────────────────────────────────────────

func TestAssessmentIDFromPath_ValidUUID(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	r := httptest.NewRequest(http.MethodGet, "/risks/"+id.String(), nil)
	r.SetPathValue("id", id.String())
	w := httptest.NewRecorder()

	got, ok := assessmentIDFromPath(w, r)
	if !ok {
		t.Fatal("expected ok=true for valid UUID")
	}
	if got != id {
		t.Errorf("got %v, want %v", got, id)
	}
}

func TestAssessmentIDFromPath_InvalidUUID_Returns400(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/risks/not-a-uuid", nil)
	r.SetPathValue("id", "not-a-uuid")
	w := httptest.NewRecorder()

	_, ok := assessmentIDFromPath(w, r)
	if ok {
		t.Fatal("expected ok=false for invalid UUID")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

type riskHelpersQ struct {
	testutil.StubQuerier
	users []db.User
	gs    db.RiskGlobalSetting
}

func (q *riskHelpersQ) ListUsers(_ context.Context) ([]db.User, error) {
	return q.users, nil
}

func (q *riskHelpersQ) GetRiskGlobalSettings(_ context.Context) (db.RiskGlobalSetting, error) {
	return q.gs, nil
}

func TestRiskHelperFunctions(t *testing.T) {
	if got := nullUUID("not-a-uuid"); got.Valid {
		t.Fatal("expected invalid null UUID")
	}
	id := uuid.New()
	if got := nullUUID(id.String()); !got.Valid || got.UUID != id {
		t.Fatalf("unexpected nullUUID parse: %v", got)
	}
	if got := maxStep(2, 1); got != 2 {
		t.Fatalf("maxStep expected 2, got %d", got)
	}
	if got := maxStep(2, 4); got != 4 {
		t.Fatalf("maxStep expected 4, got %d", got)
	}
	if low, high := thresholdDefaults(0, 0); low != 5 || high != 12 {
		t.Fatalf("unexpected defaults: %d %d", low, high)
	}
	if low, high := thresholdDefaults(4, 10); low != 4 || high != 10 {
		t.Fatalf("unexpected passthrough: %d %d", low, high)
	}
	if got := assessmentPageTitle(db.RiskAssessment{Name: "Name"}); got != "Name" {
		t.Fatalf("unexpected title: %q", got)
	}
	if got := assessmentPageTitle(db.RiskAssessment{Name: "Name", RefNum: sql.NullInt32{Int32: 7, Valid: true}}); got != "RA-007 · Name" {
		t.Fatalf("unexpected ref title: %q", got)
	}
}

func TestResolveMeasureAssigneeAndLowMaxFromContext(t *testing.T) {
	uid := uuid.New()
	q := &riskHelpersQ{
		users: []db.User{{ID: uid, Name: "Alice", Email: "alice@example.com"}},
		gs:    db.RiskGlobalSetting{LowMax: 4, HighMin: 10},
	}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(url.Values{
		"owner":           {"Owner"},
		"assignee_lookup": {"alice@example.com"},
	}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatalf("parse form: %v", err)
	}
	owner, assignee := resolveMeasureAssignee(r, q)
	if owner != "Owner" || !assignee.Valid || assignee.UUID != uid {
		t.Fatalf("unexpected assignee resolution: owner=%q assignee=%v", owner, assignee)
	}

	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := lowMaxFromContext(r2, q); got != 4 {
		t.Fatalf("expected low max 4, got %d", got)
	}
}

func TestDefaultDecisionForRisk(t *testing.T) {
	low := db.Risk{LikelihoodCurrent: sql.NullInt32{Int32: 2, Valid: true}, ConsequenceCurrent: sql.NullInt32{Int32: 2, Valid: true}}
	high := db.Risk{LikelihoodCurrent: sql.NullInt32{Int32: 4, Valid: true}, ConsequenceCurrent: sql.NullInt32{Int32: 4, Valid: true}}
	if got := defaultDecisionForRisk(low, 5); got != "accept" {
		t.Fatalf("expected accept, got %q", got)
	}
	if got := defaultDecisionForRisk(high, 5); got != "treat" {
		t.Fatalf("expected treat, got %q", got)
	}
}

func TestRiskModuleContract(t *testing.T) {
	m := New()
	if got := m.Name(); got != "risk" {
		t.Fatalf("module name = %q, want risk", got)
	}
	mux := http.NewServeMux()
	r := modregistry.NewRegistrar(mux, nil)
	if err := m.Register(modregistry.Dependencies{}, r); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if got := len(r.Meta()); got != 55 {
		t.Fatalf("expected 55 routes, got %d", got)
	}
}

func TestDecisionPathHelpers(t *testing.T) {
	assessmentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	riskID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	if got := decisionStartPath(assessmentID, false); got != "/risks/11111111-1111-1111-1111-111111111111/decision" {
		t.Fatalf("unexpected standard decision start path: %q", got)
	}
	if got := decisionStartPath(assessmentID, true); got != "/risks/11111111-1111-1111-1111-111111111111/review/decision" {
		t.Fatalf("unexpected review decision start path: %q", got)
	}
	if got := decisionRiskPath(assessmentID, riskID, false); got != "/risks/11111111-1111-1111-1111-111111111111/decision/22222222-2222-2222-2222-222222222222" {
		t.Fatalf("unexpected standard decision risk path: %q", got)
	}
	if got := decisionRiskPath(assessmentID, riskID, true); got != "/risks/11111111-1111-1111-1111-111111111111/review/decision/22222222-2222-2222-2222-222222222222" {
		t.Fatalf("unexpected review decision risk path: %q", got)
	}
}

func TestDecisionRiskPathFromRequest(t *testing.T) {
	assessmentID := uuid.New()
	riskID := uuid.New()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := decisionRiskPathFromRequest(r, assessmentID, riskID); got != decisionRiskPath(assessmentID, riskID, false) {
		t.Fatalf("expected standard path, got %q", got)
	}

	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.Header.Set("Referer", "https://app.example/risks/"+assessmentID.String()+"/review/decision/"+riskID.String())
	if got := decisionRiskPathFromRequest(r2, assessmentID, riskID); got != decisionRiskPath(assessmentID, riskID, true) {
		t.Fatalf("expected review path, got %q", got)
	}
}

func TestReviewStep1URLWithQuery(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	if got := reviewStep1URLWithQuery(id, "", ""); got != "/risks/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee/review/step/1" {
		t.Fatalf("unexpected base URL: %q", got)
	}
	got := reviewStep1URLWithQuery(id, "  alice  ", "  laptop ")
	if !strings.HasPrefix(got, "/risks/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee/review/step/1?") ||
		!strings.Contains(got, "participant_q=alice") ||
		!strings.Contains(got, "asset_q=laptop") {
		t.Fatalf("unexpected URL with query params: %q", got)
	}
}

func TestParseUUIDsAndBuildRiskAssets(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	got := parseUUIDs([]string{" " + id1.String() + " ", "bad", id2.String()})
	if len(got) != 2 || got[0] != id1 || got[1] != id2 {
		t.Fatalf("unexpected parsed UUIDs: %#v", got)
	}

	risks := []db.Risk{{ID: id1}, {ID: id2}}
	assetsByRisk := buildRiskAssets(risks, func(riskID uuid.UUID) ([]db.Asset, error) {
		if riskID == id1 {
			return []db.Asset{{ID: uuid.New(), Name: "A1"}}, nil
		}
		return nil, errors.New("failed")
	})
	if len(assetsByRisk) != 1 {
		t.Fatalf("expected 1 mapped risk, got %d", len(assetsByRisk))
	}
	if _, ok := assetsByRisk[id2]; ok {
		t.Fatal("did not expect failed risk lookup to be present")
	}
}

type riskDecisionQ struct {
	testutil.StubQuerier
	settings      db.RiskGlobalSetting
	measures      []db.Measure
	decisionErr   error
	targetErr     error
	decisionCalls int
	targetCalls   int
	lastDecision  db.UpdateRiskDecisionParams
	lastTarget    db.UpdateRiskTargetScoreParams
}

func (q *riskDecisionQ) GetRiskGlobalSettings(context.Context) (db.RiskGlobalSetting, error) {
	return q.settings, nil
}
func (q *riskDecisionQ) ListMeasuresForRisk(context.Context, uuid.UUID) ([]db.Measure, error) {
	return q.measures, nil
}
func (q *riskDecisionQ) UpdateRiskDecision(_ context.Context, arg db.UpdateRiskDecisionParams) error {
	q.decisionCalls++
	q.lastDecision = arg
	return q.decisionErr
}
func (q *riskDecisionQ) UpdateRiskTargetScore(_ context.Context, arg db.UpdateRiskTargetScoreParams) error {
	q.targetCalls++
	q.lastTarget = arg
	return q.targetErr
}

func TestValidateDecisionChoice(t *testing.T) {
	risk := db.Risk{
		ID:                 uuid.New(),
		LikelihoodCurrent:  sql.NullInt32{Int32: 4, Valid: true},
		ConsequenceCurrent: sql.NullInt32{Int32: 4, Valid: true},
	}
	assessmentID := uuid.New()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	h := NewHandler(&riskDecisionQ{settings: db.RiskGlobalSetting{LowMax: 5, HighMin: 12}}, nil)

	if h.validateDecisionChoice(w, r, assessmentID, risk, "document", "", false, false) {
		t.Fatal("document decision without notes should fail")
	}
	w = httptest.NewRecorder()
	if h.validateDecisionChoice(w, r, assessmentID, risk, "accept", "n", false, false) {
		t.Fatal("accept on non-green risk should fail")
	}

	greenRisk := db.Risk{
		ID:                 uuid.New(),
		LikelihoodCurrent:  sql.NullInt32{Int32: 1, Valid: true},
		ConsequenceCurrent: sql.NullInt32{Int32: 2, Valid: true},
	}
	w = httptest.NewRecorder()
	if h.validateDecisionChoice(w, r, assessmentID, greenRisk, "treat", "n", false, false) {
		t.Fatal("green risk non-accept decision should fail")
	}

	q := &riskDecisionQ{
		settings: db.RiskGlobalSetting{LowMax: 5, HighMin: 12},
		measures: []db.Measure{{ID: uuid.New()}},
	}
	h2 := NewHandler(q, nil)
	if !h2.validateDecisionChoice(httptest.NewRecorder(), r, assessmentID, risk, "treat", "n", false, true) {
		t.Fatal("treat with linked measure should pass")
	}
}

func TestPersistDecision(t *testing.T) {
	risk := db.Risk{
		ID:                 uuid.New(),
		LikelihoodCurrent:  sql.NullInt32{Int32: 2, Valid: true},
		ConsequenceCurrent: sql.NullInt32{Int32: 3, Valid: true},
	}
	q := &riskDecisionQ{}
	h := NewHandler(q, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)

	if err := h.persistDecision(w, r, risk, "invalid", "", sql.NullInt32{}, sql.NullInt32{}); err != nil {
		t.Fatalf("invalid decision should be ignored, got err %v", err)
	}
	if q.decisionCalls != 0 {
		t.Fatalf("expected no writes on invalid decision, got %d", q.decisionCalls)
	}

	if err := h.persistDecision(w, r, risk, "accept", "ok", sql.NullInt32{}, sql.NullInt32{}); err != nil {
		t.Fatalf("accept persist error: %v", err)
	}
	if q.decisionCalls == 0 || q.targetCalls == 0 {
		t.Fatalf("expected decision and target updates for accept, got decision=%d target=%d", q.decisionCalls, q.targetCalls)
	}

	q2 := &riskDecisionQ{}
	h2 := NewHandler(q2, nil)
	if err := h2.persistDecision(w, r, risk, "treat", "ok", sql.NullInt32{}, sql.NullInt32{}); err != nil {
		t.Fatalf("treat persist error: %v", err)
	}
	if q2.targetCalls != 0 {
		t.Fatalf("expected no target write without valid target scores, got %d", q2.targetCalls)
	}
}

func TestDecisionFlowWrappers_InvalidIDs(t *testing.T) {
	h := NewHandler(&riskDecisionQ{}, nil)
	methods := []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
	}{
		{"review-evaluate", h.ReviewEvaluate},
		{"review-step4", h.ReviewStep4},
		{"save-review-step4", h.SaveReviewStep4},
		{"decision-start", h.DecisionStart},
		{"review-decision-start", h.ReviewDecisionStart},
		{"decision-risk", h.DecisionRisk},
		{"review-decision-risk", h.ReviewDecisionRisk},
		{"save-decision-risk", h.SaveDecisionRisk},
		{"save-review-decision-risk", h.SaveReviewDecisionRisk},
		{"review-step2", h.ReviewStep2},
	}
	for _, tc := range methods {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/risks/not-a-uuid", nil)
			r.SetPathValue("id", "not-a-uuid")
			r.SetPathValue("rid", "not-a-uuid")
			w := httptest.NewRecorder()
			tc.call(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", tc.name, w.Code)
			}
		})
	}
}

func TestStep2AssetSearchAndRenderStep2_Errors(t *testing.T) {
	h := NewHandler(&riskDecisionQ{}, nil)
	r1 := httptest.NewRequest(http.MethodGet, "/?asset_q=x&asset_risk_id=not-a-uuid", nil)
	q, res := h.step2AssetSearch(r1)
	if len(q) != 0 || len(res) != 0 {
		t.Fatalf("expected empty search maps for invalid risk ID, got q=%v res=%v", q, res)
	}

	r2 := httptest.NewRequest(http.MethodGet, "/?asset_search_partial=1&asset_risk_id=not-a-uuid", nil)
	r2.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.renderStep2(w, r2, db.RiskAssessment{ID: uuid.New()}, nil, true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 from partial render with bad risk id, got %d", w.Code)
	}
}

func TestLoadDecisionContextAndAssessmentWithRisks_BadInput(t *testing.T) {
	h := NewHandler(&riskDecisionQ{}, nil)
	r := httptest.NewRequest(http.MethodGet, "/risks/not-a-uuid/decision/not-a-uuid", nil)
	r.SetPathValue("id", "not-a-uuid")
	r.SetPathValue("rid", "not-a-uuid")
	w := httptest.NewRecorder()
	if _, _, _, _, ok := h.loadDecisionContext(w, r); ok {
		t.Fatal("expected loadDecisionContext to fail on invalid IDs")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	if _, _, ok := h.loadAssessmentWithRisks(w2, r, "test"); ok {
		t.Fatal("expected loadAssessmentWithRisks to fail on invalid ID")
	}
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w2.Code)
	}
}

func TestCompleteDecisionFlow(t *testing.T) {
	h := NewHandler(&riskDecisionQ{}, nil)
	assessment := db.RiskAssessment{ID: uuid.New(), Name: "A"}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	h.completeDecisionFlow(w, r, assessment, true)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 for review complete, got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodPost, "/", nil)
	h.completeDecisionFlow(w2, r2, assessment, false)
	if w2.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 for submit flow, got %d", w2.Code)
	}
}
