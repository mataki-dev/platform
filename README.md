# Mataki Platform

Shared Go library for all Mataki products. Provides two packages:

- **`errors`** -- Semantic error types with infrastructure mappers and Huma integration
- **`search`** -- Search endpoint infrastructure: validation, SQL generation, cursor pagination, full-text search

## Install

```bash
go get github.com/mataki-dev/platform
```

Requires Go 1.22+ (uses generics and `slices`).

## `errors` Package

Consistent error handling across all Mataki APIs. Five semantic error types map to HTTP status codes and render through a standard JSON envelope.

### Error Types

| Constructor | Code | HTTP | Use |
|---|---|---|---|
| `NewNotFound` | `not_found` | 404 | Resource doesn't exist |
| `NewConflict` | `conflict` | 409 | Unique constraint / duplicate |
| `NewForbidden` | `forbidden` | 403 | Authorization failure |
| `NewInvalidInput` | `invalid_input` | 422 | Validation / bad input |
| `NewInternal` | `internal` | 500 | Unexpected errors |

### Options

```go
// Preserve the original error for errors.Is / errors.As
err := errors.NewNotFound("user not found", errors.WithCause(originalErr))

// Attach structured context (for logging, never exposed in API responses)
err := errors.NewConflict("duplicate email",
    errors.WithDetail("constraint", "users_email_key"),
)
```

### Infrastructure Mappers

```go
// PostgreSQL -- maps known pgx/pgconn errors to semantic errors.
// Returns nil for unrecognized errors (caller decides).
if semErr := errors.MapPgError(err); semErr != nil {
    return semErr
}

// Recognized: pgx.ErrNoRows -> NotFound
//             23505 (unique_violation) -> Conflict
//             23503 (foreign_key_violation) -> Conflict
//             23514 (check_violation) -> InvalidInput
//             23502 (not_null_violation) -> InvalidInput

// HTTP -- maps upstream service responses to semantic errors.
err := errors.MapHTTPStatus(resp.StatusCode, "upstream failed")
```

### Huma Integration

```go
// Convert any error to a huma.StatusError with the standard envelope.
handler := errors.NewHumaErrorHandler()
humaErr := handler(err) // *SemanticError -> correct status; other -> 500

// Or convert directly when you have a *SemanticError:
return nil, errors.ToHumaError(semErr)
```

**Error envelope:**

```json
{
  "type": "not_found",
  "message": "User not found."
}
```

For validation errors, an `errors` array provides field-level detail:

```json
{
  "type": "invalid_input",
  "message": "The request contains invalid parameters.",
  "errors": [
    {"field": "email", "code": "invalid_value", "message": "Not a valid email."}
  ]
}
```

## `search` Package

Every Mataki product exposes searchable resources via `POST /{resource}/search`. This package handles the shared machinery: request parsing, validation, SQL fragment generation, cursor pagination, full-text search, and Huma operation registration.

Products supply a declarative `ResourceSchema` and execute the resulting query. Everything else is handled here.

### Request Schema

All search endpoints accept this JSON body (all fields optional):

```json
{
  "query": "full-text search string",
  "filter": {
    "status": { "eq": "active" },
    "created_at": { "gte": "2026-01-01T00:00:00Z" }
  },
  "sort": [{ "field": "created_at", "direction": "desc" }],
  "cursor": "opaque-base64-token",
  "offset": 0,
  "limit": 25,
  "fields": ["id", "name", "status"]
}
```

### Define a Resource Schema

```go
var taskSearchSchema = search.ResourceSchema{
    Fields: map[string]search.FieldDef{
        "status":     {Type: search.Enum, Sortable: true, AllowedValues: []string{"open", "in_progress", "done"}},
        "priority":   {Type: search.Numeric, Sortable: true},
        "title":      {Type: search.String, Operators: []string{"contains"}},
        "created_at": {Type: search.Timestamp, Sortable: true},
        "assignee_id":{Type: search.String, Nullable: true},
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
```

### Field Types and Operators

| Type | Operators |
|---|---|
| `String` | `eq`, `neq`, `in`, `nin`, `contains`, `is_null` |
| `Numeric` | `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `in`, `nin`, `is_null` |
| `Timestamp` | `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `is_null` |
| `Bool` | `eq`, `neq`, `is_null` |
| `Enum` | `eq`, `neq`, `in`, `nin`, `is_null` |

Restrict operators per-field via `FieldDef.Operators`. If empty, all type-compatible operators are allowed.

### Validation

```go
validated, errs := search.Validate(req, schema)
if len(errs) > 0 {
    // errs is []ValidationError with Field, Code, Message
    // Validation is exhaustive -- all errors collected, not fail-fast.
}
```

The SQL builder only accepts `ValidatedSearch`, enforcing parse-don't-validate.

### SQL Generation

```go
frag := search.BuildSQL(validated, 2) // $1 = tenant_id

query := fmt.Sprintf(`
    SELECT t.id, t.title, t.status, t.created_at
    FROM tasks t
    WHERE t.tenant_id = $1
      AND t.deleted_at IS NULL
      %s
      %s
    ORDER BY %s
    LIMIT %d`,
    frag.Where,       // "AND t.status = $2 AND t.created_at >= $3"
    frag.CursorWhere, // "AND (t.created_at, t.id) < ($4, $5)"
    frag.OrderBy,     // "t.created_at DESC, t.id ASC"
    frag.Limit,       // limit+1 for has_more detection
)

args := append([]any{tenantID}, frag.Args...)
rows, err := pool.Query(ctx, query, args...)
```

The library generates only the dynamic WHERE/ORDER BY/LIMIT fragment. Products own the SELECT, FROM, joins, and fixed clauses (tenant scoping, soft-delete).

**SQL injection prevention:** Column names are resolved from the schema allowlist at validation time. All values use positional parameters. The `contains` operator escapes `%`, `_`, and `\`.

### Full-Text Search

Products pre-compute a `tsvector` column (via trigger or generated column) and declare it in the schema. The library handles the rest:

- **Filtering:** `WHERE search_vector @@ plainto_tsquery('english', $N)`
- **Relevance sort:** When `query` is present with no explicit sort, results are ordered by `ts_rank` descending
- **Explicit sort override:** When the client provides a `sort`, relevance ranking is dropped; only the `@@` filter applies
- **Cursor interaction:** Cursor pagination is disabled during relevance sort (use `offset` instead)

### Cursor Pagination

Keyset pagination (seek method). Cursors encode sort column values + primary key, base64url-encoded.

```go
resp, err := search.BuildResponse(tasks, validated, func(t Task) map[string]any {
    return map[string]any{"created_at": t.CreatedAt, "id": t.ID}
})
// resp.Data       -- trimmed to limit (extra row removed)
// resp.HasMore    -- true if more results exist
// resp.NextCursor -- opaque token for the next page
// resp.Limit      -- applied limit
```

### Huma Registration

Wire up a complete search endpoint with one call:

```go
search.RegisterSearchOperation[Task](api, "/tasks/search", taskSearchSchema,
    func(ctx context.Context, req search.ValidatedSearch) (*search.SearchResponse[Task], error) {
        return taskRepo.Search(ctx, tenantID, req)
    },
    search.WithTag("Tasks"),
    search.WithSummary("Search tasks"),
)
```

This registers the POST endpoint, binds the request body, validates before calling your handler, and renders errors through the standard envelope.

## Response Envelope

```json
{
  "data": [{"id": "...", "title": "...", "status": "open"}],
  "next_cursor": "eyJ2IjoxLC...",
  "has_more": true,
  "limit": 25
}
```

## API Versioning

The library is agnostic about API versions. Products select the appropriate `ResourceSchema` based on the resolved `API-Version` header:

```go
func taskSchemaForVersion(v apiversion.Version) search.ResourceSchema {
    schema := baseTaskSchema
    if v.OnOrAfter("2026-06-15") {
        schema.Fields["priority"] = updatedPriorityField
    }
    return schema
}
```

## Performance

The library does not create or manage indexes. Recommended patterns:

```sql
-- Cursor pagination (composite index matching default sort)
CREATE INDEX idx_tasks_search ON tasks (tenant_id, created_at DESC, id ASC)
    WHERE deleted_at IS NULL;

-- Full-text search
CREATE INDEX idx_tasks_search_vector ON tasks USING gin (search_vector);

-- Contains (ILIKE) on high-cardinality text
CREATE INDEX idx_tasks_title_trgm ON tasks USING gin (title gin_trgm_ops);
```

## Development

```bash
# Run tests
go test ./...

# Run tests with race detector
go test -race ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Vet
go vet ./...
```

## Package Structure

```
platform/
├── errors/
│   ├── errors.go       # Error interface, SemanticError, constructors, options
│   ├── pg.go           # MapPgError() -- pgx/pgconn error mapper
│   ├── http.go         # MapHTTPStatus() -- HTTP status code mapper
│   └── huma.go         # NewHumaErrorHandler(), ToHumaError(), ErrorBody
├── search/
│   ├── schema.go       # ResourceSchema, FieldDef, FieldType, FullTextConfig
│   ├── types.go        # SearchRequest, SearchResponse
│   ├── validated.go    # ValidatedSearch, ValidatedFilter, SortDirective
│   ├── errors.go       # ValidationError, error code constants
│   ├── validate.go     # Validate() -- exhaustive request validation
│   ├── builder.go      # BuildSQL() -- SQL fragment generation
│   ├── cursor.go       # EncodeCursor(), DecodeCursor()
│   ├── response.go     # BuildResponse[T]() -- pagination helper
│   └── huma.go         # RegisterSearchOperation() -- Huma endpoint wiring
```
