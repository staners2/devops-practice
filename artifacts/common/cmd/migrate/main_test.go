package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"devops-practice/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestApplyConcurrentRepeatAndRollback(t *testing.T) {
	url := os.Getenv("PG_MIGRATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set PG_MIGRATION_TEST_DATABASE_URL to an isolated *_test database")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal("invalid test connection configuration")
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("migration checks require a dedicated *_test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	schema := pgx.Identifier{fmt.Sprintf("migration_check_%d", time.Now().UnixNano())}.Sanitize()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("cannot create test pool")
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal("cannot create isolated test schema")
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error("cannot clean up isolated migration test schema")
		}
	}()
	versions := []migrations.Migration{
		{Version: 1, SQL: "CREATE TABLE migration_fixture (id integer PRIMARY KEY)"},
		{Version: 2, SQL: "INSERT INTO migration_fixture VALUES (1)"},
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- apply(ctx, pool, versions) }()
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatalf("parallel apply failed: %v", err)
		}
	}
	if err := apply(ctx, pool, versions); err != nil {
		t.Fatalf("repeat apply failed: %v", err)
	}
	broken := append(versions, migrations.Migration{
		Version: 3, SQL: "INSERT INTO migration_fixture VALUES (2); CREATE INDEX broken_index ON migration_fixture (missing_column)",
	})
	if err := apply(ctx, pool, broken); err == nil {
		t.Fatal("invalid DDL must fail")
	}
	var rows, applied int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM migration_fixture").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || applied != 2 {
		t.Fatalf("rollback/repeat failed: rows=%d, versions=%d", rows, applied)
	}
}
