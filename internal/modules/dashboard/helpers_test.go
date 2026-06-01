package dashboard

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/ui/layout"
)

func TestDescendantOrgIDsAndRiskScope(t *testing.T) {
	root := uuid.New()
	child := uuid.New()
	grandchild := uuid.New()
	other := uuid.New()
	orgs := []db.Organization{
		{ID: root, Name: "Root"},
		{ID: child, Name: "Child", ParentID: uuid.NullUUID{UUID: root, Valid: true}},
		{ID: grandchild, Name: "Grandchild", ParentID: uuid.NullUUID{UUID: child, Valid: true}},
		{ID: other, Name: "Other"},
	}

	allowed := descendantOrgIDs(root, orgs)
	if len(allowed) != 3 {
		t.Fatalf("expected 3 org IDs, got %d", len(allowed))
	}
	if !riskInOrgScope(uuid.NullUUID{UUID: child, Valid: true}, allowed) {
		t.Fatal("expected child org to be in scope")
	}
	if riskInOrgScope(uuid.NullUUID{UUID: other, Valid: true}, allowed) {
		t.Fatal("did not expect unrelated org in scope")
	}
	if riskInOrgScope(uuid.NullUUID{}, allowed) {
		t.Fatal("did not expect null org in scope")
	}
	if !riskInOrgScope(uuid.NullUUID{}, nil) {
		t.Fatal("empty allowed set should allow all")
	}
}

func TestParseRiskOrgFilter(t *testing.T) {
	id := uuid.New()
	orgs := []db.Organization{{ID: id, Name: "Acme"}}

	gotID, gotName, gotAllowed := parseRiskOrgFilter(id.String(), orgs)
	if gotID != id || gotName != "Acme" || len(gotAllowed) != 1 {
		t.Fatalf("unexpected parsed result: id=%v name=%q allowed=%d", gotID, gotName, len(gotAllowed))
	}

	nilID, nilName, nilAllowed := parseRiskOrgFilter("not-a-uuid", orgs)
	if nilID != uuid.Nil || nilName != "" || nilAllowed != nil {
		t.Fatalf("invalid ID should return zero values, got id=%v name=%q", nilID, nilName)
	}
}

func TestBuildRiskReviewItemAndRowAdapters(t *testing.T) {
	riskID := uuid.New()
	assessmentID := uuid.New()
	due := time.Now()
	adminRow := db.ListRiskReviewQueueRow{
		ID: riskID, AssessmentID: assessmentID, Name: "Risk A", AssessmentName: "A1",
		LikelihoodCurrent:  sql.NullInt32{Int32: 3, Valid: true},
		ConsequenceCurrent: sql.NullInt32{Int32: 4, Valid: true},
		ReviewDue:          sql.NullTime{Time: due, Valid: true},
	}
	item := reviewQueueItemFromAdminRow(adminRow)
	if !item.HasCurrentScore || item.CurrentScore != 12 || !item.HasReviewDue {
		t.Fatalf("unexpected admin item: %#v", item)
	}

	userRow := db.ListRiskReviewQueueForUserRow{
		ID: riskID, AssessmentID: assessmentID, Name: "Risk B", AssessmentName: "A2",
	}
	item2 := reviewQueueItemFromUserRow(userRow)
	if item2.HasCurrentScore || item2.CurrentScore != 0 {
		t.Fatalf("unexpected user item: %#v", item2)
	}
}

func TestAggregateRiskStatsAndHelpers(t *testing.T) {
	assessmentID := uuid.New()
	riskID := uuid.New()
	rows := []db.ListAllRisksRow{
		{
			ID:                 riskID,
			AssessmentID:       assessmentID,
			Name:               "R1",
			LikelihoodCurrent:  sql.NullInt32{Int32: 4, Valid: true},
			ConsequenceCurrent: sql.NullInt32{Int32: 4, Valid: true},
		},
		{
			ID:                 uuid.New(),
			AssessmentID:       assessmentID,
			Name:               "R2",
			LikelihoodCurrent:  sql.NullInt32{Valid: false},
			ConsequenceCurrent: sql.NullInt32{Int32: 2, Valid: true},
		},
	}
	assessmentOrg := map[uuid.UUID]uuid.NullUUID{
		assessmentID: {UUID: uuid.New(), Valid: true},
	}
	thresholds := layout.RiskThresholds{LowMax: 5, HighMin: 12}
	matrix, top, red, yellow, green := aggregateRiskStats(rows, assessmentOrg, nil, thresholds)
	if red != 1 || yellow != 0 || green != 0 {
		t.Fatalf("unexpected counters red=%d yellow=%d green=%d", red, yellow, green)
	}
	if len(matrix) != 1 || matrix[[2]int{4, 4}] != 1 {
		t.Fatalf("unexpected matrix counts: %#v", matrix)
	}
	if len(top) != 1 || top[0].ID != riskID {
		t.Fatalf("unexpected top risks: %#v", top)
	}

	if !isScoredRisk(rows[0]) || isScoredRisk(rows[1]) {
		t.Fatal("isScoredRisk did not match expected scoring validity")
	}

	adapted := toListTopRiskRow(rows[0])
	if adapted.ID != rows[0].ID || adapted.AssessmentID != rows[0].AssessmentID {
		t.Fatalf("adapter mismatch: %#v", adapted)
	}
}
