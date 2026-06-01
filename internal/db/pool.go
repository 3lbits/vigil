package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// Connect creates a pgxpool connection and returns both a pgxpool (for health
// checks / lifecycle) and a *sql.DB wrapper (for sqlc Queries and goose).
//
// tracer is an optional pgx.QueryTracer (pass nil to disable). When non-nil it
// fires for every query executed through the pool, including those issued by
// sqlc via the database/sql stdlib bridge.
func Connect(ctx context.Context, databaseURL string, tracer pgx.QueryTracer) (*pgxpool.Pool, *sql.DB, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse database URL: %w", err)
	}

	if tracer != nil {
		config.ConnConfig.Tracer = tracer
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("ping database: %w", err)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)
	return pool, sqlDB, nil
}
