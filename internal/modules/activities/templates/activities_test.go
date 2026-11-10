package activitiestemplates

import (
	"bytes"
	"context"
	"database/sql"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/3lbits/vigil/internal/db"
)

func TestActivityDetailUsesConfiguredRiskThresholds(t *testing.T) {
	risk := db.Risk{
		Name:               "Risk",
		LikelihoodCurrent:  sql.NullInt32{Int32: 1, Valid: true},
		ConsequenceCurrent: sql.NullInt32{Int32: 5, Valid: true},
		LikelihoodTarget:   sql.NullInt32{Int32: 2, Valid: true},
		ConsequenceTarget:  sql.NullInt32{Int32: 5, Valid: true},
	}
	var out bytes.Buffer
	if err := ActivityDetail(db.GetActivityRow{}, false, "", "", nil, nil, []db.Risk{risk}, 4, 10).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(&out)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Find(`.ops-risk-score[data-tone="warning"]`).Length() != 1 ||
		doc.Find(`.ops-risk-score[data-tone="danger"]`).Length() != 1 {
		t.Error("linked risk scores do not use configured thresholds")
	}
}
