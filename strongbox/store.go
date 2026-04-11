// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package strongbox

import (
	"context"
	"errors"
	"time"
)

const (
	defaultMaxValueSize = 65536
	defaultMaxBatchSize = 500
	defaultListLimit    = 100
	maxListLimit        = 1000
)

// Store is the central strongbox type that encrypts secrets and delegates
// persistence to a Provider.
type Store struct {
	provider     Provider
	enc          encryptor
	rootKey      []byte
	rootKeyID    string
	prevKey      []byte
	prevKeyID    string
	audit        AuditLogger
	maxValSize   int
	maxBatchSize int
}

// NewStore creates a Store from a Provider and functional options.
func NewStore(provider Provider, opts ...Option) (*Store, error) {
	if provider == nil {
		return nil, errors.New("strongbox: provider must not be nil")
	}

	cfg := &storeConfig{}
	for _, o := range opts {
		o(cfg)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// Apply defaults.
	if cfg.maxValSize <= 0 {
		cfg.maxValSize = defaultMaxValueSize
	}
	if cfg.maxBatchSize <= 0 {
		cfg.maxBatchSize = defaultMaxBatchSize
	}
	if cfg.audit == nil {
		cfg.audit = noopAudit{}
	}

	s := &Store{
		provider:     provider,
		enc:          NewEnvelopeEncryptor(),
		rootKey:      cfg.rootKey,
		rootKeyID:    RootKeyID(cfg.rootKey),
		audit:        cfg.audit,
		maxValSize:   cfg.maxValSize,
		maxBatchSize: cfg.maxBatchSize,
	}

	if len(cfg.prevKey) == 32 {
		s.prevKey = cfg.prevKey
		s.prevKeyID = RootKeyID(cfg.prevKey)
	}

	return s, nil
}

// ---------------------------------------------------------------------------
// noopAudit — default audit logger that discards events
// ---------------------------------------------------------------------------

type noopAudit struct{}

func (noopAudit) Log(_ context.Context, _ AuditEvent) {}

// ---------------------------------------------------------------------------
// emitAudit helper
// ---------------------------------------------------------------------------

func (s *Store) emitAudit(ctx context.Context, op string, scope Scope, ref SecretRef, refs []SecretRef, count int, startTime time.Time, err error) {
	ev := AuditEvent{
		Timestamp: startTime,
		Operation: op,
		ClientID:  scope.ClientID,
		TenantID:  scope.TenantID,
		Ref:       ref,
		Refs:      refs,
		Count:     count,
		Duration:  time.Since(startTime),
	}
	if err != nil {
		ev.Error = err.Error()
	}
	s.audit.Log(ctx, ev)
}

// ---------------------------------------------------------------------------
// normalizeListOptions applies defaults and caps to ListOptions
// ---------------------------------------------------------------------------

func normalizeListOptions(opts ListOptions) ListOptions {
	if opts.Limit <= 0 {
		opts.Limit = defaultListLimit
	}
	if opts.Limit > maxListLimit {
		opts.Limit = maxListLimit
	}
	if opts.SortField == "" {
		opts.SortField = "ref"
	}
	if opts.SortDir == "" {
		opts.SortDir = "asc"
	}
	return opts
}

// ---------------------------------------------------------------------------
// Put stores or updates a single secret.
// ---------------------------------------------------------------------------

func (s *Store) Put(ctx context.Context, scope Scope, input PutInput) (*PutResult, error) {
	start := time.Now()
	if err := ValidateScope(scope); err != nil {
		return nil, err
	}
	if err := ValidateSecretRef(input.Ref); err != nil {
		return nil, err
	}
	if len(input.Value) > s.maxValSize {
		return nil, ErrValueTooLarge
	}

	dk, err := DeriveKey(s.rootKey, scope.ClientID, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer zero(dk)

	ct, err := s.enc.Encrypt(dk, []byte(input.Value))
	if err != nil {
		return nil, err
	}

	// Check if entry already exists (for action detection).
	existing, getErr := s.provider.GetEntry(ctx, scope.ClientID, scope.TenantID, input.Ref)
	var prevVersion *int64
	action := "created"
	if getErr == nil && existing.DeletedAt == nil {
		action = "updated"
		prevVersion = &existing.Version
	}

	now := time.Now()
	entry := StoredEntry{
		ClientID:   scope.ClientID,
		TenantID:   scope.TenantID,
		Ref:        input.Ref,
		Ciphertext: ct,
		KeyID:      s.rootKeyID,
		Metadata:   input.Metadata,
		CreatedAt:  now,
		UpdatedAt:  now,
		ExpiresAt:  input.ExpiresAt,
	}

	version, err := s.provider.PutEntry(ctx, entry)
	if err != nil {
		s.emitAudit(ctx, "put", scope, input.Ref, nil, 0, start, err)
		return nil, err
	}

	result := &PutResult{
		Ref:             input.Ref,
		Version:         version,
		PreviousVersion: prevVersion,
		Action:          action,
	}
	s.emitAudit(ctx, "put", scope, input.Ref, nil, 0, start, nil)
	return result, nil
}

// ---------------------------------------------------------------------------
// Get retrieves a single secret by ref.
// ---------------------------------------------------------------------------

func (s *Store) Get(ctx context.Context, scope Scope, ref SecretRef) (*SecretValue, error) {
	start := time.Now()
	if err := ValidateScope(scope); err != nil {
		return nil, err
	}
	if err := ValidateSecretRef(ref); err != nil {
		return nil, err
	}

	entry, err := s.provider.GetEntry(ctx, scope.ClientID, scope.TenantID, ref)
	if err != nil {
		s.emitAudit(ctx, "get", scope, ref, nil, 0, start, err)
		return nil, err
	}

	if entry.DeletedAt != nil {
		s.emitAudit(ctx, "get", scope, ref, nil, 0, start, ErrDeleted)
		return nil, ErrDeleted
	}
	if entry.ExpiresAt != nil && entry.ExpiresAt.Before(time.Now()) {
		s.emitAudit(ctx, "get", scope, ref, nil, 0, start, ErrExpired)
		return nil, ErrExpired
	}

	dk, err := DeriveKey(s.rootKey, scope.ClientID, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer zero(dk)

	plaintext, err := s.enc.Decrypt(dk, entry.Ciphertext)
	if err != nil {
		s.emitAudit(ctx, "get", scope, ref, nil, 0, start, err)
		return nil, err
	}
	defer zero(plaintext)

	sv := &SecretValue{
		Ref:       entry.Ref,
		Value:     string(plaintext),
		Version:   entry.Version,
		Metadata:  entry.Metadata,
		CreatedAt: entry.CreatedAt,
		UpdatedAt: entry.UpdatedAt,
		ExpiresAt: entry.ExpiresAt,
	}
	s.emitAudit(ctx, "get", scope, ref, nil, 0, start, nil)
	return sv, nil
}

// ---------------------------------------------------------------------------
// BatchPut stores or updates multiple secrets.
// ---------------------------------------------------------------------------

func (s *Store) BatchPut(ctx context.Context, scope Scope, inputs []PutInput) ([]PutResult, error) {
	start := time.Now()
	if err := ValidateScope(scope); err != nil {
		return nil, err
	}
	if len(inputs) > s.maxBatchSize {
		return nil, ErrBatchTooLarge
	}

	dk, err := DeriveKey(s.rootKey, scope.ClientID, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer zero(dk)

	var validEntries []StoredEntry
	var validInputs []PutInput
	var refErrors []RefError

	now := time.Now()
	for _, inp := range inputs {
		if err := ValidateSecretRef(inp.Ref); err != nil {
			refErrors = append(refErrors, RefError{Ref: inp.Ref, Code: "INVALID_REF", Message: err.Error()})
			continue
		}
		if len(inp.Value) > s.maxValSize {
			refErrors = append(refErrors, RefError{Ref: inp.Ref, Code: "VALUE_TOO_LARGE", Message: ErrValueTooLarge.Error()})
			continue
		}

		ct, err := s.enc.Encrypt(dk, []byte(inp.Value))
		if err != nil {
			refErrors = append(refErrors, RefError{Ref: inp.Ref, Code: "ENCRYPT_FAILED", Message: err.Error()})
			continue
		}

		validEntries = append(validEntries, StoredEntry{
			ClientID:   scope.ClientID,
			TenantID:   scope.TenantID,
			Ref:        inp.Ref,
			Ciphertext: ct,
			KeyID:      s.rootKeyID,
			Metadata:   inp.Metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
			ExpiresAt:  inp.ExpiresAt,
		})
		validInputs = append(validInputs, inp)
	}

	if len(validEntries) == 0 {
		s.emitAudit(ctx, "batch_put", scope, "", nil, 0, start, nil)
		return nil, nil
	}

	versions, err := s.provider.PutEntries(ctx, validEntries)
	if err != nil {
		s.emitAudit(ctx, "batch_put", scope, "", nil, 0, start, err)
		return nil, err
	}

	results := make([]PutResult, len(versions))
	for i, v := range versions {
		r := PutResult{
			Ref:     validInputs[i].Ref,
			Version: v,
			Action:  "created",
		}
		if v > 1 {
			r.Action = "updated"
			prev := v - 1
			r.PreviousVersion = &prev
		}
		results[i] = r
	}

	refs := make([]SecretRef, len(validInputs))
	for i, inp := range validInputs {
		refs[i] = inp.Ref
	}
	s.emitAudit(ctx, "batch_put", scope, "", refs, len(results), start, nil)

	if len(refErrors) > 0 {
		// Return results along with the error info; callers can inspect both.
		return results, &batchError{Results: results, RefErrors: refErrors}
	}
	return results, nil
}

// batchError carries partial results alongside ref-level errors.
type batchError struct {
	Results   []PutResult
	RefErrors []RefError
}

func (e *batchError) Error() string {
	return "strongbox: batch contained invalid entries"
}

// ---------------------------------------------------------------------------
// GetMany retrieves multiple secrets by ref.
// ---------------------------------------------------------------------------

func (s *Store) GetMany(ctx context.Context, scope Scope, refs []SecretRef) ([]SecretValue, error) {
	if err := ValidateScope(scope); err != nil {
		return nil, err
	}

	var values []SecretValue
	var refErrors []RefError

	for _, ref := range refs {
		sv, err := s.Get(ctx, scope, ref)
		if err != nil {
			code := "ERROR"
			if errors.Is(err, ErrNotFound) {
				code = "NOT_FOUND"
			} else if errors.Is(err, ErrDeleted) {
				code = "DELETED"
			} else if errors.Is(err, ErrExpired) {
				code = "EXPIRED"
			}
			refErrors = append(refErrors, RefError{Ref: ref, Code: code, Message: err.Error()})
			continue
		}
		values = append(values, *sv)
	}

	if len(refErrors) > 0 {
		return values, &getManyError{Values: values, RefErrors: refErrors}
	}
	return values, nil
}

// getManyError carries partial results alongside ref-level errors.
type getManyError struct {
	Values    []SecretValue
	RefErrors []RefError
}

func (e *getManyError) Error() string {
	return "strongbox: some refs could not be retrieved"
}

// ---------------------------------------------------------------------------
// List returns a page of secret headers.
// ---------------------------------------------------------------------------

func (s *Store) List(ctx context.Context, scope Scope, opts ListOptions) (*ListResult, error) {
	start := time.Now()
	if err := ValidateScope(scope); err != nil {
		return nil, err
	}

	opts = normalizeListOptions(opts)
	result, err := s.provider.ListEntries(ctx, scope.ClientID, scope.TenantID, opts)
	if err != nil {
		s.emitAudit(ctx, "list", scope, "", nil, 0, start, err)
		return nil, err
	}

	s.emitAudit(ctx, "list", scope, "", nil, len(result.Secrets), start, nil)
	return &result, nil
}

// ---------------------------------------------------------------------------
// Delete soft-deletes a secret.
// ---------------------------------------------------------------------------

func (s *Store) Delete(ctx context.Context, scope Scope, ref SecretRef) error {
	start := time.Now()
	if err := ValidateScope(scope); err != nil {
		return err
	}
	if err := ValidateSecretRef(ref); err != nil {
		return err
	}

	err := s.provider.DeleteEntry(ctx, scope.ClientID, scope.TenantID, ref)
	s.emitAudit(ctx, "delete", scope, ref, nil, 0, start, err)
	return err
}

// ---------------------------------------------------------------------------
// DeleteMany soft-deletes multiple secrets.
// ---------------------------------------------------------------------------

func (s *Store) DeleteMany(ctx context.Context, scope Scope, refs []SecretRef) error {
	start := time.Now()
	if err := ValidateScope(scope); err != nil {
		return err
	}
	for _, ref := range refs {
		if err := ValidateSecretRef(ref); err != nil {
			return err
		}
	}

	err := s.provider.DeleteEntries(ctx, scope.ClientID, scope.TenantID, refs)
	s.emitAudit(ctx, "delete_many", scope, "", refs, len(refs), start, err)
	return err
}

// ---------------------------------------------------------------------------
// Sync performs a bulk sync operation.
// ---------------------------------------------------------------------------

func (s *Store) Sync(ctx context.Context, scope Scope, input SyncInput) (*SyncResult, error) {
	start := time.Now()
	if err := ValidateScope(scope); err != nil {
		return nil, err
	}

	// Upsert all inputs via BatchPut.
	results, err := s.BatchPut(ctx, scope, input.Secrets)
	var refErrors []RefError
	if err != nil {
		var be *batchError
		if errors.As(err, &be) {
			results = be.Results
			refErrors = be.RefErrors
		} else {
			return nil, err
		}
	}

	var deleted []SecretHeader

	if input.SyncMode == SyncFull {
		// Collect all existing refs by paginating through all results.
		var allHeaders []SecretHeader
		cursor := ""
		for {
			result, err := s.provider.ListEntries(ctx, scope.ClientID, scope.TenantID, ListOptions{
				Limit:  maxListLimit,
				Cursor: cursor,
			})
			if err != nil {
				return nil, err
			}
			allHeaders = append(allHeaders, result.Secrets...)
			if !result.HasMore {
				break
			}
			cursor = result.Cursor
		}

		// Build set of input refs.
		inputRefs := make(map[SecretRef]bool, len(input.Secrets))
		for _, inp := range input.Secrets {
			inputRefs[inp.Ref] = true
		}

		// Find refs to delete (exist in store but not in input).
		var toDelete []SecretRef
		for _, hdr := range allHeaders {
			if !inputRefs[hdr.Ref] {
				toDelete = append(toDelete, hdr.Ref)
				deleted = append(deleted, hdr)
			}
		}

		if len(toDelete) > 0 {
			if err := s.provider.DeleteEntries(ctx, scope.ClientID, scope.TenantID, toDelete); err != nil {
				return nil, err
			}
		}
	}

	s.emitAudit(ctx, "sync", scope, "", nil, len(results), start, nil)
	return &SyncResult{
		Synced:  results,
		Deleted: deleted,
		Errors:  refErrors,
	}, nil
}

// ---------------------------------------------------------------------------
// RotateKeys re-encrypts all secrets from the previous key to the current key.
// ---------------------------------------------------------------------------

func (s *Store) RotateKeys(ctx context.Context, scope Scope) error {
	start := time.Now()
	if err := ValidateScope(scope); err != nil {
		return err
	}
	if len(s.prevKey) == 0 {
		return ErrKeyUnavailable
	}

	entries, err := s.provider.ListByKeyID(ctx, scope.ClientID, scope.TenantID, s.prevKeyID)
	if err != nil {
		s.emitAudit(ctx, "rotate_keys", scope, "", nil, 0, start, err)
		return err
	}

	oldDK, err := DeriveKey(s.prevKey, scope.ClientID, scope.TenantID)
	if err != nil {
		return err
	}
	defer zero(oldDK)

	newDK, err := DeriveKey(s.rootKey, scope.ClientID, scope.TenantID)
	if err != nil {
		return err
	}
	defer zero(newDK)

	for _, entry := range entries {
		plaintext, err := s.enc.Decrypt(oldDK, entry.Ciphertext)
		if err != nil {
			s.emitAudit(ctx, "rotate_keys", scope, entry.Ref, nil, 0, start, err)
			return err
		}

		newCT, err := s.enc.Encrypt(newDK, plaintext)
		zero(plaintext)
		if err != nil {
			s.emitAudit(ctx, "rotate_keys", scope, entry.Ref, nil, 0, start, err)
			return err
		}

		entry.Ciphertext = newCT
		entry.KeyID = s.rootKeyID
		entry.UpdatedAt = time.Now()

		if _, err := s.provider.PutEntry(ctx, entry); err != nil {
			s.emitAudit(ctx, "rotate_keys", scope, entry.Ref, nil, 0, start, err)
			return err
		}
	}

	s.emitAudit(ctx, "rotate_keys", scope, "", nil, len(entries), start, nil)
	return nil
}

// ---------------------------------------------------------------------------
// HardDelete permanently removes a secret.
// ---------------------------------------------------------------------------

func (s *Store) HardDelete(ctx context.Context, scope Scope, ref SecretRef) error {
	start := time.Now()
	if err := ValidateScope(scope); err != nil {
		return err
	}
	if err := ValidateSecretRef(ref); err != nil {
		return err
	}

	err := s.provider.HardDeleteEntry(ctx, scope.ClientID, scope.TenantID, ref)
	s.emitAudit(ctx, "hard_delete", scope, ref, nil, 0, start, err)
	return err
}

// ---------------------------------------------------------------------------
// HardDeleteTenant permanently removes all secrets for a tenant.
// ---------------------------------------------------------------------------

func (s *Store) HardDeleteTenant(ctx context.Context, scope Scope) error {
	start := time.Now()
	if err := ValidateScope(scope); err != nil {
		return err
	}

	err := s.provider.HardDeleteTenant(ctx, scope.ClientID, scope.TenantID)
	s.emitAudit(ctx, "hard_delete_tenant", scope, "", nil, 0, start, err)
	return err
}

// HealthCheck delegates to the provider's Ping method.
func (s *Store) HealthCheck(ctx context.Context) error {
	return s.provider.Ping(ctx)
}