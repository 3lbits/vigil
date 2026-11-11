package dashboardtemplates

import (
	"strings"
	"time"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/ui2"
)

type Stats struct {
	FrameworksCount            int64
	RequirementsCount          int64
	MeasuresCount              int64
	ImplementedCount           int64
	CoveredRequirementsCount   int64
	OverdueActivitiesCount     int64
	ActivitiesDueThisWeekCount int64
}

type FrameworkSummary struct {
	ID       string
	Name     string
	Ref      string
	Type     string
	Total    int64
	Coverage int
}

func (s Stats) CoveredPct() int {
	if s.RequirementsCount == 0 {
		return 0
	}
	return int(s.CoveredRequirementsCount * 100 / s.RequirementsCount)
}

func (s Stats) UncoveredRequirementsCount() int64 {
	return s.RequirementsCount - s.CoveredRequirementsCount
}

func riskDisplayName(name, assessment string) string {
	return strings.TrimPrefix(name, assessment+": ")
}

func zeroFrameworkCount(fws []FrameworkSummary) int {
	n := 0
	for _, fw := range fws {
		if fw.Coverage == 0 {
			n++
		}
	}
	return n
}

func formatActivityTime(t time.Time) string {
	return t.Format("2 Jan")
}

type RiskStats struct {
	Total             int64
	RedCount          int64
	YellowCount       int64
	GreenCount        int64
	Matrix            []ui2.RiskMatrixCell
	TopRisks          []db.ListTopRisksRow
	ReviewNeededCount int64
	ReviewQueue       []RiskReviewItem
	LowMax            int
	HighMin           int
	OrgOptions        []OrgFilterOption
	OrgSelected       string
	OrgName           string
}

type RiskReviewItem struct {
	RiskID          string
	AssessmentID    string
	Name            string
	AssessmentName  string
	CurrentScore    int
	HasCurrentScore bool
	ReviewDue       time.Time
	HasReviewDue    bool
}

type OrgFilterOption struct {
	ID    string
	Name  string
	Depth int
}

func orgFilterOptionLabel(opt OrgFilterOption) string {
	if opt.Depth <= 0 {
		return opt.Name
	}
	return strings.Repeat("↳ ", opt.Depth) + opt.Name
}
