// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

// Package strongbox provides multi-tenant secret storage types, interfaces,
// validation, and sentinel errors.
package strongbox

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// ---------------------------------------------------------------------------
// Core value types
// ---------------------------------------------------------------------------

// ClientID is a lowercase alphanumeric identifier with hyphens, max 63 chars.
// It must match ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$.
type ClientID string

// TenantID is an opaque string, max 255 chars, must not be empty.
type TenantID string

// SecretRef identifies a secret within a scope.
// Must match ^[a-zA-Z0-9][a-zA-Z0-9:._-]{0,511}$.
type SecretRef string

// Scope pins every operation to a client+tenant pair.
type Scope struct {
	ClientID ClientID
	TenantID TenantID
}

// ---------------------------------------------------------------------------
// Secret data types
// ---------------------------------------------------------------------------

// SecretValue is what callers receive from Get (contains plaintext).
type SecretValue struct {
	Ref       SecretRef
	Value     string
	Version   int64
	Metadata  map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt *time.Time
}

// SecretHeader is metadata without the value (returned by List).
type SecretHeader struct {
	Ref       SecretRef
	Version   int64
	Metadata  map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt *time.Time
}

// ---------------------------------------------------------------------------
// Input / output types
// ---------------------------------------------------------------------------

// PutInput describes a secret to store.
type PutInput struct {
	Ref       SecretRef
	Value     string
	Metadata  map[string]string
	ExpiresAt *time.Time
}

// PutResult is the outcome of a Put operation.
type PutResult struct {
	Ref             SecretRef
	Version         int64
	PreviousVersion *int64
	Action          string // "created", "updated", "unchanged"
}

// SyncMode controls how Sync handles secrets not in the input set.
type SyncMode string

const (
	// SyncPartial creates or updates secrets in the input set but
	// does not delete any existing secrets.
	SyncPartial SyncMode = "partial"
	// SyncFull creates or updates secrets in the input set and
	// soft-deletes secrets not present in the input set.
	SyncFull SyncMode = "full"
)

// SyncInput is the input for a bulk sync operation.
type SyncInput struct {
	Secrets  []PutInput
	SyncMode SyncMode
}

// SyncResult is the outcome of a Sync operation.
type SyncResult struct {
	Synced  []PutResult
	Deleted []SecretHeader
	Errors  []RefError
}

// ListOptions controls pagination and sorting for list operations.
type ListOptions struct {
	Prefix    string
	Cursor    string
	Limit     int    // Default 100, max 1000 (caller normalizes)
	SortField string // Default "ref". Also: "created_at", "updated_at", "version"
	SortDir   string // "asc" (default) or "desc"
}

// ListResult is a page of secret headers with an optional next cursor.
type ListResult struct {
	Secrets []SecretHeader
	Cursor  string
	HasMore bool
}

// RefError pairs a SecretRef with a coded error encountered operating on it.
type RefError struct {
	Ref     SecretRef
	Code    string
	Message string
}

// Error implements the error interface.
func (re RefError) Error() string {
	return fmt.Sprintf("%s: [%s] %s", re.Ref, re.Code, re.Message)
}

// ---------------------------------------------------------------------------
// StoredEntry — what Provider persists (opaque ciphertext)
// ---------------------------------------------------------------------------

// StoredEntry is the encrypted representation persisted by a Provider.
type StoredEntry struct {
	ClientID   ClientID
	TenantID   TenantID
	Ref        SecretRef
	Ciphertext []byte
	KeyID      string
	Version    int64
	Metadata   map[string]string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ExpiresAt  *time.Time
	DeletedAt  *time.Time
}

// ---------------------------------------------------------------------------
// Provider interface
// ---------------------------------------------------------------------------

// Provider is the storage backend interface that persists encrypted entries.
type Provider interface {
	PutEntry(ctx context.Context, entry StoredEntry) (version int64, err error)
	PutEntries(ctx context.Context, entries []StoredEntry) ([]int64, error)
	GetEntry(ctx context.Context, clientID ClientID, tenantID TenantID, ref SecretRef) (StoredEntry, error)
	ListEntries(ctx context.Context, clientID ClientID, tenantID TenantID, opts ListOptions) (ListResult, error)
	DeleteEntry(ctx context.Context, clientID ClientID, tenantID TenantID, ref SecretRef) error
	DeleteEntries(ctx context.Context, clientID ClientID, tenantID TenantID, refs []SecretRef) error
	HardDeleteEntry(ctx context.Context, clientID ClientID, tenantID TenantID, ref SecretRef) error
	HardDeleteTenant(ctx context.Context, clientID ClientID, tenantID TenantID) error
	ListByKeyID(ctx context.Context, clientID ClientID, tenantID TenantID, keyID string) ([]StoredEntry, error)
	Ping(ctx context.Context) error
}

// ---------------------------------------------------------------------------
// Audit
// ---------------------------------------------------------------------------

// AuditEvent records a single auditable operation.
type AuditEvent struct {
	Timestamp time.Time
	Operation string
	ClientID  ClientID
	TenantID  TenantID
	Ref       SecretRef
	Refs      []SecretRef
	Count     int
	Source    string
	Actor    string
	Error    string
	Duration time.Duration
}

// AuditLogger receives audit events.
type AuditLogger interface {
	Log(ctx context.Context, event AuditEvent)
}

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

var (
	ErrNotFound       = errors.New("strongbox: secret not found")
	ErrDeleted        = errors.New("strongbox: secret is soft-deleted")
	ErrExpired        = errors.New("strongbox: secret has expired")
	ErrInvalidRef     = errors.New("strongbox: invalid secret ref")
	ErrInvalidClient  = errors.New("strongbox: invalid client ID")
	ErrInvalidTenant  = errors.New("strongbox: invalid tenant ID")
	ErrValueTooLarge  = errors.New("strongbox: value too large")
	ErrBatchTooLarge  = errors.New("strongbox: batch too large")
	ErrDecryptFailed  = errors.New("strongbox: decryption failed")
	ErrKeyUnavailable = errors.New("strongbox: encryption key unavailable")
)

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// clientIDRe matches lowercase alphanumeric with interior hyphens, 1-63 chars.
var clientIDRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// secretRefRe matches the allowed ref pattern.
var secretRefRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9:._-]{0,511}$`)

// ValidateClientID checks that id is a valid ClientID.
func ValidateClientID(id ClientID) error {
	if !clientIDRe.MatchString(string(id)) {
		return ErrInvalidClient
	}
	return nil
}

// ValidateTenantID checks that id is a valid TenantID.
func ValidateTenantID(id TenantID) error {
	s := string(id)
	if s == "" || len(s) > 255 {
		return ErrInvalidTenant
	}
	return nil
}

// ValidateSecretRef checks that ref is a valid SecretRef.
func ValidateSecretRef(ref SecretRef) error {
	if !secretRefRe.MatchString(string(ref)) {
		return ErrInvalidRef
	}
	return nil
}

// ValidateScope validates both the ClientID and TenantID in a Scope.
func ValidateScope(s Scope) error {
	if err := ValidateClientID(s.ClientID); err != nil {
		return err
	}
	return ValidateTenantID(s.TenantID)
}