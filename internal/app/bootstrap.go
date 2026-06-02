package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pressly/goose/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	dbmigrations "github.com/3lbits/vigil/db/migrations"
	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/config"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/locale"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/obs"
	"github.com/3lbits/vigil/internal/ui/layout"
)

type appState struct {
	obsShutdown  func()
	pool         *pgxpool.Pool
	sqlDB        *sql.DB
	queries      *db.Queries
	engine       *authz.Engine
	bundle       *i18n.Bundle
	metrics      *obs.Metrics
	reg          *prometheus.Registry
	csrfKey      []byte
	trustedCIDRs []*net.IPNet
	moduleFlags  *middleware.ModuleFlagsCache
}

func (s appState) closeDB() {
	if s.pool != nil {
		s.pool.Close()
	}
	if s.sqlDB != nil {
		if err := s.sqlDB.Close(); err != nil {
			slog.Error("close db", "error", err)
		}
	}
}

func bootstrap(ctx context.Context, cfg *config.Config, opts Options) (appState, error) {
	obsShutdown, err := obs.Init(ctx, "vigil")
	if err != nil {
		return appState{}, fmt.Errorf("obs init: %w", err)
	}
	if cfg.SessionHMACKey == "" {
		slog.Warn("SESSION_HMAC_KEY not set — session token hashes in security events will be unhashed")
	}

	csrfKeyBytes := sha256.Sum256([]byte("vigil-csrf:" + cfg.SessionHMACKey))
	trustedCIDRs := parseCIDRs(cfg.TrustedProxyCIDRs)

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	metrics := obs.NewMetrics(reg)

	dbTracer := obs.NewDBTracer(metrics.DBDuration)
	pool, sqlDB, err := db.Connect(ctx, cfg.DatabaseURL, dbTracer)
	if err != nil {
		return appState{}, fmt.Errorf("database connect: %w", err)
	}

	if err = goose.SetDialect("postgres"); err != nil {
		return appState{}, fmt.Errorf("goose set dialect: %w", err)
	}
	goose.SetBaseFS(dbmigrations.FS)
	if err = goose.Up(sqlDB, "."); err != nil {
		return appState{}, fmt.Errorf("goose migrate up: %w", err)
	}

	queries := db.New(sqlDB)
	moduleFlags := startModuleFlagsCache(ctx, queries)

	engine, err := authz.New(ctx, opts.PolicySource)
	if err != nil {
		return appState{}, fmt.Errorf("authz compile: %w", err)
	}
	bundle, err := locale.NewBundle()
	if err != nil {
		return appState{}, fmt.Errorf("locale bundle init: %w", err)
	}

	css, err := fs.ReadFile(opts.StaticFS, "public/css/output.css")
	if err != nil {
		return appState{}, fmt.Errorf("read embedded css: %w", err)
	}
	h := sha256.New()
	h.Write(css)
	layout.AssetVer = fmt.Sprintf("%x", h.Sum(nil)[:4])

	return appState{
		obsShutdown:  obsShutdown,
		pool:         pool,
		sqlDB:        sqlDB,
		queries:      queries,
		engine:       engine,
		bundle:       bundle,
		metrics:      metrics,
		reg:          reg,
		csrfKey:      csrfKeyBytes[:],
		trustedCIDRs: trustedCIDRs,
		moduleFlags:  moduleFlags,
	}, nil
}

func startModuleFlagsCache(
	ctx context.Context,
	queries *db.Queries,
) *middleware.ModuleFlagsCache {
	moduleFlags := middleware.NewModuleFlagsCache(queries)
	if refreshErr := moduleFlags.Refresh(ctx); refreshErr != nil {
		slog.Error("warm module flags cache", "error", refreshErr)
	}
	go func() {
		ticker := time.NewTicker(45 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if refreshErr := moduleFlags.Refresh(ctx); refreshErr != nil {
					slog.Error("refresh module flags cache", "error", refreshErr)
				}
			}
		}
	}()
	return moduleFlags
}
