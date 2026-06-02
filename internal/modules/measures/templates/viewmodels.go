package measurestemplates

import "github.com/3lbits/vigil/internal/db"

// MeasureVM is the view model for displaying a measure in the list.
type MeasureVM struct {
	Measure      db.Measure
	Frameworks   []string // framework short names this measure covers
	HasRiskLinks bool     // linked to at least one risk
	OwnerDisplay string
}

// MeasureEditVM is the view model for the measure edit page with links management.
type MeasureEditVM struct {
	Measure    db.Measure
	LinkedReqs []db.ListRequirementsForMeasureRow
	Users      []db.User
	Links      []db.MeasureLink
	AuditLog   []db.ListAuditLogForMeasureRow
}

// MeasureDetailVM is the view model for the measure detail page.
type MeasureDetailVM struct {
	Measure     db.Measure
	LinkedReqs  []db.ListRequirementsForMeasureRow
	Activities  []db.ListActivitiesForMeasureRow
	Links       []db.MeasureLink
	LinkedRisks []db.Risk
	CanEdit     bool
	AuditLog    []db.ListAuditLogForMeasureRow
}
