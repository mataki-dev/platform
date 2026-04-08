# Mataki Platform Library — Design Specification

**Module:** `github.com/mataki-dev/platform`  
**Status:** Approved  
**Date:** 2026-04-08

---

## 1. Overview

The platform library provides shared infrastructure for all Mataki products: a search system for `POST /{resource}/search` endpoints, and a semantic error system for consistent error handling. Products depend on this library and supply only domain-specific configuration.

### 1.1 Packages

```
platform/
├── search/        # Search request handling, validation, SQL generation, pagination
│   ├── schema.go      # ResourceSchema, FieldDef, FieldType, FullTextConfig
│   ├── types.go       # SearchRequest, FilterExpr, SortDirective, SearchResponse
│   ├── validate.go    # Validate() → ValidatedSearch, []ValidationError
│   ├── validated.go   # ValidatedSearch, ValidatedFilter (output types)
│   ├── builder.go     # BuildSQL() → SQLFragment
│   ├── cursor.go      # EncodeCursor, DecodeCursor, cursor types
│   ├── response.go    # BuildResponse[T]() generic helper
│   ├── huma.go        # RegisterSearchOperation(), OperationOption
│   └── errors.go      # Search-specific error codes/constructors
├── errors/        # Semantic error types + infrastructure mappers
│   ├── errors.go      # Error types, Error interface, constructors
│   ├── pg.go          # pgx/pgconn error mappers
│   ├── http.go        # HTTP status code mappers
│   └── huma.go        # Huma error handler (semantic → HTTP status + envelope)
```

`search` depends on `errors` for validation/HTTP error responses. Both packages have no other internal dependencies.

### 1.2 Design Constraints

- All Mataki APIs use Go/Huma with PostgreSQL (pgx) and sqlc.
- sqlc owns all static queries. The search package owns only the dynamic WHERE/ORDER BY/LIMIT fragment.
- All Mataki APIs use date-based versioning via the `API-Version` header (Stripe-style).
- The library is opinionated about shape, agnostic about domain.

---

## 2. Semantic Errors (`errors` package)

### 2.1 Core Interface

```go
type Error interface {
    error
    Code() string      // machine-readable: "not_found", "conflict", etc.
    HTTPStatus() int   // canonical HTTP status
    Message() string   // human-readable
}
```

### 2.2 Error Types

| Type           | Code             | HTTP | When                               |
|----------------|------------------|------|------------------------------------|
| `NotFound`     | `not_found`      | 404  | Resource doesn't exist             |
| `Conflict`     | `conflict`       | 409  | Unique constraint / duplicate      |
| `Forbidden`    | `forbidden`      | 403  | Authorization failure              |
| `InvalidInput` | `invalid_input`  | 422  | Validation / bad request data      |
| `Internal`     | `internal`       | 500  | Unexpected / unrecognized errors   |

### 2.3 Constructors

```go
func NewNotFound(msg string, opts ...Option) Error
func NewConflict(msg string, opts ...Option) Error
func NewForbidden(msg string, opts ...Option) Error
func NewInvalidInput(msg string, opts ...Option) Error
func NewInternal(msg string, opts ...Option) Error

// Options for attaching context
func WithDetail(key string, val any) Option
func WithCause(err error) Option  // wraps original for logging/debugging
```

`WithCause` preserves the original error for `errors.Is`/`errors.As` unwrapping and structured logging, but the cause is never exposed in API responses.

### 2.4 Infrastructure Mappers

**PostgreSQL mapper:**

```go
func MapPgError(err error) Error
```

Recognized mappings:
- `pgx.ErrNoRows` → `NotFound`
- unique_violation (23505) → `Conflict`
- foreign_key_violation (23503) → `Conflict` (with detail: constraint name)
- check_violation (23514) → `InvalidInput`
- not_null_violation (23502) → `InvalidInput`
- Anything else → `nil` (forces caller to decide)

Returning `nil` for unrecognized errors is intentional — products must consciously decide whether an unknown PG error is `Internal` or needs specific handling.

**HTTP mapper:**

```go
func MapHTTPStatus(status int, msg string) Error
```

- 404 → `NotFound`
- 403 → `Forbidden`
- 409 → `Conflict`
- 422 → `InvalidInput`
- 5xx → `Internal`
- Other 4xx → `InvalidInput`

### 2.5 Huma Integration

```go
func NewHumaErrorHandler() func(ctx context.Context, err error) huma.StatusError
```

Checks if the error implements `Error`. If so, renders with the correct HTTP status and standard Mataki error envelope:

```json
{
  "type": "<code>",
  "message": "<message>",
  "errors": [...]
}
```

Unrecognized errors become 500 `Internal`.

---

## 3. Search Package

The search package implements the full spec from `mataki-platform-search-spec.md` with the additions described below. The original spec is incorporated by reference — this section covers only the additions and decisions made during design.

### 3.1 Request Schema

All search endpoints accept a JSON body:

```json
{
  "query": "full-text search string",
  "filter": { "field": { "operator": "value" } },
  "sort": [{ "field": "name", "direction": "desc" }],
  "cursor": "opaque-token",
  "offset": 0,
  "limit": 25,
  "fields": ["id", "name"]
}
```

All top-level keys are optional. An empty body `{}` returns the default page with default sort.

`cursor` and `offset` are mutually exclusive — providing both is a validation error. `offset` is a zero-based integer, clamped to a reasonable maximum (default 10,000) to prevent deep-pagination abuse. Products can override this ceiling in `ResourceSchema`.

### 3.2 Full-Text Search

Products opt into full-text search by providing a `FullTextConfig` in their `ResourceSchema`:

```go
type FullTextConfig struct {
    // Column is the pre-computed tsvector column name (e.g., "search_vector").
    Column string

    // Language is the text search config for tsquery parsing.
    // Defaults to "english".
    Language string
}

type ResourceSchema struct {
    // ... all existing fields from the spec ...

    // FullText configures full-text search via the "query" field.
    // When nil, the "query" field is rejected in requests.
    FullText *FullTextConfig
}
```

**Design decisions:**

- Products own their tsvector strategy entirely (triggers, generated columns, weighting). The library only needs to know the column name and language config.
- The `query` input is parsed via `plainto_tsquery(language, input)` — no tsquery syntax is exposed to API consumers.
- When `query` is present and `FullText` is nil → validation error (`"query"`, `"unsupported_field"`).

**SQL generation when `query` is present:**

```sql
-- Added to WHERE clause:
AND t.search_vector @@ plainto_tsquery('english', $N)

-- Default ORDER BY (no explicit sort):
ORDER BY ts_rank(t.search_vector, plainto_tsquery('english', $N+1)) DESC, t.id ASC
```

**Sort interaction:**

- When `query` is present and no explicit `sort` → order by `ts_rank` descending (relevance), with `id` tiebreaker.
- When `query` is present and explicit `sort` is provided → relevance ranking is dropped entirely, user's sort takes over. The `@@ tsquery` filter still applies to restrict results; only the ordering changes.

**Cursor interaction with relevance sort:**

When sorting by relevance (query present, no explicit sort), cursor pagination is disabled. `ts_rank` is a computed value that shifts as data changes, making keyset cursors unreliable. In this mode:

- `has_more` and `limit` are still returned.
- `next_cursor` is omitted.
- Clients must use `offset` for pagination.
- If a `cursor` is provided with no explicit sort while `query` is present → validation error.

### 3.3 Filter Operators

Per the original spec:

| Operator   | JSON Key     | SQL                              | Applicable Types              |
|------------|--------------|----------------------------------|-------------------------------|
| `eq`       | `eq`         | `col = $N`                       | All                           |
| `neq`      | `neq`        | `col != $N`                      | All                           |
| `gt`       | `gt`         | `col > $N`                       | Numeric, Timestamp            |
| `gte`      | `gte`        | `col >= $N`                      | Numeric, Timestamp            |
| `lt`       | `lt`         | `col < $N`                       | Numeric, Timestamp            |
| `lte`      | `lte`        | `col <= $N`                      | Numeric, Timestamp            |
| `in`       | `in`         | `col = ANY($N)`                  | String, Numeric, Enum         |
| `nin`      | `nin`        | `col != ALL($N)`                 | String, Numeric, Enum         |
| `contains` | `contains`   | `col ILIKE $N`                   | String                        |
| `is_null`  | `is_null`    | `col IS NULL` / `IS NOT NULL`    | All (nullable only)           |

Multiple operators on the same field are AND-joined. Multiple fields are AND-joined. No OR logic in v1.

### 3.4 Validation

`search.Validate(req SearchRequest, schema ResourceSchema) (ValidatedSearch, []ValidationError)`

Exhaustive validation — all errors collected before returning. Parse-don't-validate: once validated, the search is known-good. The SQL builder only accepts `ValidatedSearch`.

Error codes: `unknown_field`, `invalid_operator`, `invalid_value`, `invalid_type`, `exceeds_max`, `invalid_cursor`, `unsortable_field`, `duplicate_sort_field`, `not_nullable`, `unsupported_field`.

### 3.5 Cursor Pagination

Keyset pagination (seek method). Cursor encodes sort column values + primary key of last row, base64url-encoded JSON with version field.

For mixed-direction sorts, the builder generates the expanded boolean form rather than row-value comparison.

`has_more` detection: fetch `limit + 1` rows, trim the extra.

### 3.6 SQL Builder

```go
func BuildSQL(vs ValidatedSearch, argOffset int) SQLFragment
```

`SQLFragment` provides `Where`, `CursorWhere`, `OrderBy`, `Limit`, `Args`, and `ArgCount`. Products compose these into their full query with tenant scoping, soft-delete filters, and joins.

SQL injection prevention: column names from schema allowlist, all values as positional parameters, `contains` escapes `%`, `_`, `\`.

### 3.7 Response Helper

```go
func BuildResponse[T any](rows []T, req ValidatedSearch, extractFn func(T) map[string]any) (*SearchResponse[T], error)
```

Handles limit+1 trim, `has_more`, and cursor encoding. `extractFn` maps the product's row struct to the cursor value map.

### 3.8 Huma Registration

```go
func RegisterSearchOperation[T any](
    api huma.API,
    path string,
    schema ResourceSchema,
    handler func(ctx context.Context, req *SearchRequest) (*SearchResponse[T], error),
    opts ...OperationOption,
)
```

Provides: operation registration, OpenAPI schema generation, validation before handler invocation (422 on failure via `errors.NewInvalidInput`), standard response types.

### 3.9 Product Integration Example

```go
var taskSchema = search.ResourceSchema{
    Fields: map[string]search.FieldDef{
        "status":     {Type: search.Enum, Sortable: true, AllowedValues: []string{"open", "done"}},
        "created_at": {Type: search.Timestamp, Sortable: true},
        "title":      {Type: search.String, Operators: []string{"contains"}},
    },
    DefaultSort:  []search.SortDirective{{Field: "created_at", Dir: search.Desc}},
    DefaultLimit: 25,
    MaxLimit:     100,
    TableAlias:   "t",
    FullText: &search.FullTextConfig{
        Column:   "search_vector",
        Language: "english",
    },
}

func (r *TaskRepo) Search(ctx context.Context, tenantID string, req search.ValidatedSearch) (*search.SearchResponse[Task], error) {
    frag := search.BuildSQL(req, 2) // $1 = tenant_id

    query := fmt.Sprintf(`
        SELECT t.id, t.title, t.status, t.created_at, t.updated_at
        FROM tasks t
        WHERE t.tenant_id = $1
          AND t.deleted_at IS NULL
          %s %s
        ORDER BY %s
        LIMIT %d`,
        frag.Where, frag.CursorWhere, frag.OrderBy, frag.Limit)

    args := append([]any{tenantID}, frag.Args...)
    rows, err := r.pool.Query(ctx, query, args...)
    if err != nil {
        if semErr := errors.MapPgError(err); semErr != nil {
            return nil, semErr
        }
        return nil, errors.NewInternal("search tasks", errors.WithCause(err))
    }
    defer rows.Close()

    tasks, err := pgx.CollectRows(rows, pgx.RowToStructByName[Task])
    if err != nil {
        return nil, errors.NewInternal("scan tasks", errors.WithCause(err))
    }

    return search.BuildResponse(tasks, req, func(t Task) map[string]any {
        return map[string]any{"created_at": t.CreatedAt, "id": t.ID}
    })
}
```

---

## 4. Error Responses

All Mataki APIs use the same error envelope, rendered by `errors.NewHumaErrorHandler()`:

**Validation error (422):**

```json
{
  "type": "invalid_input",
  "message": "The search request contains invalid parameters.",
  "errors": [
    {"field": "filter.priority.eq", "code": "invalid_type", "message": "Expected numeric value, got string."},
    {"field": "sort[0].field", "code": "unsortable_field", "message": "Field 'title' is not sortable."}
  ]
}
```

The `errors` array is present only for `InvalidInput` responses that carry multiple field-level issues. Other error types (`NotFound`, `Conflict`, `Forbidden`, `Internal`) omit it:

**Not found (404):**

```json
{
  "type": "not_found",
  "message": "Task not found."
}
```

**Conflict (409):**

```json
{
  "type": "conflict",
  "message": "A task with this external ID already exists."
}
```

---

## 5. Testing Strategy

### 5.1 `errors` Package

- Each error type constructor produces correct code, HTTP status, and message.
- `WithCause` preserves the original error for `errors.Is`/`errors.As`.
- `MapPgError`: test each recognized PG error code maps correctly; unrecognized returns nil.
- `MapHTTPStatus`: test each status code range.
- Huma error handler: semantic errors render correct status + envelope; non-semantic errors become 500.

### 5.2 `search` Package

- **Validation:** exhaustive matrix of valid/invalid filter+operator+type combinations. Every error code path tested. Multi-error accumulation tested. Full-text `query` rejected when `FullText` is nil. Cursor rejected when relevance-sorting.
- **SQL builder:** single filters, compound filters, all operators, empty filters, cursor clauses, mixed-direction sorts, full-text WHERE and ORDER BY generation. Parameter indices verified.
- **Cursor round-trip:** encode then decode, sort signature mismatch, malformed input.
- **Contains escaping:** `%`, `_`, `\` produce correct ILIKE patterns.
- **Full-text:** `query` generates correct `@@` and `ts_rank` SQL. Explicit sort overrides relevance. Offset pagination when relevance-sorting.

### 5.3 Property-Based Tests

- Any `ValidatedSearch` → syntactically valid SQL.
- Any cursor round-trips through encode/decode.

### 5.4 Integration Tests

In consuming products against real PostgreSQL. The platform library itself has no database dependency.

---

## 6. Performance Notes

- Products own indexes. Recommended: composite index on (tenant_id, sort_columns, id) with partial WHERE for soft-delete.
- `contains` (ILIKE) needs `pg_trgm` GIN index on high-cardinality text fields.
- Full-text search needs a GIN index on the tsvector column.
- `IN` set size defaults to max 100, configurable per-field.

---

## 7. Future Considerations

Out of scope for v1, accommodated by the design:

- **OR logic** — `filter_any` or explicit `$or` operator.
- **Aggregation/count** — `POST /{resource}/search/count` reusing validation + WHERE generation.
- **Saved searches** — persisting `SearchRequest` bodies.
- **Additional error types** — e.g., `RateLimited` (429), `Unavailable` (503).
