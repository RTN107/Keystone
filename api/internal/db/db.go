// Package db opens the connection pool and applies migrations on startup.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver for goose
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Connect opens a pool and waits for the database to answer. Neon computes sleep
// when idle, so the first connection can take several seconds.
//
// The pool uses the simple query protocol. Neon's pooled endpoint runs PgBouncer
// in transaction mode, where named prepared statements break.
func Connect(ctx context.Context, url string, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	cfg.MaxConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := waitForDB(ctx, pool, log); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func waitForDB(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	delay := time.Second
	var err error
	for attempt := 1; attempt <= 8; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		log.Warn("database not ready", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if delay < 8*time.Second {
			delay *= 2
		}
	}
	return fmt.Errorf("database unreachable: %w", err)
}

// Migrate applies every pending migration. It takes a Postgres advisory lock, so
// two API instances starting together cannot race. Pass a direct (unpooled) URL.
func Migrate(ctx context.Context, directURL string, log *slog.Logger) error {
	sqlDB, err := sql.Open("pgx", directURL)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	files, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, files, goose.WithSessionLocker(locker))
	if err != nil {
		return err
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	for _, r := range results {
		log.Info("migration applied", "version", r.Source.Version, "took", r.Duration)
	}
	return nil
}
