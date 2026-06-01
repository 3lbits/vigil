package activities

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/modregistry"
)

var errNoDevUsersForActivities = errors.New("no dev stub users found; run stub-user seeding first")

var activitySeeds = []struct {
	Title        string
	Description  string
	ActivityType string
	Recurrence   string
	Priority     string
	Kind         string
}{
	{
		Title:        "Run monthly privileged access review",
		Description:  "Review admin memberships and remove stale elevated access.",
		ActivityType: "recurring",
		Recurrence:   "monthly",
		Priority:     "high",
		Kind:         "review",
	},
	{
		Title:        "Execute annual incident response tabletop",
		Description:  "Practice coordinated response with engineering, security, and legal.",
		ActivityType: "recurring",
		Recurrence:   "annual",
		Priority:     "medium",
		Kind:         "training",
	},
	{
		Title:        "Perform ad-hoc penetration test follow-up",
		Description:  "Validate remediation for externally reported penetration test findings.",
		ActivityType: "one_off",
		Recurrence:   "ad_hoc",
		Priority:     "high",
		Kind:         "assessment",
	},
	{
		Title:        "Track critical patch remediation sprint",
		Description:  "Coordinate patch rollout and verify no high-risk assets are missed.",
		ActivityType: "one_off",
		Recurrence:   "none",
		Priority:     "high",
		Kind:         "remediation",
	},
	{
		Title:        "Quarterly evidence collection audit",
		Description:  "Sample control evidence and ensure retention and traceability.",
		ActivityType: "recurring",
		Recurrence:   "tertially",
		Priority:     "medium",
		Kind:         "audit",
	},
}

func (activitiesModule) DevSeed(ctx context.Context, deps modregistry.Dependencies) error {
	existingByTitle, err := listExistingActivitiesByTitle(ctx, deps.Queries)
	if err != nil {
		return err
	}

	measures, err := deps.Queries.ListMeasures(ctx)
	if err != nil {
		return fmt.Errorf("list measures: %w", err)
	}
	users, err := deps.Queries.ListDevStubUsers(ctx)
	if err != nil {
		return fmt.Errorf("list dev stub users: %w", err)
	}
	if len(users) == 0 {
		return errNoDevUsersForActivities
	}

	seeded, err := ensureSeedActivities(ctx, deps.Queries, existingByTitle, users, measures, time.Now())
	if err != nil {
		return err
	}
	return applyActivityStatusMix(ctx, deps.Queries, users, seeded)
}

func listExistingActivitiesByTitle(ctx context.Context, q db.Querier) (map[string]db.ListActivitiesRow, error) {
	existing, err := q.ListActivities(ctx)
	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	byTitle := make(map[string]db.ListActivitiesRow, len(existing))
	for _, activity := range existing {
		byTitle[activity.Title] = activity
	}
	return byTitle, nil
}

func ensureSeedActivities(
	ctx context.Context,
	q db.Querier,
	byTitle map[string]db.ListActivitiesRow,
	users []db.User,
	measures []db.Measure,
	now time.Time,
) ([]db.Activity, error) {
	seeded := make([]db.Activity, 0, len(activitySeeds))
	for i, seed := range activitySeeds {
		if existingActivity, ok := byTitle[seed.Title]; ok {
			seeded = append(seeded, db.Activity{
				ID:     existingActivity.ID,
				Status: existingActivity.Status,
			})
			continue
		}
		var measureID uuid.NullUUID
		if len(measures) > 0 {
			measureID = uuid.NullUUID{UUID: measures[i%len(measures)].ID, Valid: true}
		}
		user := users[i%len(users)]
		created, createErr := q.CreateActivity(ctx, db.CreateActivityParams{
			MeasureID:    measureID,
			Title:        seed.Title,
			Description:  seed.Description,
			ActivityType: seed.ActivityType,
			Recurrence:   seed.Recurrence,
			Priority:     seed.Priority,
			Kind:         seed.Kind,
			Owner:        user.Name,
			AssigneeID:   uuid.NullUUID{UUID: user.ID, Valid: true},
			DueDate:      sql.NullTime{Time: now.Add(time.Duration(i-3) * 72 * time.Hour), Valid: true},
		})
		if createErr != nil {
			return nil, fmt.Errorf("create activity %q: %w", seed.Title, createErr)
		}
		seeded = append(seeded, created)
	}
	return seeded, nil
}

func applyActivityStatusMix(ctx context.Context, q db.Querier, users []db.User, seeded []db.Activity) error {
	if err := ensureSecondActivityInProgress(ctx, q, seeded); err != nil {
		return err
	}
	if err := ensureThirdActivityCompleted(ctx, q, users, seeded); err != nil {
		return err
	}
	if err := reopenFourthActivity(ctx, q, seeded); err != nil {
		return err
	}
	return nil
}

func ensureSecondActivityInProgress(ctx context.Context, q db.Querier, seeded []db.Activity) error {
	if len(seeded) <= 1 || seeded[1].Status == "in_progress" {
		return nil
	}
	if _, err := q.MarkActivityInProgress(ctx, seeded[1].ID); err != nil {
		return fmt.Errorf("mark activity in_progress: %w", err)
	}
	return nil
}

func ensureThirdActivityCompleted(ctx context.Context, q db.Querier, users []db.User, seeded []db.Activity) error {
	if len(seeded) <= 2 || seeded[2].Status == "completed" {
		return nil
	}
	if _, err := q.CompleteActivity(ctx, db.CompleteActivityParams{
		ID:          seeded[2].ID,
		CompletedBy: users[0].Name,
		Notes:       "Completed during dev seed.",
		EvidenceUrl: "https://example.invalid/evidence/dev-seed",
	}); err != nil {
		return fmt.Errorf("complete activity: %w", err)
	}
	return nil
}

func reopenFourthActivity(ctx context.Context, q db.Querier, seeded []db.Activity) error {
	if len(seeded) <= 3 {
		return nil
	}
	if seeded[3].Status != "in_progress" {
		if _, err := q.MarkActivityInProgress(ctx, seeded[3].ID); err != nil {
			return fmt.Errorf("mark activity in_progress (second): %w", err)
		}
	}
	if _, err := q.ReopenActivity(ctx, seeded[3].ID); err != nil {
		return fmt.Errorf("reopen activity: %w", err)
	}
	return nil
}
