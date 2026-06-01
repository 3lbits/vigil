// Package dashboard renders the compliance overview page.
package dashboard

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/locale"
	"github.com/3lbits/vigil/internal/middleware"
	dashboardtemplates "github.com/3lbits/vigil/internal/modules/dashboard/templates"
	"github.com/3lbits/vigil/internal/ui/layout"
)

type Handler struct {
	q db.Querier
}

func NewHandler(q db.Querier) *Handler {
	return &Handler{q: q}
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) { //nolint:gocognit,funlen,cyclop
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	user, _ := middleware.FromContext(r.Context())
	flags := middleware.ModuleFlagsFromContext(r.Context())
	orgName := middleware.TopOrgNameFromContext(r.Context())
	frameworkFilter := r.URL.Query().Get("framework_type")
	if frameworkFilter != "regulation" && frameworkFilter != "standard" && frameworkFilter != "directive" {
		frameworkFilter = "all"
	}

	stats, err := h.q.GetDashboardStats(r.Context())
	if err != nil {
		slog.Error("dashboard stats", "error", err)
		http.Error(w, "failed to load stats", http.StatusInternalServerError)
		return
	}

	// Build framework summaries — skip entirely when compliance module is off.
	summaries := make([]dashboardtemplates.FrameworkSummary, 0)
	regCount := 0
	stdCount := 0
	dirCount := 0
	subtitle := orgName
	if flags.ComplianceEnabled { //nolint:nestif
		frameworks, fwErr := h.q.ListFrameworks(r.Context())
		if fwErr != nil {
			slog.Error("dashboard frameworks", "error", fwErr)
			http.Error(w, "failed to load frameworks", http.StatusInternalServerError)
			return
		}
		var latestUpdated time.Time
		for _, fw := range frameworks {
			if fw.NotRelevant {
				continue
			}
			if fw.UpdatedAt.After(latestUpdated) {
				latestUpdated = fw.UpdatedAt
			}
			total, err := h.q.CountRequirementsByFramework(r.Context(), fw.ID) //nolint:govet
			if err != nil {
				slog.Warn("count requirements by framework", "framework_id", fw.ID, "error", err)
			}
			covered, err := h.q.CountCoveredRequirementsByFramework(r.Context(), fw.ID)
			if err != nil {
				slog.Warn("count covered requirements by framework", "framework_id", fw.ID, "error", err)
			}
			pct := 0
			if total > 0 {
				pct = int(covered * 100 / total)
			}
			displayName := fw.ShortName
			if displayName == "" {
				displayName = fw.Name
				if fw.Version != "" {
					displayName += " " + fw.Version
				}
			}
			ref := ""
			if fw.ShortName != "" {
				ref = fw.Name
				if fw.Version != "" {
					ref += " " + fw.Version
				}
			}
			summaries = append(summaries, dashboardtemplates.FrameworkSummary{
				ID:       fw.ID.String(),
				Name:     displayName,
				Ref:      ref,
				Type:     fw.FrameworkType,
				Total:    total,
				Coverage: pct,
			})
			switch fw.FrameworkType {
			case "standard":
				stdCount++
			case "directive":
				dirCount++
			default:
				regCount++
			}
		}
		if !latestUpdated.IsZero() {
			subtitle = orgName + " · " + locale.T(r.Context(), "dashboard_subtitle_updated") + " " + latestUpdated.Format("2 Jan 2006")
		}
	}

	// Activities queries — skip when activities module is off.
	var recentActivities []db.ListRecentActivitiesRow
	var myActivities []db.ListActivitiesForUserRow
	if flags.ActivitiesEnabled {
		recentActivities, err = h.q.ListRecentActivities(r.Context())
		if err != nil {
			slog.Error("dashboard recent activities", "error", err)
			recentActivities = nil
		}
	}

	var myMeasures []db.Measure
	if uid, parseErr := uuid.Parse(user.ID); parseErr == nil {
		nullUID := uuid.NullUUID{UUID: uid, Valid: true}
		if flags.ActivitiesEnabled {
			myActivities, err = h.q.ListActivitiesForUser(r.Context(), nullUID)
			if err != nil {
				slog.Error("dashboard my activities", "error", err)
				myActivities = nil
			}
		}
		myMeasures, err = h.q.ListMeasuresForUser(r.Context(), nullUID)
		if err != nil {
			slog.Error("dashboard my measures", "error", err)
			myMeasures = nil
		}
	}

	// Risk queries — skip when risk module is off.
	var riskStats dashboardtemplates.RiskStats
	if flags.RiskEnabled {
		riskStats = h.buildRiskStats(r)
	}

	vmStats := dashboardtemplates.Stats{
		FrameworksCount:            stats.FrameworksCount,
		RequirementsCount:          stats.RequirementsCount,
		MeasuresCount:              stats.MeasuresCount,
		ImplementedCount:           stats.ImplementedCount,
		CoveredRequirementsCount:   stats.CoveredRequirementsCount,
		OverdueActivitiesCount:     stats.OverdueActivitiesCount,
		ActivitiesDueThisWeekCount: stats.ActivitiesDueThisWeekCount,
	}
	visibleFrameworks := summaries
	if frameworkFilter != "all" {
		visibleFrameworks = make([]dashboardtemplates.FrameworkSummary, 0, len(summaries))
		for _, fw := range summaries {
			if fw.Type == frameworkFilter {
				visibleFrameworks = append(visibleFrameworks, fw)
			}
		}
	}

	httputil.Render(w, r, layout.Layout("page_dashboard_title", subtitle, "dashboard", user,
		dashboardtemplates.DashboardContent(
			vmStats,
			visibleFrameworks,
			frameworkFilter,
			len(summaries),
			regCount,
			stdCount,
			dirCount,
			recentActivities,
			myActivities,
			myMeasures,
			riskStats,
			flags,
		),
	))
}

func buildOrgFilterOptions(orgs []db.Organization) []dashboardtemplates.OrgFilterOption {
	children := make(map[uuid.UUID][]db.Organization)
	roots := make([]db.Organization, 0)
	for _, org := range orgs {
		if org.ParentID.Valid {
			children[org.ParentID.UUID] = append(children[org.ParentID.UUID], org)
			continue
		}
		roots = append(roots, org)
	}
	options := make([]dashboardtemplates.OrgFilterOption, 0, len(orgs))
	var walk func(parent db.Organization, depth int)
	walk = func(parent db.Organization, depth int) {
		options = append(options, dashboardtemplates.OrgFilterOption{
			ID:    parent.ID.String(),
			Name:  parent.Name,
			Depth: depth,
		})
		for _, child := range children[parent.ID] {
			walk(child, depth+1)
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}
	return options
}

func parseRiskOrgFilter(raw string, orgs []db.Organization) (uuid.UUID, string, map[uuid.UUID]struct{}) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, "", nil
	}
	orgByID := make(map[uuid.UUID]db.Organization, len(orgs))
	for _, org := range orgs {
		orgByID[org.ID] = org
	}
	selected, ok := orgByID[id]
	if !ok {
		return uuid.Nil, "", nil
	}
	return id, selected.Name, descendantOrgIDs(id, orgs)
}

func descendantOrgIDs(root uuid.UUID, orgs []db.Organization) map[uuid.UUID]struct{} {
	children := make(map[uuid.UUID][]uuid.UUID)
	for _, org := range orgs {
		if org.ParentID.Valid {
			children[org.ParentID.UUID] = append(children[org.ParentID.UUID], org.ID)
		}
	}
	out := map[uuid.UUID]struct{}{root: {}}
	queue := []uuid.UUID{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, childID := range children[cur] {
			if _, exists := out[childID]; exists {
				continue
			}
			out[childID] = struct{}{}
			queue = append(queue, childID)
		}
	}
	return out
}

func riskInOrgScope(orgID uuid.NullUUID, allowed map[uuid.UUID]struct{}) bool {
	if len(allowed) == 0 {
		return true
	}
	if !orgID.Valid {
		return false
	}
	_, ok := allowed[orgID.UUID]
	return ok
}

func toListTopRiskRow(r db.ListAllRisksRow) db.ListTopRisksRow {
	return db.ListTopRisksRow{
		ID:                   r.ID,
		AssessmentID:         r.AssessmentID,
		Name:                 r.Name,
		LikelihoodCurrent:    r.LikelihoodCurrent,
		ConsequenceCurrent:   r.ConsequenceCurrent,
		LikelihoodTarget:     r.LikelihoodTarget,
		ConsequenceTarget:    r.ConsequenceTarget,
		RiskDecision:         r.RiskDecision,
		Status:               r.Status,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
		Description:          r.Description,
		LikelihoodReasoning:  r.LikelihoodReasoning,
		ConsequenceReasoning: r.ConsequenceReasoning,
		OwnerID:              r.OwnerID,
		DecisionNotes:        r.DecisionNotes,
		RefNum:               r.RefNum,
		AssessmentName:       r.AssessmentName,
	}
}

func (h *Handler) buildRiskStats(r *http.Request) dashboardtemplates.RiskStats {
	orgs, err := h.q.ListOrganizations(r.Context())
	if err != nil {
		slog.Warn("dashboard organizations for risk filter", "error", err)
		orgs = nil
	}
	orgOptions := buildOrgFilterOptions(orgs)
	selectedOrgID, selectedOrgName, allowedOrgIDs := parseRiskOrgFilter(r.URL.Query().Get("risk_org_id"), orgs)
	selectedOrgFilter := selectedOrgID.String()
	if selectedOrgID == uuid.Nil {
		selectedOrgFilter = ""
	}

	assessments, err := h.q.ListRiskAssessments(r.Context())
	if err != nil {
		slog.Warn("dashboard risk assessments", "error", err)
		assessments = nil
	}
	assessmentOrg := make(map[uuid.UUID]uuid.NullUUID, len(assessments))
	for _, a := range assessments {
		assessmentOrg[a.ID] = a.OrgID
	}

	allRisks, err := h.q.ListAllRisks(r.Context())
	if err != nil {
		slog.Warn("dashboard all risks", "error", err)
		allRisks = nil
	}
	reviewNeededCount, reviewQueue := h.buildRiskReviewQueue(r, assessmentOrg, allowedOrgIDs)
	gs, _ := h.q.GetRiskGlobalSettings(r.Context())
	thresholds := layout.RiskThresholds{LowMax: int(gs.LowMax), HighMin: int(gs.HighMin)}
	if thresholds.LowMax == 0 {
		thresholds = layout.DefaultThresholds()
	}
	matrixCounts, topRisks, redCount, yellowCount, greenCount := aggregateRiskStats(allRisks, assessmentOrg, allowedOrgIDs, thresholds)
	total := redCount + yellowCount + greenCount
	return dashboardtemplates.RiskStats{
		Total:       total,
		RedCount:    redCount,
		YellowCount: yellowCount,
		GreenCount:  greenCount,
		Matrix:      layout.BuildRiskMatrixCellsT(matrixCounts, thresholds),
		TopRisks:    topRisks,
		ReviewNeededCount: reviewNeededCount,
		ReviewQueue:       reviewQueue,
		LowMax:      thresholds.LowMax,
		HighMin:     thresholds.HighMin,
		OrgOptions:  orgOptions,
		OrgSelected: selectedOrgFilter,
		OrgName:     selectedOrgName,
	}
}

func (h *Handler) buildRiskReviewQueue(
	r *http.Request,
	assessmentOrg map[uuid.UUID]uuid.NullUUID,
	allowedOrgIDs map[uuid.UUID]struct{},
) (int64, []dashboardtemplates.RiskReviewItem) {
	user, _ := middleware.FromContext(r.Context())
	if user.Role == "admin" || user.Role == "editor" {
		return h.buildAdminRiskReviewQueue(r, assessmentOrg, allowedOrgIDs)
	}
	uid, err := uuid.Parse(user.ID)
	if err != nil {
		return 0, nil
	}
	return h.buildUserRiskReviewQueue(r, assessmentOrg, allowedOrgIDs, uid)
}

func (h *Handler) buildAdminRiskReviewQueue(
	r *http.Request,
	assessmentOrg map[uuid.UUID]uuid.NullUUID,
	allowedOrgIDs map[uuid.UUID]struct{},
) (int64, []dashboardtemplates.RiskReviewItem) {
	rows, err := h.q.ListRiskReviewQueue(r.Context())
	if err != nil {
		slog.Warn("dashboard risk review queue", "error", err)
		return 0, nil
	}
	items := make([]dashboardtemplates.RiskReviewItem, 0, 5)
	var count int64
	for _, row := range rows {
		if !riskInOrgScope(assessmentOrg[row.AssessmentID], allowedOrgIDs) {
			continue
		}
		count++
		if len(items) < 5 {
			items = append(items, reviewQueueItemFromAdminRow(row))
		}
	}
	return count, items
}

func (h *Handler) buildUserRiskReviewQueue(
	r *http.Request,
	assessmentOrg map[uuid.UUID]uuid.NullUUID,
	allowedOrgIDs map[uuid.UUID]struct{},
	uid uuid.UUID,
) (int64, []dashboardtemplates.RiskReviewItem) {
	rows, err := h.q.ListRiskReviewQueueForUser(r.Context(), uid)
	if err != nil {
		slog.Warn("dashboard risk review queue for user", "error", err)
		return 0, nil
	}
	items := make([]dashboardtemplates.RiskReviewItem, 0, 5)
	var count int64
	for _, row := range rows {
		if !riskInOrgScope(assessmentOrg[row.AssessmentID], allowedOrgIDs) {
			continue
		}
		count++
		if len(items) < 5 {
			items = append(items, reviewQueueItemFromUserRow(row))
		}
	}
	return count, items
}

func reviewQueueItemFromAdminRow(row db.ListRiskReviewQueueRow) dashboardtemplates.RiskReviewItem {
	return buildRiskReviewItem(
		row.ID,
		row.AssessmentID,
		row.Name,
		row.AssessmentName,
		row.LikelihoodCurrent,
		row.ConsequenceCurrent,
		row.ReviewDue,
	)
}

func reviewQueueItemFromUserRow(row db.ListRiskReviewQueueForUserRow) dashboardtemplates.RiskReviewItem {
	return buildRiskReviewItem(
		row.ID,
		row.AssessmentID,
		row.Name,
		row.AssessmentName,
		row.LikelihoodCurrent,
		row.ConsequenceCurrent,
		row.ReviewDue,
	)
}

func buildRiskReviewItem(
	riskID uuid.UUID,
	assessmentID uuid.UUID,
	name string,
	assessmentName string,
	likelihood sql.NullInt32,
	consequence sql.NullInt32,
	reviewDue sql.NullTime,
) dashboardtemplates.RiskReviewItem {
	hasScore := likelihood.Valid && consequence.Valid
	score := 0
	if hasScore {
		score = int(likelihood.Int32) * int(consequence.Int32)
	}
	return dashboardtemplates.RiskReviewItem{
		RiskID:          riskID.String(),
		AssessmentID:    assessmentID.String(),
		Name:            name,
		AssessmentName:  assessmentName,
		CurrentScore:    score,
		HasCurrentScore: hasScore,
		ReviewDue:       reviewDue.Time,
		HasReviewDue:    reviewDue.Valid,
	}
}

func aggregateRiskStats(
	allRisks []db.ListAllRisksRow,
	assessmentOrg map[uuid.UUID]uuid.NullUUID,
	allowedOrgIDs map[uuid.UUID]struct{},
	thresholds layout.RiskThresholds,
) (map[[2]int]int, []db.ListTopRisksRow, int64, int64, int64) {
	matrixCounts := make(map[[2]int]int)
	topRisks := make([]db.ListTopRisksRow, 0, 5)
	var redCount, yellowCount, greenCount int64
	for _, risk := range allRisks {
		if !isScoredRisk(risk) {
			continue
		}
		if !riskInOrgScope(assessmentOrg[risk.AssessmentID], allowedOrgIDs) {
			continue
		}
		likelihood := int(risk.LikelihoodCurrent.Int32)
		consequence := int(risk.ConsequenceCurrent.Int32)
		score := likelihood * consequence
		matrixCounts[[2]int{likelihood, consequence}]++
		switch {
		case score >= thresholds.HighMin:
			redCount++
		case score > thresholds.LowMax:
			yellowCount++
		default:
			greenCount++
		}
		if len(topRisks) < 5 {
			topRisks = append(topRisks, toListTopRiskRow(risk))
		}
	}
	return matrixCounts, topRisks, redCount, yellowCount, greenCount
}

func isScoredRisk(risk db.ListAllRisksRow) bool {
	return risk.LikelihoodCurrent.Valid && risk.ConsequenceCurrent.Valid
}
