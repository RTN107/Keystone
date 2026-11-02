// Package testutil gives integration tests a real Postgres with pgvector.
// Tests that need it skip unless TEST_DATABASE_URL is set. Run them with -p 1,
// since packages share one database.
package testutil

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"keystone/internal/db"
)

func Logger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// DB migrates the test database and resets it to a known state: every table emptied, and
// exactly the three sample teams the tests use. Production creates teams from KEYSTONE_TEAMS;
// tests seed their own. Resetting teams every time means a test that changes them cannot
// affect the next one.
func DB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	if err := db.Migrate(ctx, url, Logger()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Connect(ctx, url, Logger())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `TRUNCATE employees, documents, document_access, query_logs, teams RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO teams (name, number_prefix) VALUES
		('Sales & Marketing', 'SAL'), ('Delivery', 'DEL'), ('HR', 'HR')`); err != nil {
		t.Fatalf("seed teams: %v", err)
	}
	return pool
}
