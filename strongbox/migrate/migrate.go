// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

// Package migrate provides utilities for migrating existing encrypted columns
// into Strongbox.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/mataki-dev/platform/strongbox"
)

// ColumnMapping describes how to read an old encrypted column, decrypt it, and
// store it under a Strongbox ref.
type ColumnMapping struct {
	Table        string                                  // Source table
	IDColumn     string                                  // Row identifier column
	SecretColumn string                                  // Column with old encrypted value
	RefTemplate  string                                  // e.g. "conn:{{id}}:client_secret"
	DecryptFunc  func(ciphertext []byte) (string, error) // Caller's existing decryption
}

// Options controls migration behaviour.
type Options struct {
	BatchSize     int  // Default 100
	NullOldColumn bool // SET old column to NULL after migration
	DryRun        bool
}

// Result summarises a completed migration run.
type Result struct {
	Migrated int
	Skipped  int
	Errors   []error
}

// Run reads old encrypted columns, decrypts with the caller's function,
// stores in Strongbox, and optionally nulls the old column.
// Idempotent: skips rows where the ref already exists in Strongbox.
func Run(ctx context.Context, db *sql.DB, store *strongbox.Store,
	scope strongbox.Scope, mappings []ColumnMapping, opts Options) (Result, error) {

	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}

	var result Result

	for _, m := range mappings {
		if err := processMapping(ctx, db, store, scope, m, opts, &result); err != nil {
			return result, err
		}
	}

	return result, nil
}

// processMapping handles a single ColumnMapping.
func processMapping(ctx context.Context, db *sql.DB, store *strongbox.Store,
	scope strongbox.Scope, m ColumnMapping, opts Options, result *Result) error {

	query := fmt.Sprintf(
		"SELECT %s, %s FROM %s WHERE %s IS NOT NULL",
		quoteIdent(m.IDColumn),
		quoteIdent(m.SecretColumn),
		quoteIdent(m.Table),
		quoteIdent(m.SecretColumn),
	)

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("migrate: query %s: %w", m.Table, err)
	}
	defer rows.Close()

	var batch []pendingRow
	for rows.Next() {
		var id string
		var ciphertext []byte
		if err := rows.Scan(&id, &ciphertext); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("migrate: scan %s: %w", m.Table, err))
			continue
		}
		batch = append(batch, pendingRow{id: id, ciphertext: ciphertext})

		if len(batch) >= opts.BatchSize {
			processBatch(ctx, db, store, scope, m, opts, batch, result)
			batch = batch[:0]
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrate: iterate %s: %w", m.Table, err)
	}

	// Process remaining rows.
	if len(batch) > 0 {
		processBatch(ctx, db, store, scope, m, opts, batch, result)
	}

	return nil
}

type pendingRow struct {
	id         string
	ciphertext []byte
}

// processBatch handles a slice of rows for a single mapping.
func processBatch(ctx context.Context, db *sql.DB, store *strongbox.Store,
	scope strongbox.Scope, m ColumnMapping, opts Options,
	batch []pendingRow, result *Result) {

	for _, row := range batch {
		ref := strongbox.SecretRef(buildRef(m.RefTemplate, row.id))

		// Idempotent: skip if already in Strongbox.
		if _, err := store.Get(ctx, scope, ref); err == nil {
			result.Skipped++
			continue
		}

		plaintext, err := m.DecryptFunc(row.ciphertext)
		if err != nil {
			result.Errors = append(result.Errors,
				fmt.Errorf("migrate: decrypt %s id=%s: %w", m.Table, row.id, err))
			continue
		}

		if !opts.DryRun {
			_, err = store.Put(ctx, scope, strongbox.PutInput{
				Ref:   ref,
				Value: plaintext,
			})
			if err != nil {
				result.Errors = append(result.Errors,
					fmt.Errorf("migrate: put %s id=%s: %w", m.Table, row.id, err))
				continue
			}
		}

		if opts.NullOldColumn && !opts.DryRun {
			nullQuery := fmt.Sprintf(
				"UPDATE %s SET %s = NULL WHERE %s = $1",
				quoteIdent(m.Table),
				quoteIdent(m.SecretColumn),
				quoteIdent(m.IDColumn),
			)
			if _, err := db.ExecContext(ctx, nullQuery, row.id); err != nil {
				result.Errors = append(result.Errors,
					fmt.Errorf("migrate: null %s id=%s: %w", m.Table, row.id, err))
				continue
			}
		}

		result.Migrated++
	}
}

// buildRef replaces "{{id}}" in the template with the actual row ID.
func buildRef(template, id string) string {
	return strings.ReplaceAll(template, "{{id}}", id)
}

// quoteIdent wraps a SQL identifier in double quotes for safety.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}