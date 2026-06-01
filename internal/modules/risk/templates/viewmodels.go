package risktemplates

import (
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/ui/layout"
	"github.com/google/uuid"
)

type AssessmentListVM struct {
	Assessment  db.RiskAssessment
	RiskCount   int64
	RedCount    int64
	YellowCount int64
}

type AssessmentListPageVM struct {
	Assessments   []AssessmentListVM
	Flash         string
	FlashType     string
	CurrentUserID string
}

type WizardStep1VM struct {
	Assessment         db.RiskAssessment
	Users              []db.User
	Orgs               []db.Organization
	Participants       []db.User
	ParticipantQuery   string
	ParticipantResults []db.User
	Assets             []db.Asset
	AssetQuery         string
	AssetResults       []db.Asset
	IsNew              bool
	Flash              string
	AcceptanceCriteria string
}

type WizardStep2VM struct {
	Assessment   db.RiskAssessment
	Risks        []db.Risk
	Users        []db.User
	LinkedAssets map[uuid.UUID][]db.Asset
	AssetQueries map[uuid.UUID]string
	AssetResults map[uuid.UUID][]db.Asset
	IsReview     bool
	Flash        string
}

type WizardStep3VM struct {
	Assessment  db.RiskAssessment
	Risks       []db.Risk
	IsReview    bool
	LowMax      int
	HighMin     int
	ScaleLabels []db.RiskScaleLabel
}

type EvaluationVM struct {
	Assessment         db.RiskAssessment
	Risks              []db.Risk
	IsReview           bool
	AcceptanceCriteria string
	LowMax             int
	HighMin            int
}

type WizardStep4VM struct {
	Assessment     db.RiskAssessment
	Risks          []db.Risk
	RiskMeasures   map[uuid.UUID][]db.Measure
	RiskActivities map[uuid.UUID][]db.Activity
	Users          []db.User
	IsReview       bool
	OverdueRisks   map[uuid.UUID]bool
	LowMax         int
	HighMin        int
}

type DecisionVM struct {
	Assessment db.RiskAssessment
	Risk       db.Risk
	Risks      []db.Risk

	Assets     []db.Asset
	Measures   []db.Measure
	Activities []db.Activity
	Users      []db.User
	Overdue    bool

	AcceptanceCriteria string
	LowMax             int
	HighMin            int

	Position int
	Total    int

	PrevRiskID uuid.NullUUID
	NextRiskID uuid.NullUUID

	IsReview  bool
	CanAccept bool
	OwnerName string
}

type AssessmentDetailVM struct {
	Assessment      db.RiskAssessment
	Risks           []db.Risk
	CurrentMatrix   []layout.RiskMatrixCell
	TargetMatrix    []layout.RiskMatrixCell
	RiskMeasures    map[uuid.UUID][]db.Measure
	RiskActivities  map[uuid.UUID][]db.Activity
	Flash           string
	FlashType       string
	OwnerName       string
	OrgName         string
	Participants    []db.User
	LowMax          int
	HighMin         int
	CurrentUserID   string
	CanTogglePublic bool
	AuditLog        []db.ListAuditLogForAssessmentRow
}

type RiskDetailVM struct {
	Assessment          db.RiskAssessment
	Risk                db.Risk
	Measures            []db.Measure
	Activities          []db.Activity
	Assets              []db.Asset
	AssetQuery          string
	AssetResults        []db.Asset
	LowMax              int
	HighMin             int
	ReviewEvents        []db.ListRiskReassessmentEventsRow
	AuditLog            []db.ListAuditLogForRiskRow
	AssessedByName      string
	ReviewRationale     string
	ReassessmentMessage string
	ReassessmentError   bool
}
