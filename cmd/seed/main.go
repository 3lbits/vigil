package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"

	dbmigrations "github.com/3lbits/vigil/db/migrations"
	"github.com/3lbits/vigil/internal/config"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/devseed"
	"github.com/3lbits/vigil/internal/modregistry"
	"github.com/3lbits/vigil/internal/modules/about"
	"github.com/3lbits/vigil/internal/modules/activities"
	"github.com/3lbits/vigil/internal/modules/admin"
	"github.com/3lbits/vigil/internal/modules/assets"
	"github.com/3lbits/vigil/internal/modules/compliance"
	"github.com/3lbits/vigil/internal/modules/dashboard"
	"github.com/3lbits/vigil/internal/modules/me"
	"github.com/3lbits/vigil/internal/modules/measures"
	"github.com/3lbits/vigil/internal/modules/risk"
)

var errDevSeedingDisabled = errors.New("dev seeding is disabled; require APP_ENV=development and DEV_SEED=true")
var errNoSeededAssessments = errors.New("seed completed but no risk assessments were created")

func main() {
	if err := run(); err != nil {
		slog.Error("dev seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()
	cfg := config.Load()
	if cfg.AppEnv != "development" || !cfg.DevSeed {
		return errDevSeedingDisabled
	}
	slog.Info("starting development seed", "env", cfg.AppEnv)

	ctx := context.Background()
	pool, sqlDB, err := db.Connect(ctx, cfg.DatabaseURL, nil)
	if err != nil {
		return fmt.Errorf("database connect: %w", err)
	}
	defer pool.Close()
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			slog.Error("close db", "error", closeErr)
		}
	}()

	if err = goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}
	goose.SetBaseFS(dbmigrations.FS)
	slog.Info("applying migrations")
	if err = goose.Up(sqlDB, "."); err != nil {
		return fmt.Errorf("goose migrate up: %w", err)
	}

	queries := db.New(sqlDB)
	slog.Info("seeding core data")
	users, orgs, err := seedCoreData(ctx, queries)
	if err != nil {
		return err
	}
	slog.Info("core data seeded", "users", len(users), "organizations", len(orgs))

	slog.Info("seeding module data")
	if err = seedModuleData(ctx, pool, queries); err != nil {
		return err
	}
	slog.Info("module data seeded")

	assessments, err := queries.ListRiskAssessments(ctx)
	if err != nil {
		return fmt.Errorf("verify risk assessments: %w", err)
	}
	if len(assessments) == 0 {
		return errNoSeededAssessments
	}

	slog.Info("development seed complete", "users", len(users), "organizations", len(orgs), "risk_assessments", len(assessments))
	return nil
}

func seedCoreData(ctx context.Context, queries *db.Queries) ([]db.User, []db.Organization, error) {
	users, err := devseed.SeedStubUsers(ctx, queries)
	if err != nil {
		return nil, nil, fmt.Errorf("seed dev users: %w", err)
	}
	orgs, err := devseed.EnsureOrganizations(ctx, queries)
	if err != nil {
		return nil, nil, fmt.Errorf("seed organizations: %w", err)
	}
	if err = devseed.AssignUsersToOrgs(ctx, queries, users, orgs); err != nil {
		return nil, nil, fmt.Errorf("assign users to organizations: %w", err)
	}
	return users, orgs, nil
}

func seedModuleData(ctx context.Context, pool dbPinger, queries *db.Queries) error {
	deps := modregistry.Dependencies{Queries: queries}
	for _, module := range seedModules(pool) {
		seeder, ok := module.(modregistry.DevSeeder)
		if !ok {
			continue
		}
		slog.Info("seeding module", "module", module.Name())
		if err := seeder.DevSeed(ctx, deps); err != nil {
			return fmt.Errorf("seed module %q: %w", module.Name(), err)
		}
		slog.Info("seeded module", "module", module.Name())
	}
	return nil
}

func seedModules(pool dbPinger) []modregistry.Module {
	return []modregistry.Module{
		about.New(),
		dashboard.New(),
		compliance.New(),
		measures.New(),
		me.New(),
		assets.New(),
		activities.New(),
		risk.New(),
		admin.New(pool, time.Now(), "dev-seed"),
	}
}

type dbPinger interface {
	Ping(context.Context) error
}
