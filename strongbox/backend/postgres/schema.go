// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

// Package postgres implements the strongbox.Provider interface backed by PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
)

const schemaDDL = `
CREATE TABLE IF NOT EXISTS strongbox_secrets (
    client_id   TEXT        NOT NULL,
    tenant_id   TEXT        NOT NULL,
    ref         TEXT        NOT NULL,
    ciphertext  BYTEA       NOT NULL,
    key_id      TEXT        NOT NULL,
    version     BIGINT      NOT NULL DEFAULT 1,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ,
    PRIMARY KEY (client_id, tenant_id, ref)
);

CREATE INDEX IF NOT EXISTS idx_strongbox_key_id
    ON strongbox_secrets (client_id, tenant_id, key_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_strongbox_ref_prefix
    ON strongbox_secrets (client_id, tenant_id, ref text_pattern_ops)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_strongbox_expires
    ON strongbox_secrets (expires_at)
    WHERE expires_at IS NOT NULL AND deleted_at IS NULL;
`

// migrate executes the schema DDL against the provided database.
func migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, schemaDDL)
	return err
}