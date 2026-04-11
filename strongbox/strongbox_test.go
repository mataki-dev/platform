// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package strongbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ClientID validation
// ---------------------------------------------------------------------------

func TestValidateClientID_Valid(t *testing.T) {
	valid := []string{
		"a",
		"abc",
		"my-client",
		"client-123",
		"a1b2c3",
		// 63 chars (max)
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	for _, v := range valid {
		if err := ValidateClientID(ClientID(v)); err != nil {
			t.Errorf("ValidateClientID(%q) = %v; want nil", v, err)
		}
	}
}

func TestValidateClientID_Invalid(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"uppercase", "MyClient"},
		{"underscore", "my_client"},
		{"starts-with-hyphen", "-client"},
		{"ends-with-hyphen", "client-"},
		{"space", "my client"},
		{"special", "client@1"},
		{"too-long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, // 64 chars
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateClientID(ClientID(tc.input))
			if err == nil {
				t.Errorf("ValidateClientID(%q) = nil; want error", tc.input)
			}
		})
	}
}

func TestValidateClientID_SentinelError(t *testing.T) {
	err := ValidateClientID(ClientID(""))
	if err != ErrInvalidClient {
		t.Errorf("ValidateClientID(\"\") = %v; want ErrInvalidClient", err)
	}
}

// ---------------------------------------------------------------------------
// TenantID validation
// ---------------------------------------------------------------------------

func TestValidateTenantID_Valid(t *testing.T) {
	valid := []string{
		"t",
		"tenant-1",
		"org:team:project",
		"anything-goes_here.123",
	}
	for _, v := range valid {
		if err := ValidateTenantID(TenantID(v)); err != nil {
			t.Errorf("ValidateTenantID(%q) = %v; want nil", v, err)
		}
	}
}

func TestValidateTenantID_Invalid(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"too-long", string(make([]byte, 256))}, // 256 chars
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTenantID(TenantID(tc.input))
			if err == nil {
				t.Errorf("ValidateTenantID(%q) = nil; want error", tc.input)
			}
		})
	}
}

func TestValidateTenantID_SentinelError(t *testing.T) {
	err := ValidateTenantID(TenantID(""))
	if err != ErrInvalidTenant {
		t.Errorf("ValidateTenantID(\"\") = %v; want ErrInvalidTenant", err)
	}
}

// ---------------------------------------------------------------------------
// SecretRef validation
// ---------------------------------------------------------------------------

func TestValidateSecretRef_Valid(t *testing.T) {
	valid := []string{
		"a",
		"A",
		"0",
		"my-secret",
		"db.password",
		"config:key",
		"Secret_Name-1.0",
		"a" + string(make([]byte, 511)), // will fix below
	}
	// Build a 512-char ref starting with 'a'
	buf := make([]byte, 512)
	buf[0] = 'a'
	for i := 1; i < 512; i++ {
		buf[i] = 'b'
	}
	valid[len(valid)-1] = string(buf)

	for _, v := range valid {
		if err := ValidateSecretRef(SecretRef(v)); err != nil {
			t.Errorf("ValidateSecretRef(%q) = %v; want nil", v, err)
		}
	}
}

func TestValidateSecretRef_Invalid(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"starts-with-hyphen", "-key"},
		{"starts-with-dot", ".key"},
		{"starts-with-underscore", "_key"},
		{"space", "my key"},
		{"slash", "my/key"},
		{"too-long-513", func() string {
			buf := make([]byte, 513)
			buf[0] = 'a'
			for i := 1; i < 513; i++ {
				buf[i] = 'b'
			}
			return string(buf)
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSecretRef(SecretRef(tc.input))
			if err == nil {
				t.Errorf("ValidateSecretRef(%q) = nil; want error", tc.input)
			}
		})
	}
}

func TestValidateSecretRef_SentinelError(t *testing.T) {
	err := ValidateSecretRef(SecretRef(""))
	if err != ErrInvalidRef {
		t.Errorf("ValidateSecretRef(\"\") = %v; want ErrInvalidRef", err)
	}
}

// ---------------------------------------------------------------------------
// Scope validation
// ---------------------------------------------------------------------------

func TestValidateScope_Valid(t *testing.T) {
	s := Scope{ClientID: "my-client", TenantID: "my-tenant"}
	if err := ValidateScope(s); err != nil {
		t.Errorf("ValidateScope(%+v) = %v; want nil", s, err)
	}
}

func TestValidateScope_InvalidClient(t *testing.T) {
	s := Scope{ClientID: "", TenantID: "my-tenant"}
	err := ValidateScope(s)
	if err == nil {
		t.Fatal("ValidateScope with empty ClientID should fail")
	}
	if err != ErrInvalidClient {
		t.Errorf("got %v; want ErrInvalidClient", err)
	}
}

func TestValidateScope_InvalidTenant(t *testing.T) {
	s := Scope{ClientID: "my-client", TenantID: ""}
	err := ValidateScope(s)
	if err == nil {
		t.Fatal("ValidateScope with empty TenantID should fail")
	}
	if err != ErrInvalidTenant {
		t.Errorf("got %v; want ErrInvalidTenant", err)
	}
}

// ---------------------------------------------------------------------------
// Type sanity checks
// ---------------------------------------------------------------------------

func TestSecretValueFields(t *testing.T) {
	now := time.Now()
	expires := now.Add(time.Hour)
	sv := SecretValue{
		Ref:       "my-ref",
		Value:     "secret-data",
		Version:   3,
		Metadata:  map[string]string{"env": "prod"},
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: &expires,
	}
	if sv.Ref != "my-ref" {
		t.Error("Ref mismatch")
	}
	if sv.Value != "secret-data" {
		t.Error("Value mismatch")
	}
	if sv.Version != 3 {
		t.Error("Version mismatch")
	}
	if sv.ExpiresAt == nil || !sv.ExpiresAt.Equal(expires) {
		t.Error("ExpiresAt mismatch")
	}
}

func TestSecretHeaderFields(t *testing.T) {
	now := time.Now()
	sh := SecretHeader{
		Ref:       "h-ref",
		Version:   1,
		Metadata:  map[string]string{"k": "v"},
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: nil,
	}
	if sh.Ref != "h-ref" {
		t.Error("Ref mismatch")
	}
	if sh.ExpiresAt != nil {
		t.Error("ExpiresAt should be nil")
	}
}

func TestStoredEntryFields(t *testing.T) {
	now := time.Now()
	se := StoredEntry{
		ClientID:   "client",
		TenantID:   "tenant",
		Ref:        "ref",
		Ciphertext: []byte("encrypted"),
		KeyID:      "key-1",
		Version:    2,
		Metadata:   map[string]string{},
		CreatedAt:  now,
		UpdatedAt:  now,
		ExpiresAt:  nil,
		DeletedAt:  nil,
	}
	if se.KeyID != "key-1" {
		t.Error("KeyID mismatch")
	}
}

func TestListOptionsDefaults(t *testing.T) {
	lo := ListOptions{}
	if lo.Limit != 0 {
		t.Error("zero value Limit should be 0 (caller normalizes)")
	}
	if lo.SortField != "" {
		t.Error("zero value SortField should be empty")
	}
}

func TestSyncModeConstants(t *testing.T) {
	if SyncPartial != "partial" {
		t.Errorf("SyncPartial = %q; want \"partial\"", SyncPartial)
	}
	if SyncFull != "full" {
		t.Errorf("SyncFull = %q; want \"full\"", SyncFull)
	}
}

func TestPutInputFields(t *testing.T) {
	pi := PutInput{
		Ref:   "db-pass",
		Value: "hunter2",
	}
	if pi.Ref != "db-pass" {
		t.Error("Ref mismatch")
	}
}

func TestRefErrorFields(t *testing.T) {
	re := RefError{
		Ref:     "bad-ref",
		Code:    "NOT_FOUND",
		Message: "secret not found",
	}
	if re.Error() == "" {
		t.Error("RefError.Error() should not be empty")
	}
	if re.Code != "NOT_FOUND" {
		t.Error("Code mismatch")
	}
	if re.Message != "secret not found" {
		t.Error("Message mismatch")
	}
}

func TestSentinelErrors(t *testing.T) {
	errors := []error{
		ErrNotFound, ErrDeleted, ErrExpired, ErrInvalidRef,
		ErrInvalidClient, ErrInvalidTenant, ErrValueTooLarge,
		ErrBatchTooLarge, ErrDecryptFailed, ErrKeyUnavailable,
	}
	for _, e := range errors {
		if e == nil {
			t.Error("sentinel error is nil")
		}
		if e.Error() == "" {
			t.Error("sentinel error message is empty")
		}
	}
}

func TestAuditEventFields(t *testing.T) {
	ae := AuditEvent{
		Timestamp: time.Now(),
		Operation: "get",
		ClientID:  "c",
		TenantID:  "t",
		Ref:       "r",
	}
	if ae.Operation != "get" {
		t.Error("Operation mismatch")
	}
}

// ---------------------------------------------------------------------------
// Envelope encryption tests
// ---------------------------------------------------------------------------

func makeKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestEnvelopeEncryptDecrypt(t *testing.T) {
	enc := NewEnvelopeEncryptor()
	key := makeKey(t)
	plaintext := []byte("hello, strongbox!")

	ct, err := enc.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Format version byte must be 0x01.
	if ct[0] != 0x01 {
		t.Errorf("format version = 0x%02x; want 0x01", ct[0])
	}

	// Ciphertext size = 89 + len(plaintext).
	want := 89 + len(plaintext)
	if len(ct) != want {
		t.Errorf("ciphertext len = %d; want %d", len(ct), want)
	}

	got, err := enc.Decrypt(key, ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("Decrypt = %q; want %q", got, plaintext)
	}
}

func TestEnvelopeDecryptWrongKey(t *testing.T) {
	enc := NewEnvelopeEncryptor()
	key1 := makeKey(t)
	key2 := makeKey(t)

	ct, err := enc.Encrypt(key1, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = enc.Decrypt(key2, ct)
	if err == nil {
		t.Fatal("Decrypt with wrong key should fail")
	}
	if !errors.Is(err, ErrDecryptFailed) {
		t.Errorf("err = %v; want ErrDecryptFailed", err)
	}
}

func TestEnvelopeDecryptCorrupted(t *testing.T) {
	enc := NewEnvelopeEncryptor()
	key := makeKey(t)

	ct, err := enc.Encrypt(key, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt the last byte.
	ct[len(ct)-1] ^= 0xFF

	_, err = enc.Decrypt(key, ct)
	if err == nil {
		t.Fatal("Decrypt of corrupted ciphertext should fail")
	}
	if !errors.Is(err, ErrDecryptFailed) {
		t.Errorf("err = %v; want ErrDecryptFailed", err)
	}
}

func TestEnvelopeDecryptTooShort(t *testing.T) {
	enc := NewEnvelopeEncryptor()
	key := makeKey(t)

	_, err := enc.Decrypt(key, make([]byte, 88))
	if err == nil {
		t.Fatal("Decrypt of too-short ciphertext should fail")
	}
	if !errors.Is(err, ErrDecryptFailed) {
		t.Errorf("err = %v; want ErrDecryptFailed", err)
	}
}

func TestEnvelopeDecryptBadVersion(t *testing.T) {
	enc := NewEnvelopeEncryptor()
	key := makeKey(t)

	ct, err := enc.Encrypt(key, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	ct[0] = 0x99

	_, err = enc.Decrypt(key, ct)
	if err == nil {
		t.Fatal("Decrypt with bad version should fail")
	}
	if !errors.Is(err, ErrDecryptFailed) {
		t.Errorf("err = %v; want ErrDecryptFailed", err)
	}
}

// ---------------------------------------------------------------------------
// Key derivation tests
// ---------------------------------------------------------------------------

func TestDeriveKey(t *testing.T) {
	rootKey := makeKey(t)

	// Same inputs produce same key.
	k1, err := DeriveKey(rootKey, "client-a", "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	k2, err := DeriveKey(rootKey, "client-a", "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(k1, k2) {
		t.Error("same inputs should produce same derived key")
	}

	// Key is 32 bytes.
	if len(k1) != 32 {
		t.Errorf("derived key len = %d; want 32", len(k1))
	}

	// Different client produces different key.
	k3, err := DeriveKey(rootKey, "client-b", "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(k1, k3) {
		t.Error("different clientID should produce different key")
	}

	// Different tenant produces different key.
	k4, err := DeriveKey(rootKey, "client-a", "tenant-2")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(k1, k4) {
		t.Error("different tenantID should produce different key")
	}
}

// ---------------------------------------------------------------------------
// fakeProvider — stateful in-memory Provider for testing
// ---------------------------------------------------------------------------

type entryKey struct {
	clientID ClientID
	tenantID TenantID
	ref      SecretRef
}

type fakeProvider struct {
	entries map[entryKey]StoredEntry
	nextVer int64
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{
		entries: make(map[entryKey]StoredEntry),
		nextVer: 1,
	}
}

func (f *fakeProvider) key(c ClientID, t TenantID, r SecretRef) entryKey {
	return entryKey{clientID: c, tenantID: t, ref: r}
}

func (f *fakeProvider) PutEntry(_ context.Context, e StoredEntry) (int64, error) {
	k := f.key(e.ClientID, e.TenantID, e.Ref)
	v := f.nextVer
	f.nextVer++
	e.Version = v
	e.DeletedAt = nil
	f.entries[k] = e
	return v, nil
}

func (f *fakeProvider) PutEntries(ctx context.Context, entries []StoredEntry) ([]int64, error) {
	versions := make([]int64, len(entries))
	for i, e := range entries {
		v, err := f.PutEntry(ctx, e)
		if err != nil {
			return nil, err
		}
		versions[i] = v
	}
	return versions, nil
}

func (f *fakeProvider) GetEntry(_ context.Context, c ClientID, t TenantID, ref SecretRef) (StoredEntry, error) {
	k := f.key(c, t, ref)
	e, ok := f.entries[k]
	if !ok {
		return StoredEntry{}, ErrNotFound
	}
	return e, nil
}

func (f *fakeProvider) ListEntries(_ context.Context, c ClientID, t TenantID, opts ListOptions) (ListResult, error) {
	var headers []SecretHeader
	for k, e := range f.entries {
		if k.clientID != c || k.tenantID != t {
			continue
		}
		if e.DeletedAt != nil {
			continue
		}
		if opts.Prefix != "" && len(string(e.Ref)) >= len(opts.Prefix) {
			if string(e.Ref)[:len(opts.Prefix)] != opts.Prefix {
				continue
			}
		} else if opts.Prefix != "" {
			continue
		}
		headers = append(headers, SecretHeader{
			Ref:       e.Ref,
			Version:   e.Version,
			Metadata:  e.Metadata,
			CreatedAt: e.CreatedAt,
			UpdatedAt: e.UpdatedAt,
			ExpiresAt: e.ExpiresAt,
		})
	}
	return ListResult{Secrets: headers}, nil
}

func (f *fakeProvider) DeleteEntry(_ context.Context, c ClientID, t TenantID, ref SecretRef) error {
	k := f.key(c, t, ref)
	e, ok := f.entries[k]
	if !ok {
		return ErrNotFound
	}
	now := time.Now()
	e.DeletedAt = &now
	f.entries[k] = e
	return nil
}

func (f *fakeProvider) DeleteEntries(ctx context.Context, c ClientID, t TenantID, refs []SecretRef) error {
	for _, ref := range refs {
		if err := f.DeleteEntry(ctx, c, t, ref); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeProvider) HardDeleteEntry(_ context.Context, c ClientID, t TenantID, ref SecretRef) error {
	k := f.key(c, t, ref)
	if _, ok := f.entries[k]; !ok {
		return ErrNotFound
	}
	delete(f.entries, k)
	return nil
}

func (f *fakeProvider) HardDeleteTenant(_ context.Context, c ClientID, t TenantID) error {
	for k := range f.entries {
		if k.clientID == c && k.tenantID == t {
			delete(f.entries, k)
		}
	}
	return nil
}

func (f *fakeProvider) ListByKeyID(_ context.Context, c ClientID, t TenantID, keyID string) ([]StoredEntry, error) {
	var result []StoredEntry
	for k, e := range f.entries {
		if k.clientID == c && k.tenantID == t && e.KeyID == keyID && e.DeletedAt == nil {
			result = append(result, e)
		}
	}
	return result, nil
}

func (f *fakeProvider) Ping(_ context.Context) error { return nil }

// ---------------------------------------------------------------------------
// Store constructor tests
// ---------------------------------------------------------------------------

func TestNewStoreRequiresProvider(t *testing.T) {
	key := makeKey(t)
	_, err := NewStore(nil, WithKeyFromBytes(key))
	if err == nil {
		t.Fatal("NewStore(nil, ...) should return an error")
	}
}

func TestNewStoreRequiresExactlyOneKey(t *testing.T) {
	fp := newFakeProvider()

	// 0 keys → error
	_, err := NewStore(fp)
	if err == nil {
		t.Fatal("NewStore with no key option should return an error")
	}

	// 1 key → ok
	key := makeKey(t)
	s, err := NewStore(fp, WithKeyFromBytes(key))
	if err != nil {
		t.Fatalf("NewStore with 1 key: %v", err)
	}
	if s == nil {
		t.Fatal("NewStore returned nil Store")
	}

	// 2 keys → error
	key2 := makeKey(t)
	_, err = NewStore(fp, WithKeyFromBytes(key), WithKeyFromBytes(key2))
	if err == nil {
		t.Fatal("NewStore with 2 key options should return an error")
	}
}

func TestNewStoreRejectsWrongKeySize(t *testing.T) {
	fp := newFakeProvider()
	shortKey := make([]byte, 16)

	_, err := NewStore(fp, WithKeyFromBytes(shortKey))
	if err == nil {
		t.Fatal("NewStore with 16-byte key should return an error")
	}
}

func TestRootKeyID(t *testing.T) {
	key1 := makeKey(t)
	key2 := makeKey(t)

	id1 := RootKeyID(key1)
	id2 := RootKeyID(key2)

	// 16-char hex string.
	if len(id1) != 16 {
		t.Errorf("RootKeyID len = %d; want 16", len(id1))
	}

	// Deterministic.
	if RootKeyID(key1) != id1 {
		t.Error("RootKeyID should be deterministic")
	}

	// Different keys produce different IDs.
	if id1 == id2 {
		t.Error("different keys should produce different IDs")
	}
}

// ---------------------------------------------------------------------------
// Helper: create a Store with a fresh fakeProvider
// ---------------------------------------------------------------------------

func makeStore(t *testing.T) (*Store, *fakeProvider) {
	t.Helper()
	fp := newFakeProvider()
	key := makeKey(t)
	s, err := NewStore(fp, WithKeyFromBytes(key))
	if err != nil {
		t.Fatal(err)
	}
	return s, fp
}

var testScope = Scope{ClientID: "test-client", TenantID: "test-tenant"}

// ---------------------------------------------------------------------------
// Store method tests
// ---------------------------------------------------------------------------

func TestStorePutGet(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	res, err := s.Put(ctx, testScope, PutInput{Ref: "db-pass", Value: "hunter2"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if res.Action != "created" {
		t.Errorf("Action = %q; want created", res.Action)
	}
	if res.Version < 1 {
		t.Errorf("Version = %d; want >= 1", res.Version)
	}

	sv, err := s.Get(ctx, testScope, "db-pass")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sv.Value != "hunter2" {
		t.Errorf("Value = %q; want hunter2", sv.Value)
	}
	if sv.Version != res.Version {
		t.Errorf("Get Version = %d; want %d", sv.Version, res.Version)
	}

	// Put again should be "updated"
	res2, err := s.Put(ctx, testScope, PutInput{Ref: "db-pass", Value: "newpass"})
	if err != nil {
		t.Fatalf("Put update: %v", err)
	}
	if res2.Action != "updated" {
		t.Errorf("Action = %q; want updated", res2.Action)
	}
	if res2.PreviousVersion == nil {
		t.Fatal("PreviousVersion should not be nil for update")
	}
}

func TestStoreGetNotFound(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	_, err := s.Get(ctx, testScope, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v; want ErrNotFound", err)
	}
}

func TestStoreGetDeleted(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	_, err := s.Put(ctx, testScope, PutInput{Ref: "to-delete", Value: "val"})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(ctx, testScope, "to-delete"); err != nil {
		t.Fatal(err)
	}

	_, err = s.Get(ctx, testScope, "to-delete")
	if !errors.Is(err, ErrDeleted) {
		t.Errorf("err = %v; want ErrDeleted", err)
	}
}

func TestStoreGetExpired(t *testing.T) {
	s, fp := makeStore(t)
	ctx := context.Background()

	// Put a secret normally, then manipulate the provider entry to be expired.
	_, err := s.Put(ctx, testScope, PutInput{Ref: "expired-secret", Value: "val"})
	if err != nil {
		t.Fatal(err)
	}

	// Set ExpiresAt in the past directly on the provider.
	k := fp.key(testScope.ClientID, testScope.TenantID, "expired-secret")
	e := fp.entries[k]
	past := time.Now().Add(-time.Hour)
	e.ExpiresAt = &past
	fp.entries[k] = e

	_, err = s.Get(ctx, testScope, "expired-secret")
	if !errors.Is(err, ErrExpired) {
		t.Errorf("err = %v; want ErrExpired", err)
	}
}

func TestStorePutValidation(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	// Invalid ref
	_, err := s.Put(ctx, testScope, PutInput{Ref: "", Value: "val"})
	if !errors.Is(err, ErrInvalidRef) {
		t.Errorf("empty ref: err = %v; want ErrInvalidRef", err)
	}

	// Value too large
	big := make([]byte, defaultMaxValueSize+1)
	_, err = s.Put(ctx, testScope, PutInput{Ref: "big", Value: string(big)})
	if !errors.Is(err, ErrValueTooLarge) {
		t.Errorf("big value: err = %v; want ErrValueTooLarge", err)
	}

	// Invalid scope
	_, err = s.Put(ctx, Scope{ClientID: "", TenantID: "t"}, PutInput{Ref: "x", Value: "v"})
	if !errors.Is(err, ErrInvalidClient) {
		t.Errorf("bad scope: err = %v; want ErrInvalidClient", err)
	}
}

func TestStoreBatchPut(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	inputs := []PutInput{
		{Ref: "secret1", Value: "val1"},
		{Ref: "secret2", Value: "val2"},
	}
	results, err := s.BatchPut(ctx, testScope, inputs)
	if err != nil {
		t.Fatalf("BatchPut: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results; want 2", len(results))
	}

	// Verify both are readable.
	for _, inp := range inputs {
		sv, err := s.Get(ctx, testScope, inp.Ref)
		if err != nil {
			t.Errorf("Get(%s): %v", inp.Ref, err)
			continue
		}
		if sv.Value != inp.Value {
			t.Errorf("Get(%s).Value = %q; want %q", inp.Ref, sv.Value, inp.Value)
		}
	}
}

func TestStoreGetMany(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	s.Put(ctx, testScope, PutInput{Ref: "exists1", Value: "v1"})
	s.Put(ctx, testScope, PutInput{Ref: "exists2", Value: "v2"})

	values, err := s.GetMany(ctx, testScope, []SecretRef{"exists1", "exists2", "missing"})

	// Should get partial error since "missing" doesn't exist.
	if err == nil {
		t.Fatal("expected error for missing ref")
	}
	var gme *getManyError
	if !errors.As(err, &gme) {
		t.Fatalf("err type = %T; want *getManyError", err)
	}
	if len(values) != 2 {
		t.Errorf("got %d values; want 2", len(values))
	}
	if len(gme.RefErrors) != 1 {
		t.Errorf("got %d ref errors; want 1", len(gme.RefErrors))
	}
	if gme.RefErrors[0].Ref != "missing" {
		t.Errorf("ref error ref = %q; want missing", gme.RefErrors[0].Ref)
	}
}

func TestStoreList(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	s.Put(ctx, testScope, PutInput{Ref: "prefix-a", Value: "va"})
	s.Put(ctx, testScope, PutInput{Ref: "prefix-b", Value: "vb"})
	s.Put(ctx, testScope, PutInput{Ref: "other", Value: "vo"})

	result, err := s.List(ctx, testScope, ListOptions{Prefix: "prefix"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Secrets) != 2 {
		t.Errorf("got %d secrets; want 2", len(result.Secrets))
	}
}

func TestStoreDelete(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	s.Put(ctx, testScope, PutInput{Ref: "del-me", Value: "val"})
	if err := s.Delete(ctx, testScope, "del-me"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Get(ctx, testScope, "del-me")
	if !errors.Is(err, ErrDeleted) {
		t.Errorf("err = %v; want ErrDeleted", err)
	}
}

func TestStoreDeleteMany(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	s.Put(ctx, testScope, PutInput{Ref: "dm1", Value: "v1"})
	s.Put(ctx, testScope, PutInput{Ref: "dm2", Value: "v2"})

	if err := s.DeleteMany(ctx, testScope, []SecretRef{"dm1", "dm2"}); err != nil {
		t.Fatalf("DeleteMany: %v", err)
	}

	for _, ref := range []SecretRef{"dm1", "dm2"} {
		_, err := s.Get(ctx, testScope, ref)
		if !errors.Is(err, ErrDeleted) {
			t.Errorf("Get(%s) err = %v; want ErrDeleted", ref, err)
		}
	}
}

func TestStoreSyncPartial(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	// Pre-existing secret.
	s.Put(ctx, testScope, PutInput{Ref: "existing", Value: "old"})

	result, err := s.Sync(ctx, testScope, SyncInput{
		Secrets:  []PutInput{{Ref: "new-secret", Value: "new"}},
		SyncMode: SyncPartial,
	})
	if err != nil {
		t.Fatalf("Sync partial: %v", err)
	}
	if len(result.Synced) != 1 {
		t.Errorf("Synced = %d; want 1", len(result.Synced))
	}
	if len(result.Deleted) != 0 {
		t.Errorf("Deleted = %d; want 0 (partial sync)", len(result.Deleted))
	}

	// Existing secret should still be readable.
	sv, err := s.Get(ctx, testScope, "existing")
	if err != nil {
		t.Fatalf("existing secret should survive partial sync: %v", err)
	}
	if sv.Value != "old" {
		t.Errorf("existing value = %q; want old", sv.Value)
	}
}

func TestStoreSyncFull(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	s.Put(ctx, testScope, PutInput{Ref: "keep", Value: "v1"})
	s.Put(ctx, testScope, PutInput{Ref: "remove", Value: "v2"})

	result, err := s.Sync(ctx, testScope, SyncInput{
		Secrets:  []PutInput{{Ref: "keep", Value: "v1-updated"}},
		SyncMode: SyncFull,
	})
	if err != nil {
		t.Fatalf("Sync full: %v", err)
	}
	if len(result.Synced) != 1 {
		t.Errorf("Synced = %d; want 1", len(result.Synced))
	}
	if len(result.Deleted) != 1 {
		t.Errorf("Deleted = %d; want 1", len(result.Deleted))
	}

	// "remove" should be soft-deleted.
	_, err = s.Get(ctx, testScope, "remove")
	if !errors.Is(err, ErrDeleted) {
		t.Errorf("removed secret: err = %v; want ErrDeleted", err)
	}

	// "keep" should be updated.
	sv, err := s.Get(ctx, testScope, "keep")
	if err != nil {
		t.Fatalf("kept secret: %v", err)
	}
	if sv.Value != "v1-updated" {
		t.Errorf("kept value = %q; want v1-updated", sv.Value)
	}
}

func TestStoreRotateKeys(t *testing.T) {
	fp := newFakeProvider()
	oldKey := makeKey(t)
	newKey := makeKey(t)
	ctx := context.Background()

	// Create store with old key and put secrets.
	oldStore, err := NewStore(fp, WithKeyFromBytes(oldKey))
	if err != nil {
		t.Fatal(err)
	}

	oldStore.Put(ctx, testScope, PutInput{Ref: "rot1", Value: "secret1"})
	oldStore.Put(ctx, testScope, PutInput{Ref: "rot2", Value: "secret2"})

	// Create new store with new key + old key for rotation.
	newStore, err := NewStore(fp, WithKeyFromBytes(newKey), WithPreviousKeyFromBytes(oldKey))
	if err != nil {
		t.Fatal(err)
	}

	if err := newStore.RotateKeys(ctx, testScope); err != nil {
		t.Fatalf("RotateKeys: %v", err)
	}

	// Verify secrets are readable with new store.
	sv, err := newStore.Get(ctx, testScope, "rot1")
	if err != nil {
		t.Fatalf("Get rot1 after rotate: %v", err)
	}
	if sv.Value != "secret1" {
		t.Errorf("rot1 value = %q; want secret1", sv.Value)
	}

	sv, err = newStore.Get(ctx, testScope, "rot2")
	if err != nil {
		t.Fatalf("Get rot2 after rotate: %v", err)
	}
	if sv.Value != "secret2" {
		t.Errorf("rot2 value = %q; want secret2", sv.Value)
	}
}

func TestStoreRotateKeysNoPrevKey(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	err := s.RotateKeys(ctx, testScope)
	if !errors.Is(err, ErrKeyUnavailable) {
		t.Errorf("err = %v; want ErrKeyUnavailable", err)
	}
}

func TestStoreHardDelete(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	s.Put(ctx, testScope, PutInput{Ref: "hard-del", Value: "val"})
	if err := s.HardDelete(ctx, testScope, "hard-del"); err != nil {
		t.Fatalf("HardDelete: %v", err)
	}

	_, err := s.Get(ctx, testScope, "hard-del")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v; want ErrNotFound", err)
	}
}

func TestStoreHardDeleteTenant(t *testing.T) {
	s, _ := makeStore(t)
	ctx := context.Background()

	s.Put(ctx, testScope, PutInput{Ref: "hdt1", Value: "v1"})
	s.Put(ctx, testScope, PutInput{Ref: "hdt2", Value: "v2"})

	if err := s.HardDeleteTenant(ctx, testScope); err != nil {
		t.Fatalf("HardDeleteTenant: %v", err)
	}

	for _, ref := range []SecretRef{"hdt1", "hdt2"} {
		_, err := s.Get(ctx, testScope, ref)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%s) err = %v; want ErrNotFound", ref, err)
		}
	}
}