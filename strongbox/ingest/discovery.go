// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

// Package ingest provides net/http handlers that expose an HTTP API for
// external secrets managers to push secrets into Strongbox.
package ingest

import (
	"context"
	"net/http"
	"time"

	"github.com/mataki-dev/platform/strongbox"
)

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

// Config configures the ingest HTTP handlers.
type Config struct {
	Store               *strongbox.Store
	ClientResolver      ClientResolver
	Authenticator       Authenticator
	Environments        []string
	EnvironmentResolver EnvironmentResolver
	ProviderName        string
	WebhookSigningKey   string
	KeyPattern          string // Default: ^[a-zA-Z0-9][a-zA-Z0-9:._-]{0,511}$
	MaxValueLength      int    // Default: 65536
	OnWebhook           func(ctx context.Context, payload WebhookPayload)
}

// ClientResolver extracts a ClientID from an incoming request.
type ClientResolver func(r *http.Request) (strongbox.ClientID, error)

// StaticClient returns a ClientResolver that always returns the given id.
func StaticClient(id strongbox.ClientID) ClientResolver {
	return func(_ *http.Request) (strongbox.ClientID, error) {
		return id, nil
	}
}

// Authenticator validates an incoming request and returns an AuthResult.
type Authenticator func(r *http.Request) (AuthResult, error)

// AuthResult is the outcome of authenticating an ingest request.
type AuthResult struct {
	TenantID strongbox.TenantID
	Actor    string
}

// EnvironmentResolver maps a client + environment slug to a TenantID.
type EnvironmentResolver func(clientID strongbox.ClientID, env string) (strongbox.TenantID, error)

// IdentityResolver returns an EnvironmentResolver that uses the environment
// slug directly as the TenantID.
func IdentityResolver() EnvironmentResolver {
	return func(_ strongbox.ClientID, env string) (strongbox.TenantID, error) {
		return strongbox.TenantID(env), nil
	}
}

// ---------------------------------------------------------------------------
// Discovery response
// ---------------------------------------------------------------------------

// DiscoveryResponse is returned by GET /.
type DiscoveryResponse struct {
	Provider     string       `json:"provider"`
	Capabilities Capabilities `json:"capabilities"`
	Endpoints    Endpoints    `json:"endpoints"`
}

// Capabilities describes what this ingest endpoint supports.
type Capabilities struct {
	Search bool `json:"search"`
}

// Endpoints lists the available API paths.
type Endpoints struct {
	Sync         string `json:"sync"`
	Search       string `json:"search"`
	Get          string `json:"get"`
	Delete       string `json:"delete"`
	BatchDelete  string `json:"batch_delete"`
	Webhook      string `json:"webhook"`
}

// ---------------------------------------------------------------------------
// Sync (batch upsert) types
// ---------------------------------------------------------------------------

// BatchUpsertRequest is the body of PUT /{environment}/secrets.
type BatchUpsertRequest struct {
	Secrets  []IngestSecret `json:"secrets"`
	SyncMode string         `json:"sync_mode,omitempty"` // "partial" (default) or "full"
}

// IngestSecret is a single secret in a batch upsert request.
type IngestSecret struct {
	Key       string            `json:"key"`
	Value     string            `json:"value"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	ExpiresAt *time.Time        `json:"expires_at,omitempty"`
}

// BatchUpsertResponse is the response from PUT /{environment}/secrets.
type BatchUpsertResponse struct {
	Synced  []IngestPutResult  `json:"synced"`
	Deleted []IngestHeader     `json:"deleted,omitempty"`
	Errors  []IngestError      `json:"errors,omitempty"`
}

// IngestPutResult is the outcome of a single secret in a sync.
type IngestPutResult struct {
	Key             string `json:"key"`
	Version         int64  `json:"version"`
	PreviousVersion *int64 `json:"previous_version,omitempty"`
	Action          string `json:"action"`
}

// ---------------------------------------------------------------------------
// Get response
// ---------------------------------------------------------------------------

// GetSecretResponse is the response from GET /{environment}/secrets/{key}.
type GetSecretResponse struct {
	Key       string            `json:"key"`
	Value     string            `json:"value"`
	Version   int64             `json:"version"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	ExpiresAt *time.Time        `json:"expires_at,omitempty"`
}

// IngestHeader is secret metadata without the value.
type IngestHeader struct {
	Key       string            `json:"key"`
	Version   int64             `json:"version"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	ExpiresAt *time.Time        `json:"expires_at,omitempty"`
}

// ---------------------------------------------------------------------------
// Delete types
// ---------------------------------------------------------------------------

// BatchDeleteRequest is the body of POST /{environment}/secrets/delete.
type BatchDeleteRequest struct {
	Keys []string `json:"keys"`
}

// BatchDeleteResponse is the response from POST /{environment}/secrets/delete.
type BatchDeleteResponse struct {
	Deleted []IngestDeleteResult `json:"deleted"`
}

// IngestDeleteResult is the outcome of deleting a single secret.
type IngestDeleteResult struct {
	Key     string `json:"key"`
	Deleted bool   `json:"deleted"`
}

// ---------------------------------------------------------------------------
// Webhook
// ---------------------------------------------------------------------------

// WebhookPayload is the body of a POST /{environment}/secrets/webhook.
type WebhookPayload struct {
	Event       string          `json:"event"`
	Environment string          `json:"environment"`
	Keys        []string        `json:"keys,omitempty"`
	Timestamp   time.Time       `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// IngestError pairs a key with a coded error.
type IngestError struct {
	Key     string `json:"key"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Error   string        `json:"error"`
	Code    string        `json:"code"`
	Details []IngestError `json:"details,omitempty"`
}