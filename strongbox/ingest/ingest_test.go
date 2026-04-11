// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package ingest

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mataki-dev/platform/strongbox"
)

// ---------------------------------------------------------------------------
// fakeProvider — in-memory Provider for testing
// ---------------------------------------------------------------------------

type entryKey struct {
	clientID strongbox.ClientID
	tenantID strongbox.TenantID
	ref      strongbox.SecretRef
}

type fakeProvider struct {
	entries map[entryKey]strongbox.StoredEntry
	nextVer int64
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{
		entries: make(map[entryKey]strongbox.StoredEntry),
		nextVer: 1,
	}
}

func (f *fakeProvider) key(c strongbox.ClientID, t strongbox.TenantID, r strongbox.SecretRef) entryKey {
	return entryKey{clientID: c, tenantID: t, ref: r}
}

func (f *fakeProvider) PutEntry(_ context.Context, e strongbox.StoredEntry) (int64, error) {
	k := f.key(e.ClientID, e.TenantID, e.Ref)
	v := f.nextVer
	f.nextVer++
	e.Version = v
	e.DeletedAt = nil
	f.entries[k] = e
	return v, nil
}

func (f *fakeProvider) PutEntries(ctx context.Context, entries []strongbox.StoredEntry) ([]int64, error) {
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

func (f *fakeProvider) GetEntry(_ context.Context, c strongbox.ClientID, t strongbox.TenantID, ref strongbox.SecretRef) (strongbox.StoredEntry, error) {
	k := f.key(c, t, ref)
	e, ok := f.entries[k]
	if !ok {
		return strongbox.StoredEntry{}, strongbox.ErrNotFound
	}
	return e, nil
}

func (f *fakeProvider) ListEntries(_ context.Context, c strongbox.ClientID, t strongbox.TenantID, opts strongbox.ListOptions) (strongbox.ListResult, error) {
	var headers []strongbox.SecretHeader
	for k, e := range f.entries {
		if k.clientID != c || k.tenantID != t {
			continue
		}
		if e.DeletedAt != nil {
			continue
		}
		if opts.Prefix != "" {
			ref := string(e.Ref)
			if len(ref) < len(opts.Prefix) || ref[:len(opts.Prefix)] != opts.Prefix {
				continue
			}
		}
		headers = append(headers, strongbox.SecretHeader{
			Ref:       e.Ref,
			Version:   e.Version,
			Metadata:  e.Metadata,
			CreatedAt: e.CreatedAt,
			UpdatedAt: e.UpdatedAt,
			ExpiresAt: e.ExpiresAt,
		})
	}
	return strongbox.ListResult{Secrets: headers}, nil
}

func (f *fakeProvider) DeleteEntry(_ context.Context, c strongbox.ClientID, t strongbox.TenantID, ref strongbox.SecretRef) error {
	k := f.key(c, t, ref)
	e, ok := f.entries[k]
	if !ok {
		return strongbox.ErrNotFound
	}
	now := time.Now()
	e.DeletedAt = &now
	f.entries[k] = e
	return nil
}

func (f *fakeProvider) DeleteEntries(ctx context.Context, c strongbox.ClientID, t strongbox.TenantID, refs []strongbox.SecretRef) error {
	for _, ref := range refs {
		if err := f.DeleteEntry(ctx, c, t, ref); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeProvider) HardDeleteEntry(_ context.Context, c strongbox.ClientID, t strongbox.TenantID, ref strongbox.SecretRef) error {
	k := f.key(c, t, ref)
	if _, ok := f.entries[k]; !ok {
		return strongbox.ErrNotFound
	}
	delete(f.entries, k)
	return nil
}

func (f *fakeProvider) HardDeleteTenant(_ context.Context, c strongbox.ClientID, t strongbox.TenantID) error {
	for k := range f.entries {
		if k.clientID == c && k.tenantID == t {
			delete(f.entries, k)
		}
	}
	return nil
}

func (f *fakeProvider) ListByKeyID(_ context.Context, c strongbox.ClientID, t strongbox.TenantID, keyID string) ([]strongbox.StoredEntry, error) {
	var result []strongbox.StoredEntry
	for k, e := range f.entries {
		if k.clientID == c && k.tenantID == t && e.KeyID == keyID && e.DeletedAt == nil {
			result = append(result, e)
		}
	}
	return result, nil
}

func (f *fakeProvider) Ping(_ context.Context) error { return nil }

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func makeTestKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func setupTestMux(t *testing.T) *http.ServeMux {
	t.Helper()

	fp := newFakeProvider()
	key := makeTestKey(t)
	store, err := strongbox.NewStore(fp, strongbox.WithKeyFromBytes(key))
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	Register(mux, "/ingest", Config{
		Store:          store,
		ClientResolver: StaticClient("test-client"),
		Authenticator: func(_ *http.Request) (AuthResult, error) {
			return AuthResult{TenantID: "test-tenant", Actor: "test"}, nil
		},
		Environments:        []string{"dev", "prod"},
		EnvironmentResolver: IdentityResolver(),
		ProviderName:        "test-provider",
		WebhookSigningKey:   "test-signing-key",
	})

	return mux
}

// tlsRequest creates a request that appears to be over TLS.
func tlsRequest(method, url string, body []byte) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, url, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	r.TLS = &tls.ConnectionState{}
	return r
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestDiscovery(t *testing.T) {
	mux := setupTestMux(t)

	r := httptest.NewRequest("GET", "/ingest/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}

	var resp DiscoveryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Provider != "test-provider" {
		t.Errorf("provider = %q; want test-provider", resp.Provider)
	}
	if !resp.Capabilities.Search {
		t.Error("capabilities.search should be true")
	}
	if resp.Endpoints.Sync == "" {
		t.Error("endpoints.sync should not be empty")
	}
}

func TestTLSEnforcement(t *testing.T) {
	mux := setupTestMux(t)

	// Request without TLS should get 403.
	r := httptest.NewRequest("GET", "/ingest/dev/secrets/my-key", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d; want 403", w.Code)
	}
}

func TestTLSEnforcementWithForwardedProto(t *testing.T) {
	mux := setupTestMux(t)

	// With X-Forwarded-Proto: https, should pass TLS check.
	r := httptest.NewRequest("GET", "/ingest/dev/secrets/nonexistent", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	// Should get past TLS check (404 for nonexistent secret is fine).
	if w.Code == http.StatusForbidden {
		t.Fatal("request with X-Forwarded-Proto: https should not be rejected for TLS")
	}
}

func TestUnknownEnvironment(t *testing.T) {
	mux := setupTestMux(t)

	r := tlsRequest("GET", "/ingest/staging/secrets/my-key", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", w.Code)
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Code != "unknown_environment" {
		t.Errorf("code = %q; want unknown_environment", resp.Code)
	}
}

func TestSyncSecrets(t *testing.T) {
	mux := setupTestMux(t)

	body, _ := json.Marshal(BatchUpsertRequest{
		Secrets: []IngestSecret{
			{Key: "db-pass", Value: "hunter2"},
			{Key: "api-key", Value: "sk-123"},
		},
		SyncMode: "partial",
	})

	r := tlsRequest("PUT", "/ingest/dev/secrets", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body: %s", w.Code, w.Body.String())
	}

	var resp BatchUpsertResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Synced) != 2 {
		t.Errorf("synced = %d; want 2", len(resp.Synced))
	}

	// Verify we can get the secret back.
	r2 := tlsRequest("GET", "/ingest/dev/secrets/db-pass", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("get status = %d; want 200; body: %s", w2.Code, w2.Body.String())
	}

	var getResp GetSecretResponse
	json.NewDecoder(w2.Body).Decode(&getResp)
	if getResp.Value != "hunter2" {
		t.Errorf("value = %q; want hunter2", getResp.Value)
	}
}

func TestSearchSecrets(t *testing.T) {
	mux := setupTestMux(t)

	// First, sync some secrets.
	syncBody, _ := json.Marshal(BatchUpsertRequest{
		Secrets: []IngestSecret{
			{Key: "app.db-pass", Value: "v1"},
			{Key: "app.api-key", Value: "v2"},
			{Key: "other.thing", Value: "v3"},
		},
	})
	r := tlsRequest("PUT", "/ingest/dev/secrets", syncBody)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("sync status = %d; want 200", w.Code)
	}

	// Now search with a filter.
	searchBody, _ := json.Marshal(map[string]any{
		"filter": map[string]any{
			"ref": map[string]any{
				"contains": "app.",
			},
		},
	})

	r2 := tlsRequest("POST", "/ingest/dev/secrets/search", searchBody)
	r2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, r2)

	if w2.Code != http.StatusOK {
		t.Fatalf("search status = %d; want 200; body: %s", w2.Code, w2.Body.String())
	}

	var resp SearchSecretsResponse
	json.NewDecoder(w2.Body).Decode(&resp)
	if len(resp.Secrets) != 2 {
		t.Errorf("secrets = %d; want 2", len(resp.Secrets))
	}
}

func TestGetSecret(t *testing.T) {
	mux := setupTestMux(t)

	// Sync a secret first.
	syncBody, _ := json.Marshal(BatchUpsertRequest{
		Secrets: []IngestSecret{
			{Key: "my-secret", Value: "secret-value", Metadata: map[string]string{"env": "dev"}},
		},
	})
	r := tlsRequest("PUT", "/ingest/dev/secrets", syncBody)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("sync status = %d", w.Code)
	}

	// Get it back.
	r2 := tlsRequest("GET", "/ingest/dev/secrets/my-secret", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, r2)

	if w2.Code != http.StatusOK {
		t.Fatalf("get status = %d; want 200; body: %s", w2.Code, w2.Body.String())
	}

	var resp GetSecretResponse
	json.NewDecoder(w2.Body).Decode(&resp)
	if resp.Key != "my-secret" {
		t.Errorf("key = %q; want my-secret", resp.Key)
	}
	if resp.Value != "secret-value" {
		t.Errorf("value = %q; want secret-value", resp.Value)
	}
}

func TestGetSecretNotFound(t *testing.T) {
	mux := setupTestMux(t)

	r := tlsRequest("GET", "/ingest/dev/secrets/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", w.Code)
	}
}

func TestDeleteSecret(t *testing.T) {
	mux := setupTestMux(t)

	// Sync a secret first.
	syncBody, _ := json.Marshal(BatchUpsertRequest{
		Secrets: []IngestSecret{{Key: "to-delete", Value: "val"}},
	})
	r := tlsRequest("PUT", "/ingest/dev/secrets", syncBody)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("sync status = %d", w.Code)
	}

	// Delete it.
	r2 := tlsRequest("DELETE", "/ingest/dev/secrets/to-delete", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, r2)

	if w2.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d; want 204; body: %s", w2.Code, w2.Body.String())
	}

	// Get should return 410 (deleted).
	r3 := tlsRequest("GET", "/ingest/dev/secrets/to-delete", nil)
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, r3)

	if w3.Code != http.StatusGone {
		t.Fatalf("get after delete status = %d; want 410", w3.Code)
	}
}

func TestWebhookSignatureVerification(t *testing.T) {
	mux := setupTestMux(t)
	signingKey := "test-signing-key"

	payload := `{"event":"secret.updated","environment":"dev","timestamp":"2024-01-01T00:00:00Z"}`
	ts := fmt.Sprintf("%d", time.Now().Unix())

	// Compute valid signature.
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))

	header := fmt.Sprintf("t=%s;s=sha256;v=%s", ts, sig)

	r := tlsRequest("POST", "/ingest/dev/secrets/webhook", []byte(payload))
	r.Header.Set("X-Webhook-Signature", header)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d; want 204; body: %s", w.Code, w.Body.String())
	}
}

func TestWebhookBadSignature(t *testing.T) {
	mux := setupTestMux(t)

	payload := `{"event":"test"}`
	r := tlsRequest("POST", "/ingest/dev/secrets/webhook", []byte(payload))
	r.Header.Set("X-Webhook-Signature", "t=0;s=sha256;v=badbeef")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", w.Code)
	}
}

func TestWebhookOnCallback(t *testing.T) {
	fp := newFakeProvider()
	key := makeTestKey(t)
	store, err := strongbox.NewStore(fp, strongbox.WithKeyFromBytes(key))
	if err != nil {
		t.Fatal(err)
	}

	called := make(chan WebhookPayload, 1)

	mux := http.NewServeMux()
	Register(mux, "/ingest", Config{
		Store:          store,
		ClientResolver: StaticClient("test-client"),
		Authenticator: func(_ *http.Request) (AuthResult, error) {
			return AuthResult{TenantID: "test-tenant", Actor: "test"}, nil
		},
		Environments:        []string{"dev"},
		EnvironmentResolver: IdentityResolver(),
		ProviderName:        "test",
		WebhookSigningKey:   "key",
		OnWebhook: func(_ context.Context, payload WebhookPayload) {
			called <- payload
		},
	})

	payload := `{"event":"secret.updated","environment":"dev"}`
	ts := fmt.Sprintf("%d", time.Now().Unix())
	mac := hmac.New(sha256.New, []byte("key"))
	mac.Write([]byte(ts + "." + payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	header := fmt.Sprintf("t=%s;s=sha256;v=%s", ts, sig)

	r := tlsRequest("POST", "/ingest/dev/secrets/webhook", []byte(payload))
	r.Header.Set("X-Webhook-Signature", header)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d; want 204", w.Code)
	}

	select {
	case p := <-called:
		if p.Event != "secret.updated" {
			t.Errorf("event = %q; want secret.updated", p.Event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnWebhook was not called within timeout")
	}
}

func TestStaticClient(t *testing.T) {
	resolver := StaticClient("my-client")
	r := httptest.NewRequest("GET", "/", nil)
	id, err := resolver(r)
	if err != nil {
		t.Fatal(err)
	}
	if id != "my-client" {
		t.Errorf("id = %q; want my-client", id)
	}
}

func TestIdentityResolver(t *testing.T) {
	resolver := IdentityResolver()
	tid, err := resolver("client", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if tid != "dev" {
		t.Errorf("tid = %q; want dev", tid)
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	key := "my-secret-key"
	body := []byte("test-body")
	ts := fmt.Sprintf("%d", time.Now().Unix())

	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(ts + "." + string(body)))
	sig := hex.EncodeToString(mac.Sum(nil))

	header := fmt.Sprintf("t=%s;s=sha256;v=%s", ts, sig)

	if err := verifyWebhookSignature(body, header, key); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}

	// Bad signature should fail.
	if err := verifyWebhookSignature(body, "t=0;s=sha256;v=bad", key); err == nil {
		t.Fatal("bad signature should be rejected")
	}

	// Missing header should fail.
	if err := verifyWebhookSignature(body, "", key); err == nil {
		t.Fatal("empty header should be rejected")
	}
}

func TestRequireTLS(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireTLS(inner)

	// No TLS.
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("no TLS: status = %d; want 403", w.Code)
	}

	// With TLS.
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.TLS = &tls.ConnectionState{}
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Errorf("with TLS: status = %d; want 200", w2.Code)
	}

	// With X-Forwarded-Proto.
	r3 := httptest.NewRequest("GET", "/", nil)
	r3.Header.Set("X-Forwarded-Proto", "https")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, r3)
	if w3.Code != http.StatusOK {
		t.Errorf("with X-Forwarded-Proto: status = %d; want 200", w3.Code)
	}
}

func TestSearchSecretsEmptyResult(t *testing.T) {
	mux := setupTestMux(t)

	searchBody, _ := json.Marshal(map[string]any{})
	r := tlsRequest("POST", "/ingest/dev/secrets/search", searchBody)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body: %s", w.Code, w.Body.String())
	}

	var resp SearchSecretsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Secrets == nil {
		t.Error("secrets should be empty array, not nil")
	}
	if len(resp.Secrets) != 0 {
		t.Errorf("secrets = %d; want 0", len(resp.Secrets))
	}
}

func TestBatchDelete(t *testing.T) {
	mux := setupTestMux(t)

	// Sync secrets first.
	syncBody, _ := json.Marshal(BatchUpsertRequest{
		Secrets: []IngestSecret{
			{Key: "del1", Value: "v1"},
			{Key: "del2", Value: "v2"},
		},
	})
	r := tlsRequest("PUT", "/ingest/dev/secrets", syncBody)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("sync status = %d", w.Code)
	}

	// Batch delete.
	delBody, _ := json.Marshal(BatchDeleteRequest{Keys: []string{"del1", "del2"}})
	r2 := tlsRequest("POST", "/ingest/dev/secrets/delete", delBody)
	r2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, r2)

	if w2.Code != http.StatusOK {
		t.Fatalf("batch delete status = %d; want 200; body: %s", w2.Code, w2.Body.String())
	}

	var resp BatchDeleteResponse
	json.NewDecoder(w2.Body).Decode(&resp)
	if len(resp.Deleted) != 2 {
		t.Errorf("deleted = %d; want 2", len(resp.Deleted))
	}
}