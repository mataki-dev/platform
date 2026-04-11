// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package postgres

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/mataki-dev/platform/strongbox"
)

// allowedSortColumns maps user-facing sort field names to SQL column names.
var allowedSortColumns = map[string]string{
	"ref":        "ref",
	"created_at": "created_at",
	"updated_at": "updated_at",
	"version":    "version",
}

// Config holds the configuration for a Postgres-backed Provider.
type Config struct {
	DB          *sql.DB
	TableName   string // Default: "strongbox_secrets"
	AutoMigrate bool
}

// provider implements strongbox.Provider.
type provider struct {
	db    *sql.DB
	table string
}

// NewProvider creates a new Postgres-backed strongbox.Provider.
// If cfg.AutoMigrate is true the schema DDL is executed before returning.
func NewProvider(ctx context.Context, cfg Config) (strongbox.Provider, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("postgres: DB must not be nil")
	}
	table := cfg.TableName
	if table == "" {
		table = "strongbox_secrets"
	}
	if cfg.AutoMigrate {
		if err := migrate(ctx, cfg.DB); err != nil {
			return nil, fmt.Errorf("postgres: auto-migrate failed: %w", err)
		}
	}
	return &provider{db: cfg.DB, table: table}, nil
}

// ---------------------------------------------------------------------------
// Write operations
// ---------------------------------------------------------------------------

// PutEntry inserts or updates a single stored entry.
func (p *provider) PutEntry(ctx context.Context, entry strongbox.StoredEntry) (int64, error) {
	meta, err := encodeMetadata(entry.Metadata)
	if err != nil {
		return 0, err
	}
	query := fmt.Sprintf(`
		INSERT INTO %s (client_id, tenant_id, ref, ciphertext, key_id, version, metadata, expires_at)
		VALUES ($1, $2, $3, $4, $5, 1, $6, $7)
		ON CONFLICT (client_id, tenant_id, ref) DO UPDATE SET
			ciphertext = EXCLUDED.ciphertext,
			key_id     = EXCLUDED.key_id,
			metadata   = EXCLUDED.metadata,
			expires_at = EXCLUDED.expires_at,
			updated_at = now(),
			deleted_at = NULL,
			version    = %s.version + 1
		RETURNING version
	`, p.table, p.table)

	var version int64
	err = p.db.QueryRowContext(ctx, query,
		string(entry.ClientID),
		string(entry.TenantID),
		string(entry.Ref),
		entry.Ciphertext,
		entry.KeyID,
		meta,
		entry.ExpiresAt,
	).Scan(&version)
	return version, err
}

// PutEntries inserts or updates multiple entries atomically within a
// SERIALIZABLE transaction.
func (p *provider) PutEntries(ctx context.Context, entries []strongbox.StoredEntry) ([]int64, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("postgres: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	query := fmt.Sprintf(`
		INSERT INTO %s (client_id, tenant_id, ref, ciphertext, key_id, version, metadata, expires_at)
		VALUES ($1, $2, $3, $4, $5, 1, $6, $7)
		ON CONFLICT (client_id, tenant_id, ref) DO UPDATE SET
			ciphertext = EXCLUDED.ciphertext,
			key_id     = EXCLUDED.key_id,
			metadata   = EXCLUDED.metadata,
			expires_at = EXCLUDED.expires_at,
			updated_at = now(),
			deleted_at = NULL,
			version    = %s.version + 1
		RETURNING version
	`, p.table, p.table)

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("postgres: prepare: %w", err)
	}
	defer stmt.Close()

	versions := make([]int64, 0, len(entries))
	for _, entry := range entries {
		meta, err := encodeMetadata(entry.Metadata)
		if err != nil {
			return nil, err
		}
		var version int64
		if err := stmt.QueryRowContext(ctx,
			string(entry.ClientID),
			string(entry.TenantID),
			string(entry.Ref),
			entry.Ciphertext,
			entry.KeyID,
			meta,
			entry.ExpiresAt,
		).Scan(&version); err != nil {
			return nil, fmt.Errorf("postgres: put entry %s: %w", entry.Ref, err)
		}
		versions = append(versions, version)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return versions, nil
}

// DeleteEntry soft-deletes a single entry by setting deleted_at and wiping
// the ciphertext. Returns strongbox.ErrNotFound if no active row matches.
func (p *provider) DeleteEntry(ctx context.Context, clientID strongbox.ClientID, tenantID strongbox.TenantID, ref strongbox.SecretRef) error {
	query := fmt.Sprintf(`
		UPDATE %s
		SET deleted_at  = now(),
		    ciphertext  = '\x00',
		    updated_at  = now()
		WHERE client_id = $1
		  AND tenant_id = $2
		  AND ref       = $3
		  AND deleted_at IS NULL
	`, p.table)

	res, err := p.db.ExecContext(ctx, query, string(clientID), string(tenantID), string(ref))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return strongbox.ErrNotFound
	}
	return nil
}

// DeleteEntries soft-deletes multiple entries within a transaction.
func (p *provider) DeleteEntries(ctx context.Context, clientID strongbox.ClientID, tenantID strongbox.TenantID, refs []strongbox.SecretRef) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgres: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	query := fmt.Sprintf(`
		UPDATE %s
		SET deleted_at  = now(),
		    ciphertext  = '\x00',
		    updated_at  = now()
		WHERE client_id = $1
		  AND tenant_id = $2
		  AND ref       = $3
		  AND deleted_at IS NULL
	`, p.table)

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("postgres: prepare: %w", err)
	}
	defer stmt.Close()

	for _, ref := range refs {
		if _, err := stmt.ExecContext(ctx, string(clientID), string(tenantID), string(ref)); err != nil {
			return fmt.Errorf("postgres: delete entry %s: %w", ref, err)
		}
	}
	return tx.Commit()
}

// HardDeleteEntry permanently removes a single entry.
func (p *provider) HardDeleteEntry(ctx context.Context, clientID strongbox.ClientID, tenantID strongbox.TenantID, ref strongbox.SecretRef) error {
	query := fmt.Sprintf(`
		DELETE FROM %s
		WHERE client_id = $1
		  AND tenant_id = $2
		  AND ref       = $3
	`, p.table)

	_, err := p.db.ExecContext(ctx, query, string(clientID), string(tenantID), string(ref))
	return err
}

// HardDeleteTenant permanently removes all entries for a client+tenant pair.
func (p *provider) HardDeleteTenant(ctx context.Context, clientID strongbox.ClientID, tenantID strongbox.TenantID) error {
	query := fmt.Sprintf(`
		DELETE FROM %s
		WHERE client_id = $1
		  AND tenant_id = $2
	`, p.table)

	_, err := p.db.ExecContext(ctx, query, string(clientID), string(tenantID))
	return err
}

// ---------------------------------------------------------------------------
// Read operations
// ---------------------------------------------------------------------------

// GetEntry retrieves a single stored entry including soft-deleted rows.
// Returns strongbox.ErrNotFound if no row exists.
func (p *provider) GetEntry(ctx context.Context, clientID strongbox.ClientID, tenantID strongbox.TenantID, ref strongbox.SecretRef) (strongbox.StoredEntry, error) {
	query := fmt.Sprintf(`
		SELECT client_id, tenant_id, ref, ciphertext, key_id, version,
		       metadata, created_at, updated_at, expires_at, deleted_at
		FROM %s
		WHERE client_id = $1
		  AND tenant_id = $2
		  AND ref       = $3
	`, p.table)

	var (
		e       strongbox.StoredEntry
		metaRaw []byte
	)
	err := p.db.QueryRowContext(ctx, query, string(clientID), string(tenantID), string(ref)).Scan(
		&e.ClientID,
		&e.TenantID,
		&e.Ref,
		&e.Ciphertext,
		&e.KeyID,
		&e.Version,
		&metaRaw,
		&e.CreatedAt,
		&e.UpdatedAt,
		&e.ExpiresAt,
		&e.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return strongbox.StoredEntry{}, strongbox.ErrNotFound
	}
	if err != nil {
		return strongbox.StoredEntry{}, err
	}
	e.Metadata, err = decodeMetadata(metaRaw)
	if err != nil {
		return strongbox.StoredEntry{}, err
	}
	return e, nil
}

// ListEntries returns a paginated list of non-deleted secret headers.
func (p *provider) ListEntries(ctx context.Context, clientID strongbox.ClientID, tenantID strongbox.TenantID, opts strongbox.ListOptions) (strongbox.ListResult, error) {
	// Resolve sort column.
	sortCol, ok := allowedSortColumns[opts.SortField]
	if !ok {
		sortCol = "ref"
	}
	sortDir := "ASC"
	if opts.SortDir == "desc" {
		sortDir = "DESC"
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	// Decode cursor into the last-seen ref.
	cursorRef := ""
	if opts.Cursor != "" {
		var err error
		cursorRef, err = decodeCursor(opts.Cursor)
		if err != nil {
			return strongbox.ListResult{}, fmt.Errorf("postgres: invalid cursor: %w", err)
		}
	}

	// Query limit+1 to detect hasMore.
	query := fmt.Sprintf(`
		SELECT ref, version, metadata, created_at, updated_at, expires_at
		FROM %s
		WHERE client_id = $1
		  AND tenant_id = $2
		  AND deleted_at IS NULL
		  AND ($3 = '' OR ref LIKE $3 || '%%')
		  AND ($4 = '' OR ref > $4)
		ORDER BY %s %s, ref ASC
		LIMIT $5
	`, p.table, sortCol, sortDir)

	rows, err := p.db.QueryContext(ctx, query,
		string(clientID),
		string(tenantID),
		opts.Prefix,
		cursorRef,
		limit+1,
	)
	if err != nil {
		return strongbox.ListResult{}, err
	}
	defer rows.Close()

	var headers []strongbox.SecretHeader
	for rows.Next() {
		var (
			h       strongbox.SecretHeader
			metaRaw []byte
		)
		if err := rows.Scan(&h.Ref, &h.Version, &metaRaw, &h.CreatedAt, &h.UpdatedAt, &h.ExpiresAt); err != nil {
			return strongbox.ListResult{}, err
		}
		h.Metadata, err = decodeMetadata(metaRaw)
		if err != nil {
			return strongbox.ListResult{}, err
		}
		headers = append(headers, h)
	}
	if err := rows.Err(); err != nil {
		return strongbox.ListResult{}, err
	}

	result := strongbox.ListResult{}
	if len(headers) > limit {
		result.HasMore = true
		headers = headers[:limit]
	}
	result.Secrets = headers
	if len(headers) > 0 {
		result.Cursor = encodeCursor(string(headers[len(headers)-1].Ref))
	}
	return result, nil
}

// ListByKeyID returns all non-deleted entries encrypted with the given key ID.
func (p *provider) ListByKeyID(ctx context.Context, clientID strongbox.ClientID, tenantID strongbox.TenantID, keyID string) ([]strongbox.StoredEntry, error) {
	query := fmt.Sprintf(`
		SELECT client_id, tenant_id, ref, ciphertext, key_id, version,
		       metadata, created_at, updated_at, expires_at, deleted_at
		FROM %s
		WHERE client_id = $1
		  AND tenant_id = $2
		  AND key_id = $3
		  AND deleted_at IS NULL
	`, p.table)

	rows, err := p.db.QueryContext(ctx, query, string(clientID), string(tenantID), keyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []strongbox.StoredEntry
	for rows.Next() {
		var (
			e       strongbox.StoredEntry
			metaRaw []byte
		)
		if err := rows.Scan(
			&e.ClientID, &e.TenantID, &e.Ref, &e.Ciphertext, &e.KeyID,
			&e.Version, &metaRaw, &e.CreatedAt, &e.UpdatedAt, &e.ExpiresAt, &e.DeletedAt,
		); err != nil {
			return nil, err
		}
		e.Metadata, err = decodeMetadata(metaRaw)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Ping verifies connectivity to the database.
func (p *provider) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// encodeMetadata marshals a metadata map to JSON for storage.
// A nil map is stored as SQL NULL (nil []byte).
func encodeMetadata(m map[string]string) ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// decodeMetadata unmarshals a JSON blob back into a metadata map.
// A nil blob yields a nil map.
func decodeMetadata(data []byte) (map[string]string, error) {
	if data == nil {
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("postgres: decode metadata: %w", err)
	}
	return m, nil
}

// encodeCursor base64url-encodes a ref string for use as a pagination cursor.
func encodeCursor(ref string) string {
	return base64.URLEncoding.EncodeToString([]byte(ref))
}

// decodeCursor base64url-decodes a pagination cursor back to a ref string.
func decodeCursor(cursor string) (string, error) {
	b, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Compile-time interface check.
var _ strongbox.Provider = (*provider)(nil)