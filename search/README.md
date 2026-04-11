# search

Shared infrastructure for Mataki search endpoints (`POST /{resource}/search`).
Products supply a declarative `ResourceSchema` and execute the resulting query;
the library handles request parsing, validation, SQL fragment generation,
cursor pagination, and Huma operation registration.

## Install

    import "github.com/mataki-dev/platform/search"

## Design

The search package is built around a single principle: products describe their
searchable fields declaratively, and the library enforces the contract. A product
registers a `ResourceSchema` — a map of field names to `FieldDef` values that
declare types, allowed operators, sort eligibility, and enum constraints. The
library owns everything that can be shared: request parsing, validation, SQL
fragment generation, cursor encoding, and OpenAPI operation wiring.

Validation produces a `ValidatedSearch`, and the SQL builder only accepts that
type — never raw `SearchRequest` input. This is the parse-don't-validate
principle applied strictly: once a search passes `Validate`, it is known-good,
and no downstream code needs to re-check assumptions. `ValidatedSearch` carries
resolved column names and parsed Go values, not raw strings.

SQL injection is prevented through two mechanisms. Column names are resolved
from the schema allowlist at validation time; the builder uses the
pre-validated `Column` field, never user input directly. All filter values use
positional parameters (`$N`). The `contains` operator additionally escapes `%`,
`_`, and `\` before wrapping the value in `%..%` for ILIKE.

The library generates only the dynamic `WHERE`/`ORDER BY`/`LIMIT` fragment.
Products own the `SELECT`, `FROM`, joins, tenant scoping, and soft-delete
clauses. This keeps the library domain-agnostic while giving products full
control over access control and data shape. Cursor pagination uses keyset
(seek method), not offset — cursors encode the sort column values and primary
key of the last returned row, giving stable pages that do not degrade with
table size.

## Request Schema

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

## Resource Schema

Products define a `ResourceSchema` once per searchable resource. This is the
only per-product configuration the library requires:

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

## Field Types and Operators

| Type        | Operators                                          |
|-------------|----------------------------------------------------|
| `String`    | `eq`, `neq`, `in`, `nin`, `contains`, `is_null`   |
| `Numeric`   | `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `in`, `nin`, `is_null` |
| `Timestamp` | `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `is_null`  |
| `Bool`      | `eq`, `neq`, `is_null`                             |
| `Enum`      | `eq`, `neq`, `in`, `nin`, `is_null`                |

Restrict operators per-field via `FieldDef.Operators`. If empty, all
type-compatible operators are allowed.

`is_null` is only accepted on fields explicitly marked `Nullable: true` in the
resource schema. Multiple operators on the same field are AND-joined. Multiple
fields in `filter` are AND-joined. OR logic is intentionally unsupported in v1.

## Validation

```go
validated, errs := search.Validate(req, schema)
if len(errs) > 0 {
    // errs is []ValidationError with Field, Code, Message
    // Validation is exhaustive -- all errors collected, not fail-fast.
}
```

The SQL builder only accepts `ValidatedSearch`, enforcing parse-don't-validate.

`ValidationError` carries a dotted `Field` path (e.g., `filter.status.eq`,
`sort[1].field`), a machine-readable `Code`, and a human-readable `Message`.
Stable error codes: `unknown_field`, `invalid_operator`, `invalid_value`,
`invalid_type`, `exceeds_max`, `invalid_cursor`, `unsortable_field`,
`duplicate_sort_field`, `not_nullable`, `unsupported_field`.

## SQL Generation

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

The library generates only the dynamic WHERE/ORDER BY/LIMIT fragment. Products
own the SELECT, FROM, joins, and fixed clauses (tenant scoping, soft-delete).

**SQL injection prevention:** Column names are resolved from the schema
allowlist at validation time. All values use positional parameters. The
`contains` operator escapes `%`, `_`, and `\`.

## Full-Text Search

Products pre-compute a `tsvector` column (via trigger or generated column) and
declare it in the schema. The library handles the rest:

- **Filtering:** `WHERE search_vector @@ plainto_tsquery('english', $N)`
- **Relevance sort:** When `query` is present with no explicit sort, results are ordered by `ts_rank` descending
- **Explicit sort override:** When the client provides a `sort`, relevance ranking is dropped; only the `@@` filter applies
- **Cursor interaction:** Cursor pagination is disabled during relevance sort (use `offset` instead)

## Cursor Pagination

Keyset pagination (seek method). Cursors encode sort column values + primary
key, base64url-encoded.

```go
resp, err := search.BuildResponse(tasks, validated, func(t Task) map[string]any {
    return map[string]any{"created_at": t.CreatedAt, "id": t.ID}
})
// resp.Data       -- trimmed to limit (extra row removed)
// resp.HasMore    -- true if more results exist
// resp.NextCursor -- opaque token for the next page
// resp.Limit      -- applied limit
```

The `extractFn` callback maps the product's row struct to the cursor value map.
`has_more` is detected by fetching `limit + 1` rows; the extra row is never
included in `Data`. For mixed-direction sorts, the builder generates the
expanded boolean form rather than row-value comparison.

## Huma Registration

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

This registers the POST endpoint, binds the request body, validates before
calling your handler, and renders errors through the standard envelope. On
validation failure the handler is not called; a `422 Unprocessable Entity` is
returned automatically with all collected errors.

Available options: `WithTag`, `WithSummary`, `WithDescription`,
`WithMiddleware`.

## Response Envelope

```json
{
  "data": [{"id": "...", "title": "...", "status": "open"}],
  "next_cursor": "eyJ2IjoxLC...",
  "has_more": true,
  "limit": 25
}
```

`next_cursor` is omitted when `has_more` is false. During relevance sort
(full-text query with no explicit sort), `next_cursor` is always omitted;
clients must use `offset` for pagination.

## API Versioning

The library is agnostic about API versions. Products select the appropriate
`ResourceSchema` based on the resolved `API-Version` header:

```go
func taskSchemaForVersion(v apiversion.Version) search.ResourceSchema {
    schema := baseTaskSchema
    if v.OnOrAfter("2026-06-15") {
        schema.Fields["priority"] = updatedPriorityField
    }
    return schema
}
```

Pass the resolved schema to `Validate` and `RegisterSearchOperation`. The
library does not manage version mapping — that is a product-level decision.
The search request contract (`SearchRequest` shape, filter grammar, cursor
format) is stable across versions; per-product field availability may vary.

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

`ILIKE '%..%'` cannot use B-tree indexes. Products exposing `contains` on
high-cardinality text fields should add a `pg_trgm` GIN index. Large `IN`
sets degrade query plan quality; the default maximum is 100, configurable
per-field via `FieldDef.MaxInSize`.

## Testing

    go test -race ./search/...

Unit tests cover: exhaustive filter+operator+type validation matrix, every
error code path, multi-error accumulation, SQL builder output for all operator
types, cursor round-trip encode/decode, sort signature mismatch detection,
`contains` escaping, and full-text WHERE and ORDER BY generation. The package
has no database dependency — integration tests live in consuming products.
