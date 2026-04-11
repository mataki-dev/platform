// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mataki-dev/platform/strongbox/backend/postgres"
	"github.com/mataki-dev/platform/strongbox/conformance"
)

func TestPostgresProvider(t *testing.T) {
	dsn := os.Getenv("STRONGBOX_TEST_DSN")
	if dsn == "" {
		t.Skip("STRONGBOX_TEST_DSN not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Reset table for a clean test run.
	if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS strongbox_secrets"); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	p, err := postgres.NewProvider(ctx, postgres.Config{DB: db, AutoMigrate: true})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	conformance.RunProviderSuite(t, p)
}