// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

// Package conformance provides an importable test suite that any
// strongbox.Provider implementation can use to verify correctness.
package conformance

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/mataki-dev/platform/strongbox"
)

// RunProviderSuite runs all 14 conformance tests from the spec against p.
func RunProviderSuite(t *testing.T, p strongbox.Provider) {
	t.Helper()

	t.Run("PutGetRoundTrip", func(t *testing.T) { testPutGetRoundTrip(t, p) })
	t.Run("VersionIncrementing", func(t *testing.T) { testVersionIncrementing(t, p) })
	t.Run("PutEntriesAtomicity", func(t *testing.T) { testPutEntriesAtomicity(t, p) })
	t.Run("SoftDelete", func(t *testing.T) { testSoftDelete(t, p) })
	t.Run("HardDelete", func(t *testing.T) { testHardDelete(t, p) })
	t.Run("HardDeleteTenant", func(t *testing.T) { testHardDeleteTenant(t, p) })
	t.Run("ListWithPrefix", func(t *testing.T) { testListWithPrefix(t, p) })
	t.Run("ListPagination", func(t *testing.T) { testListPagination(t, p) })
	t.Run("ListOrdering", func(t *testing.T) { testListOrdering(t, p) })
	t.Run("ListByKeyID", func(t *testing.T) { testListByKeyID(t, p) })
	t.Run("ClientIsolation", func(t *testing.T) { testClientIsolation(t, p) })
	t.Run("TenantIsolation", func(t *testing.T) { testTenantIsolation(t, p) })
	t.Run("Ping", func(t *testing.T) { testPing(t, p) })
	t.Run("DeleteEntries", func(t *testing.T) { testDeleteEntries(t, p) })
}

func ctx() context.Context { return context.Background() }

func makeEntry(client strongbox.ClientID, tenant strongbox.TenantID, ref strongbox.SecretRef, keyID string) strongbox.StoredEntry {
	return strongbox.StoredEntry{
		ClientID:   client,
		TenantID:   tenant,
		Ref:        ref,
		Ciphertext: []byte("cipher-" + string(ref)),
		KeyID:      keyID,
		Metadata:   map[string]string{"env": "test"},
	}
}

// 1. PutEntry / GetEntry round-trip.
func testPutGetRoundTrip(t *testing.T, p strongbox.Provider) {
	client := strongbox.ClientID("c-roundtrip")
	tenant := strongbox.TenantID("t-roundtrip")
	ref := strongbox.SecretRef("roundtrip-secret")

	entry := makeEntry(client, tenant, ref, "key1")
	entry.Ciphertext = []byte("encrypted-data")
	entry.Metadata = map[string]string{"foo": "bar"}

	ver, err := p.PutEntry(ctx(), entry)
	if err != nil {
		t.Fatalf("PutEntry: %v", err)
	}
	if ver != 1 {
		t.Fatalf("expected version 1, got %d", ver)
	}

	got, err := p.GetEntry(ctx(), client, tenant, ref)
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if string(got.Ciphertext) != "encrypted-data" {
		t.Errorf("ciphertext mismatch: %q", got.Ciphertext)
	}
	if got.KeyID != "key1" {
		t.Errorf("keyID mismatch: %q", got.KeyID)
	}
	if got.Version != 1 {
		t.Errorf("version mismatch: %d", got.Version)
	}
	if got.Metadata["foo"] != "bar" {
		t.Errorf("metadata mismatch: %v", got.Metadata)
	}
}

// 2. Version incrementing.
func testVersionIncrementing(t *testing.T, p strongbox.Provider) {
	client := strongbox.ClientID("c-verinc")
	tenant := strongbox.TenantID("t-verinc")
	ref := strongbox.SecretRef("verinc-secret")

	for i := int64(1); i <= 3; i++ {
		ver, err := p.PutEntry(ctx(), makeEntry(client, tenant, ref, "k1"))
		if err != nil {
			t.Fatalf("PutEntry iter %d: %v", i, err)
		}
		if ver != i {
			t.Fatalf("expected version %d, got %d", i, ver)
		}
	}
}

// 3. PutEntries atomicity.
func testPutEntriesAtomicity(t *testing.T, p strongbox.Provider) {
	client := strongbox.ClientID("c-batch")
	tenant := strongbox.TenantID("t-batch")

	entries := make([]strongbox.StoredEntry, 5)
	for i := range entries {
		entries[i] = makeEntry(client, tenant, strongbox.SecretRef(fmt.Sprintf("batch-%d", i)), "k1")
	}

	versions, err := p.PutEntries(ctx(), entries)
	if err != nil {
		t.Fatalf("PutEntries: %v", err)
	}
	if len(versions) != 5 {
		t.Fatalf("expected 5 versions, got %d", len(versions))
	}
	for i, v := range versions {
		if v != 1 {
			t.Errorf("entry %d: expected version 1, got %d", i, v)
		}
	}

	// Verify all entries exist.
	for i := 0; i < 5; i++ {
		ref := strongbox.SecretRef(fmt.Sprintf("batch-%d", i))
		if _, err := p.GetEntry(ctx(), client, tenant, ref); err != nil {
			t.Errorf("GetEntry(%s): %v", ref, err)
		}
	}
}

// 4. Soft delete.
func testSoftDelete(t *testing.T, p strongbox.Provider) {
	client := strongbox.ClientID("c-softdel")
	tenant := strongbox.TenantID("t-softdel")
	ref := strongbox.SecretRef("softdel-secret")

	if _, err := p.PutEntry(ctx(), makeEntry(client, tenant, ref, "k1")); err != nil {
		t.Fatalf("PutEntry: %v", err)
	}

	if err := p.DeleteEntry(ctx(), client, tenant, ref); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}

	got, err := p.GetEntry(ctx(), client, tenant, ref)
	if err != nil {
		t.Fatalf("GetEntry after soft-delete: %v", err)
	}
	if got.DeletedAt == nil {
		t.Fatal("expected DeletedAt to be set after soft-delete")
	}

	// ListEntries should exclude soft-deleted entries.
	result, err := p.ListEntries(ctx(), client, tenant, strongbox.ListOptions{})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	for _, h := range result.Secrets {
		if h.Ref == ref {
			t.Error("soft-deleted entry should not appear in ListEntries")
		}
	}
}

// 5. Hard delete.
func testHardDelete(t *testing.T, p strongbox.Provider) {
	client := strongbox.ClientID("c-harddel")
	tenant := strongbox.TenantID("t-harddel")
	ref := strongbox.SecretRef("harddel-secret")

	if _, err := p.PutEntry(ctx(), makeEntry(client, tenant, ref, "k1")); err != nil {
		t.Fatalf("PutEntry: %v", err)
	}

	if err := p.HardDeleteEntry(ctx(), client, tenant, ref); err != nil {
		t.Fatalf("HardDeleteEntry: %v", err)
	}

	_, err := p.GetEntry(ctx(), client, tenant, ref)
	if !errors.Is(err, strongbox.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after hard delete, got: %v", err)
	}
}

// 6. HardDeleteTenant.
func testHardDeleteTenant(t *testing.T, p strongbox.Provider) {
	client := strongbox.ClientID("c-hardtenant")
	tenantA := strongbox.TenantID("t-hardtenant-a")
	tenantB := strongbox.TenantID("t-hardtenant-b")

	for i := 0; i < 5; i++ {
		ref := strongbox.SecretRef(fmt.Sprintf("hdt-a-%d", i))
		if _, err := p.PutEntry(ctx(), makeEntry(client, tenantA, ref, "k1")); err != nil {
			t.Fatalf("PutEntry tenantA: %v", err)
		}
	}
	for i := 0; i < 5; i++ {
		ref := strongbox.SecretRef(fmt.Sprintf("hdt-b-%d", i))
		if _, err := p.PutEntry(ctx(), makeEntry(client, tenantB, ref, "k1")); err != nil {
			t.Fatalf("PutEntry tenantB: %v", err)
		}
	}

	if err := p.HardDeleteTenant(ctx(), client, tenantA); err != nil {
		t.Fatalf("HardDeleteTenant: %v", err)
	}

	// Tenant A entries should be gone.
	for i := 0; i < 5; i++ {
		ref := strongbox.SecretRef(fmt.Sprintf("hdt-a-%d", i))
		_, err := p.GetEntry(ctx(), client, tenantA, ref)
		if !errors.Is(err, strongbox.ErrNotFound) {
			t.Errorf("tenantA entry %d: expected ErrNotFound, got %v", i, err)
		}
	}

	// Tenant B entries should remain.
	for i := 0; i < 5; i++ {
		ref := strongbox.SecretRef(fmt.Sprintf("hdt-b-%d", i))
		if _, err := p.GetEntry(ctx(), client, tenantB, ref); err != nil {
			t.Errorf("tenantB entry %d: unexpected error: %v", i, err)
		}
	}
}

// 7. List with prefix.
func testListWithPrefix(t *testing.T, p strongbox.Provider) {
	client := strongbox.ClientID("c-prefix")
	tenant := strongbox.TenantID("t-prefix")

	refs := []strongbox.SecretRef{"a:1", "a:2", "b:1"}
	for _, ref := range refs {
		if _, err := p.PutEntry(ctx(), makeEntry(client, tenant, ref, "k1")); err != nil {
			t.Fatalf("PutEntry(%s): %v", ref, err)
		}
	}

	result, err := p.ListEntries(ctx(), client, tenant, strongbox.ListOptions{Prefix: "a:"})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(result.Secrets) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Secrets))
	}
	for _, h := range result.Secrets {
		if h.Ref != "a:1" && h.Ref != "a:2" {
			t.Errorf("unexpected ref in prefix results: %s", h.Ref)
		}
	}
}

// 8. List pagination.
func testListPagination(t *testing.T, p strongbox.Provider) {
	client := strongbox.ClientID("c-page")
	tenant := strongbox.TenantID("t-page")

	for i := 0; i < 25; i++ {
		ref := strongbox.SecretRef(fmt.Sprintf("page-%02d", i))
		if _, err := p.PutEntry(ctx(), makeEntry(client, tenant, ref, "k1")); err != nil {
			t.Fatalf("PutEntry: %v", err)
		}
	}

	// Page 1: 10 items, HasMore=true
	page1, err := p.ListEntries(ctx(), client, tenant, strongbox.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1.Secrets) != 10 {
		t.Fatalf("page1: expected 10, got %d", len(page1.Secrets))
	}
	if !page1.HasMore {
		t.Fatal("page1: expected HasMore=true")
	}

	// Page 2: 10 items, HasMore=true
	page2, err := p.ListEntries(ctx(), client, tenant, strongbox.ListOptions{Limit: 10, Cursor: page1.Cursor})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2.Secrets) != 10 {
		t.Fatalf("page2: expected 10, got %d", len(page2.Secrets))
	}
	if !page2.HasMore {
		t.Fatal("page2: expected HasMore=true")
	}

	// Page 3: 5 items, HasMore=false
	page3, err := p.ListEntries(ctx(), client, tenant, strongbox.ListOptions{Limit: 10, Cursor: page2.Cursor})
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page3.Secrets) != 5 {
		t.Fatalf("page3: expected 5, got %d", len(page3.Secrets))
	}
	if page3.HasMore {
		t.Fatal("page3: expected HasMore=false")
	}
}

// 9. List ordering.
func testListOrdering(t *testing.T, p strongbox.Provider) {
	client := strongbox.ClientID("c-order")
	tenant := strongbox.TenantID("t-order")

	refs := []strongbox.SecretRef{"zulu", "alpha", "mike", "bravo"}
	for _, ref := range refs {
		if _, err := p.PutEntry(ctx(), makeEntry(client, tenant, ref, "k1")); err != nil {
			t.Fatalf("PutEntry(%s): %v", ref, err)
		}
	}

	result, err := p.ListEntries(ctx(), client, tenant, strongbox.ListOptions{})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}

	got := make([]string, len(result.Secrets))
	for i, h := range result.Secrets {
		got[i] = string(h.Ref)
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("results not sorted ascending: %v", got)
	}
}

// 10. ListByKeyID.
func testListByKeyID(t *testing.T, p strongbox.Provider) {
	client := strongbox.ClientID("c-bykey")
	tenant := strongbox.TenantID("t-bykey")

	for i := 0; i < 3; i++ {
		ref := strongbox.SecretRef(fmt.Sprintf("bykey-k1-%d", i))
		if _, err := p.PutEntry(ctx(), makeEntry(client, tenant, ref, "k1")); err != nil {
			t.Fatalf("PutEntry: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		ref := strongbox.SecretRef(fmt.Sprintf("bykey-k2-%d", i))
		if _, err := p.PutEntry(ctx(), makeEntry(client, tenant, ref, "k2")); err != nil {
			t.Fatalf("PutEntry: %v", err)
		}
	}

	entries, err := p.ListByKeyID(ctx(), client, tenant, "k1")
	if err != nil {
		t.Fatalf("ListByKeyID: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries for k1, got %d", len(entries))
	}
}

// 11. Client isolation.
func testClientIsolation(t *testing.T, p strongbox.Provider) {
	clientA := strongbox.ClientID("c-isoa")
	clientB := strongbox.ClientID("c-isob")
	tenant := strongbox.TenantID("t-clientiso")
	ref := strongbox.SecretRef("isolated-secret")

	if _, err := p.PutEntry(ctx(), makeEntry(clientA, tenant, ref, "k1")); err != nil {
		t.Fatalf("PutEntry: %v", err)
	}

	_, err := p.GetEntry(ctx(), clientB, tenant, ref)
	if !errors.Is(err, strongbox.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for different client, got: %v", err)
	}
}

// 12. Tenant isolation.
func testTenantIsolation(t *testing.T, p strongbox.Provider) {
	client := strongbox.ClientID("c-tenantiso")
	tenantX := strongbox.TenantID("t-isox")
	tenantY := strongbox.TenantID("t-isoy")
	ref := strongbox.SecretRef("isolated-secret2")

	if _, err := p.PutEntry(ctx(), makeEntry(client, tenantX, ref, "k1")); err != nil {
		t.Fatalf("PutEntry: %v", err)
	}

	_, err := p.GetEntry(ctx(), client, tenantY, ref)
	if !errors.Is(err, strongbox.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for different tenant, got: %v", err)
	}
}

// 13. Ping.
func testPing(t *testing.T, p strongbox.Provider) {
	if err := p.Ping(ctx()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// 14. DeleteEntries.
func testDeleteEntries(t *testing.T, p strongbox.Provider) {
	client := strongbox.ClientID("c-delentries")
	tenant := strongbox.TenantID("t-delentries")

	allRefs := make([]strongbox.SecretRef, 5)
	for i := 0; i < 5; i++ {
		allRefs[i] = strongbox.SecretRef(fmt.Sprintf("delent-%d", i))
		if _, err := p.PutEntry(ctx(), makeEntry(client, tenant, allRefs[i], "k1")); err != nil {
			t.Fatalf("PutEntry: %v", err)
		}
	}

	// Soft-delete first 3.
	toDelete := allRefs[:3]
	if err := p.DeleteEntries(ctx(), client, tenant, toDelete); err != nil {
		t.Fatalf("DeleteEntries: %v", err)
	}

	// Verify 3 are soft-deleted.
	for _, ref := range toDelete {
		got, err := p.GetEntry(ctx(), client, tenant, ref)
		if err != nil {
			t.Errorf("GetEntry(%s): %v", ref, err)
			continue
		}
		if got.DeletedAt == nil {
			t.Errorf("entry %s: expected DeletedAt set", ref)
		}
	}

	// Verify 2 remain in list.
	result, err := p.ListEntries(ctx(), client, tenant, strongbox.ListOptions{})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(result.Secrets) != 2 {
		t.Fatalf("expected 2 remaining in list, got %d", len(result.Secrets))
	}
}