package measures

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/modregistry"
)

var errNoDevStubUsers = errors.New("no dev stub users found; run stub-user seeding first")

var measureSeeds = []struct {
	Name        string
	Description string
	Category    string
	Owner       string
	Status      string
}{
	{
		Name:        "Enforce MFA for privileged access",
		Description: "Require phishing-resistant MFA for administrators and elevated operators.",
		Category:    "technical",
		Owner:       "Security",
		Status:      "implemented",
	},
	{
		Name:        "Quarterly vulnerability scanning",
		Description: "Run authenticated scans on critical systems and track remediation SLAs.",
		Category:    "process",
		Owner:       "Engineering",
		Status:      "in_progress",
	},
	{
		Name:        "Security awareness and phishing drills",
		Description: "Deliver recurring awareness training with measurable simulation outcomes.",
		Category:    "administrative",
		Owner:       "Compliance",
		Status:      "planned",
	},
	{
		Name:        "Centralized audit logging and SIEM alerts",
		Description: "Forward critical logs to SIEM and alert on suspicious privileged actions.",
		Category:    "technical",
		Owner:       "Platform",
		Status:      "implemented",
	},
	{
		Name:        "Legacy FTP decommission",
		Description: "Retire legacy file transfer process after migration to managed secure exchange.",
		Category:    "process",
		Owner:       "Operations",
		Status:      "deprecated",
	},
}

func (measuresModule) DevSeed(ctx context.Context, deps modregistry.Dependencies) error {
	users, err := deps.Queries.ListDevStubUsers(ctx)
	if err != nil {
		return fmt.Errorf("list dev stub users: %w", err)
	}
	if len(users) == 0 {
		return errNoDevStubUsers
	}

	seeded, err := ensureMeasures(ctx, deps.Queries, users)
	if err != nil {
		return err
	}
	if err := linkMeasuresToRequirements(ctx, deps.Queries, seeded); err != nil {
		return err
	}
	return nil
}

func ensureMeasures(ctx context.Context, q db.Querier, users []db.User) ([]db.Measure, error) {
	existing, err := q.ListMeasures(ctx)
	if err != nil {
		return nil, fmt.Errorf("list measures: %w", err)
	}
	byName := make(map[string]db.Measure, len(existing))
	for _, measure := range existing {
		byName[measure.Name] = measure
	}

	seeded := make([]db.Measure, 0, len(measureSeeds))
	for i := 0; i < len(measureSeeds); i++ {
		measure, createErr := ensureSingleMeasure(ctx, q, byName, users, i)
		if createErr != nil {
			return nil, createErr
		}
		seeded = append(seeded, measure)
	}
	return seeded, nil
}

func ensureSingleMeasure(
	ctx context.Context,
	q db.Querier,
	byName map[string]db.Measure,
	users []db.User,
	idx int,
) (db.Measure, error) {
	seed := measureSeeds[idx]
	if measure, ok := byName[seed.Name]; ok {
		return measure, nil
	}
	user := users[idx%len(users)]
	created, err := q.CreateMeasure(ctx, db.CreateMeasureParams{
		Name:        seed.Name,
		Description: seed.Description,
		Category:    seed.Category,
		Owner:       seed.Owner,
		AssigneeID:  uuid.NullUUID{UUID: user.ID, Valid: true},
		Status:      seed.Status,
	})
	if err != nil {
		return db.Measure{}, fmt.Errorf("create measure %q: %w", seed.Name, err)
	}
	return created, nil
}

func linkMeasuresToRequirements(ctx context.Context, q db.Querier, seeded []db.Measure) error {
	frameworks, err := q.ListFrameworks(ctx)
	if err != nil {
		return fmt.Errorf("list frameworks for linking: %w", err)
	}
	if len(frameworks) == 0 {
		return nil
	}
	for i := 0; i < len(seeded); i++ {
		framework := frameworks[i%len(frameworks)]
		reqs, reqErr := q.ListRequirementsByFramework(ctx, framework.ID)
		if reqErr != nil {
			return fmt.Errorf("list requirements for framework %s: %w", framework.ShortName, reqErr)
		}
		if len(reqs) == 0 {
			continue
		}
		req := reqs[i%len(reqs)]
		if err := q.LinkMeasureToRequirement(ctx, db.LinkMeasureToRequirementParams{
			MeasureID:     seeded[i].ID,
			RequirementID: req.ID,
		}); err != nil {
			return fmt.Errorf("link measure %s to requirement %s: %w", seeded[i].Name, req.Ref, err)
		}
	}
	return nil
}
