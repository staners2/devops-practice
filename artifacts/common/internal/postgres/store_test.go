package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"devops-practice/internal/config"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewRejectsInvalidDatabaseURL(t *testing.T) {
	_, err := New(context.Background(), "://invalid", config.Postgres{})
	if err == nil {
		t.Fatal("New() error = nil, want invalid database URL error")
	}
}

func TestPoolOptionsWithPostgres(t *testing.T) {
	url := os.Getenv("PG_MIGRATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set PG_MIGRATION_TEST_DATABASE_URL to an isolated *_test database")
	}
	parsed, err := pgxpool.ParseConfig(url)
	if err != nil || !strings.HasSuffix(parsed.ConnConfig.Database, "_test") {
		t.Fatal("pool checks require a dedicated *_test database")
	}
	options := config.Postgres{MaxConns: 2, MinConns: 0, StatementTimeout: 50 * time.Millisecond,
		LockTimeout: 20 * time.Millisecond, IdleInTransactionTimeout: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := New(ctx, url, options)
	if err != nil {
		t.Fatal("cannot open test PostgreSQL pool")
	}
	defer store.Close()
	var statement, lock, idle string
	if err := store.pool.QueryRow(ctx, "SELECT current_setting('statement_timeout'), current_setting('lock_timeout'), current_setting('idle_in_transaction_session_timeout')").Scan(&statement, &lock, &idle); err != nil {
		t.Fatal(err)
	}
	if statement != "50ms" || lock != "20ms" || idle != "1s" || store.PoolStats().MaxConns() != 2 {
		t.Fatalf("unexpected pool settings: %s, %s, %s", statement, lock, idle)
	}
	_, err = store.pool.Exec(ctx, "SELECT pg_sleep(0.2)")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "57014" {
		t.Fatal("statement_timeout did not cancel SQL")
	}
	first, err := store.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	second, err := store.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()
	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer waitCancel()
	if _, err := store.pool.Acquire(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("acquire on saturated pool did not reach deadline")
	}
	if store.PoolStats().CanceledAcquireCount() < 1 {
		t.Fatal("canceled acquire was not counted")
	}
}
