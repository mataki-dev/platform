# strongbox

Multi-client, multi-tenant secret storage with pluggable backends and envelope
encryption. Import it, hand it a backend and a key configuration, and call
`Put` / `Get` / `Delete` / `List`. The library handles encryption, key
derivation, versioning, and audit internally.

## Install

    import "github.com/mataki-dev/platform/strongbox"

## What it is, what it isn't

**Is:**

- A Go library for encrypted secret storage, keyed by `(client_id, tenant_id, ref)`
- Envelope-encrypted at rest (AES-256-GCM, per-tenant derived key, random DEK per value)
- Multi-tenant: complete data isolation between clients and between tenants
- Versioned: every write produces a monotonically increasing version number
- Auditable: every operation emits a structured event via a pluggable logger

**Is not:**

- A secrets manager — no rotation scheduling, no dynamic secrets, no lease management
- A service — no daemon, no gRPC, no sidecar; it is a library linked into your binary
- A cross-product bridge — secrets are isolated per client+tenant; nothing is shared

## Tenancy Model

```
Client (application using Strongbox — "authpipe", "chimatic", "acme-saas")
  └── Tenant (end-customer of that application — "cust_01HXY...", "org_abc")
        └── Secret (named string — "conn:github:client_secret")
```

The primary key for any secret is `(client_id, tenant_id, ref)`. Encryption keys
are derived independently per `(client_id, tenant_id)` pair via HKDF-SHA256, so
ciphertext from one tenant cannot be decrypted under another's key.

## Public Types

```go
type ClientID string  // lowercase alphanumeric + hyphens, max 63 chars
type TenantID string  // opaque string, max 255 chars
type SecretRef string // "namespace:identifier", max 512 chars

type Scope struct {
    ClientID ClientID
    TenantID TenantID
}

type SecretValue struct {
    Ref       SecretRef
    Value     string
    Version   int64
    Metadata  map[string]string
    CreatedAt time.Time
    UpdatedAt time.Time
    ExpiresAt *time.Time
}

// SecretHeader is metadata without the value — returned by List.
type SecretHeader struct {
    Ref       SecretRef
    Version   int64
    Metadata  map[string]string
    CreatedAt time.Time
    UpdatedAt time.Time
    ExpiresAt *time.Time
}

type PutInput struct {
    Ref       SecretRef
    Value     string
    Metadata  map[string]string // optional
    ExpiresAt *time.Time        // optional
}

type PutResult struct {
    Ref             SecretRef
    Version         int64
    PreviousVersion *int64 // set on "updated"
    Action          string // "created", "updated", or "unchanged"
}

type ListOptions struct {
    Prefix    string
    Cursor    string
    Limit     int    // default 100, max 1000
    SortField string // "ref" (default), "created_at", "updated_at", "version"
    SortDir   string // "asc" (default) or "desc"
}

type ListResult struct {
    Secrets []SecretHeader
    Cursor  string
    HasMore bool
}
```

The library defines sentinel errors: `ErrNotFound`, `ErrConflict`,
`ErrPermissionDenied`, `ErrVersionConflict`, and others — see package
documentation.

## Usage

### Creating a Store

`NewStore` takes a `Provider` (the storage backend) and functional options.
A root key option is required; all other options are optional.

```go
provider, err := postgres.NewProvider(ctx, postgres.Config{
    DB:          db,
    AutoMigrate: true,
})

store, err := strongbox.NewStore(
    provider,
    strongbox.WithKeyFromEnv("STRONGBOX_KEY"),         // required; hex-encoded 32-byte key
    strongbox.WithAuditLogger(audit.NewSlog(slog.Default())),
)
```

Key options: `WithKeyFromEnv`, `WithKeyFromBytes`, `WithPreviousKeyFromEnv`,
`WithPreviousKeyFromBytes`. Additional options: `WithAuditLogger`,
`WithMaxValueSize` (default 65536), `WithMaxBatchSize` (default 500).

### Putting a secret

```go
scope := strongbox.Scope{ClientID: "my-app", TenantID: "cust_01HXY"}

result, err := store.Put(ctx, scope, strongbox.PutInput{
    Ref:      "conn:github:client_secret",
    Value:    "ghs_abc123",
    Metadata: map[string]string{"source": "github"},
})
// result.Action: "created" or "updated"; result.Version: new version number
```

### Getting a secret

```go
sv, err := store.Get(ctx, scope, "conn:github:client_secret")
if err != nil {
    // errors.Is(err, strongbox.ErrNotFound)
    // errors.Is(err, strongbox.ErrDeleted)
    // errors.Is(err, strongbox.ErrExpired)
    return err
}
fmt.Println(sv.Value, sv.Version)
```

### Listing and deleting

```go
// List returns headers (no plaintext values). Paginate via Cursor.
page, err := store.List(ctx, scope, strongbox.ListOptions{Prefix: "conn:", Limit: 50})
for page.HasMore {
    page, err = store.List(ctx, scope, strongbox.ListOptions{
        Prefix: "conn:", Limit: 50, Cursor: page.Cursor,
    })
}

// Soft-delete (Get returns ErrDeleted); hard-delete permanently removes the row.
err = store.Delete(ctx, scope, "conn:github:client_secret")
err = store.HardDelete(ctx, scope, "conn:github:client_secret")

// Tenant offboarding.
err = store.HardDeleteTenant(ctx, scope)
```

Batch variants: `BatchPut`, `DeleteMany`, `HardDeleteMany`, and `Sync`
(creates/updates a set and optionally removes others via `SyncFull` /
`SyncPartial` mode).

## Encryption

Strongbox uses AES-256-GCM envelope encryption with a three-level key hierarchy:

1. A 32-byte **root key** is supplied at `NewStore` time.
2. A **per-tenant derived key (DK)** is produced by
   `HKDF-SHA256(rootKey, salt=clientID, info=tenantID)` for each `(client_id, tenant_id)` pair.
3. A random 32-byte **data encryption key (DEK)** is generated per secret. The
   DEK is encrypted with the DK (the "envelope"). Both are stored together in the
   `ciphertext` column; the root key and DK are never persisted.

Envelope format (stored as opaque bytes in `StoredEntry.Ciphertext`):

```
[version(1)] [dek_nonce(12)] [encrypted_dek(48)] [value_nonce(12)] [encrypted_value(N)]
```

Supply `WithPreviousKeyFromEnv` alongside `WithKeyFromEnv` and call
`store.RotateKeys(ctx, scope)` to re-encrypt all secrets under the current key
without changing plaintext values.

## Backends

### Postgres

```go
import "github.com/mataki-dev/platform/strongbox/backend/postgres"

provider, err := postgres.NewProvider(ctx, postgres.Config{
    DB:          db,    // *sql.DB
    TableName:   "",    // default: "strongbox_secrets"
    AutoMigrate: true,  // run CREATE TABLE IF NOT EXISTS on startup
})
```

Schema (single table):

```sql
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
```

Indexes: `key_id` (key rotation queries), `ref text_pattern_ops` (prefix list),
`expires_at` (TTL scans). Soft-deletes set `deleted_at` and wipe `ciphertext`
to `\x00`. Hard-deletes issue `DELETE`.

### Writing a custom backend

Implement the `Provider` interface. Core methods:

```go
type Provider interface {
    PutEntry(ctx context.Context, entry StoredEntry) (version int64, err error)
    GetEntry(ctx context.Context, clientID ClientID, tenantID TenantID, ref SecretRef) (StoredEntry, error)
    ListEntries(ctx context.Context, clientID ClientID, tenantID TenantID, opts ListOptions) (ListResult, error)
    DeleteEntry(ctx context.Context, clientID ClientID, tenantID TenantID, ref SecretRef) error
    // plus rotation/batch/admin methods — see source
}
```

`StoredEntry.Ciphertext` is opaque bytes produced by the store's encryptor;
providers must store and return it verbatim.

Verify your implementation against the shared conformance suite:

```go
import "github.com/mataki-dev/platform/strongbox/conformance"

func TestMyProvider(t *testing.T) {
    conformance.RunProviderSuite(t, newMyProvider(t))
}
```

`RunProviderSuite` covers round-trip correctness, versioning, batch atomicity,
soft/hard delete, pagination, prefix listing, key-ID lookup, and isolation.

## Audit Logging

Every `Store` operation emits an `AuditEvent` to the configured `AuditLogger`.
The default is a noop logger that discards all events.

```go
type AuditLogger interface {
    Log(ctx context.Context, event AuditEvent)
}

type AuditEvent struct {
    Timestamp time.Time
    Operation string        // "put", "get", "list", "delete", etc.
    ClientID  ClientID
    TenantID  TenantID
    Ref       SecretRef     // set for single-ref operations
    Refs      []SecretRef   // set for batch operations
    Count     int
    Source    string
    Actor     string
    Error     string        // non-empty on failure
    Duration  time.Duration
}
```

Two implementations are provided: `audit.NewNoop()` (discard) and
`audit.NewSlog(logger *slog.Logger)` (structured log records; secret values
are never logged). Wire in at construction time via `WithAuditLogger`.

## Ingest HTTP Handlers

`strongbox/ingest` provides `net/http` handlers for external secrets managers
to push secrets into Strongbox over TLS.

```go
cfg := ingest.Config{
    Store:               store,
    ClientResolver:      ingest.StaticClient("my-app"),
    Authenticator:       myAuthFunc,
    Environments:        []string{"dev", "prod"},
    EnvironmentResolver: ingest.IdentityResolver(), // env slug == TenantID
    ProviderName:        "my-secrets-manager",
}
ingest.Register(mux, "/secrets", cfg)
```

Registered routes (all except discovery require TLS):

| Method | Path | Description |
|---|---|---|
| `GET` | `/{base}/` | Discovery — returns capabilities and endpoint map |
| `PUT` | `/{base}/{environment}/secrets` | Batch upsert (sync) |
| `GET` | `/{base}/{environment}/secrets/{key...}` | Get single secret |
| `DELETE` | `/{base}/{environment}/secrets/{key...}` | Delete single secret |
| `POST` | `/{base}/{environment}/secrets/search` | Search / list |
| `POST` | `/{base}/{environment}/secrets/delete` | Batch delete |
| `POST` | `/{base}/{environment}/secrets/webhook` | Webhook receiver |

`EnvironmentResolver` maps `{environment}` to a `TenantID`. Webhook requests
are verified with HMAC-SHA256 when `WebhookSigningKey` is set; expected header
format: `t=<unix>; s=sha256; v=<hex_sig>`.

## Migration

`strongbox/migrate` imports secrets from an existing encrypted column into
Strongbox. It is idempotent — rows that already exist are skipped.

```go
result, err := migrate.Run(ctx, db, store, scope,
    []migrate.ColumnMapping{
        {
            Table:        "connections",
            IDColumn:     "id",
            SecretColumn: "client_secret_enc",
            RefTemplate:  "conn:{{id}}:client_secret",
            DecryptFunc:  myOldDecrypt, // func([]byte) (string, error)
        },
    },
    migrate.Options{BatchSize: 100, NullOldColumn: true, DryRun: false},
)
// result.Migrated int; result.Skipped int; result.Errors []error
```

`migrate.Run` signature:

```go
func Run(
    ctx      context.Context,
    db       *sql.DB,
    store    *strongbox.Store,
    scope    strongbox.Scope,
    mappings []ColumnMapping,
    opts     Options,
) (Result, error)
```

## Testing

Run the full suite with the race detector:

    go test -race ./strongbox/...

The Postgres backend tests require a real database. Set `STRONGBOX_TEST_DSN` to
a `postgres://` connection string; tests are skipped when it is unset.

Custom backend authors should use the conformance suite (see
[Writing a custom backend](#writing-a-custom-backend)) rather than writing
provider tests from scratch.
