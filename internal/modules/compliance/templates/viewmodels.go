package compliancetemplates

import "github.com/3lbits/vigil/internal/db"

// FrameworkVM is the view model for displaying a framework in the list.
type FrameworkVM struct {
	Framework    db.Framework
	Coverage     int
	TotalReqs    int64
	CoveredReqs  int64
	Requirements []RequirementVM
}

// FrameworkDetailVM is the view model for the framework detail page.
type FrameworkDetailVM struct {
	Framework                  db.Framework
	Requirements               []db.Requirement
	RequirementImplementations []RequirementVM
	CoveredReqs                int64
	CanEdit                    bool
	AuditLog                   []db.ListAuditLogForFrameworkRow
}

// RequirementVM is the view model for a requirement with its linked measures.
type RequirementVM struct {
	Requirement db.Requirement
	Measures    []db.ListMeasuresForRequirementRow
}

// RequirementDetailVM is the view model for the requirement detail page.
type RequirementDetailVM struct {
	Requirement    db.Requirement
	FrameworkName  string
	FrameworkShort string
	Measures       []db.ListMeasuresForRequirementRow
	CanEdit        bool
	AuditLog       []db.ListAuditLogForRequirementRow
}

// RequirementEditVM is the view model for the requirement edit page with links management.
type RequirementEditVM struct {
	Requirement    db.Requirement
	LinkedMeasures []db.ListMeasuresForRequirementRow
	AuditLog       []db.ListAuditLogForRequirementRow
}

// ImportResult holds the result of a CSV import operation.
type ImportResult struct {
	Count  int
	Errors []string
}
