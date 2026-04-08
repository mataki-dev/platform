# Mataki Platform Search Library — Technical Specification

**Module:** `github.com/mataki-dev/platform`
**Package:** `platform/search`
**Status:** Draft
**Author:** Seth / Claude
**Date:** 2026-04-07

---

## 1. Purpose

Every Mataki product exposes searchable resources via `POST /{resource}/search`. The search package provides the shared machinery for this pattern: request types, validation, SQL generation, cursor pagination, and Huma operation wiring. Products supply only a declarative field schema and execute the resulting query. Everything else — parsing, validation, SQL fragment building, cursor encoding, response envelope, OpenAPI documentation — is handled once, here.

### 1.1 Design Constraints

- All Mataki APIs use Go/Huma with PostgreSQL (pgx) and sqlc.
- sqlc owns all static queries. The search package owns only the dynamic `WHERE`/`ORDER BY`/`LIMIT` fragment for search operations.
- All Mataki APIs use date-based versioning via the `API-Version` header (Stripe-style). The search contract must be stable across versions; per-product field availability may vary by version.
- The library is opinionated about shape, agnostic about domain. It knows what a filter is. It does not know what a "project" or a "notification" is.

### 1.2 Non-Goals

- Full ORM or query builder. Only the dynamic search fragment is generated.
- Aggregation, faceted search, or full-text ranking. Products needing these use purpose-built queries or a dedicated search index.
- Multi-table joins. The search package generates clauses against a single aliased table. Products compose joins themselves.

---

## 2. Request Schema

All search endpoints accept a JSON body conforming to this shape:

```json
{
  "query": "some full-text query string",
  "filter": {
    "status": { "eq": "active" },
    "created_at": { "gte": "2026-01-01T00:00:00Z" },
    "tags": { "in": ["priority", "urgent"] }
  },
  "sort": [
    { "field": "created_at", "direction": "desc" }
  ],
  "cursor": "eyJsYXN0X2lkIjo...",
  "offset": 25,
  "limit": 25,
  "fields": ["id", "name", "status", "created_at"]
}
```

All top-level keys are optional. An empty body `{}` returns the default page of results with default sort order, as defined by the product's resource schema.

### 2.1 Filter

`filter` is a map of field name to operator expression. Each field maps to an object containing one or more operator keys.

#### Supported Operators

| Operator   | JSON Key     | Applicable Types              | Description                              |
|------------|--------------|-------------------------------|------------------------------------------|
| Equal      | `eq`         | All                           | Exact match.                             |
| Not Equal  | `neq`        | All                           | Exclusion match.                         |
| Greater    | `gt`         | Numeric, Timestamp            | Strictly greater than.                   |
| Greater/Eq | `gte`        | Numeric, Timestamp            | Greater than or equal.                   |
| Less       | `lt`         | Numeric, Timestamp            | Strictly less than.                      |
| Less/Eq    | `lte`        | Numeric, Timestamp            | Less than or equal.                      |
| In         | `in`         | String, Numeric, Enum         | Value is one of the provided set.        |
| Not In     | `nin`        | String, Numeric, Enum         | Value is none of the provided set.       |
| Contains   | `contains`   | String                        | Case-insensitive substring match.        |
| Is Null    | `is_null`    | All (nullable fields only)    | Boolean. `true` = IS NULL, `false` = IS NOT NULL. |

Multiple operators on the same field are AND-joined:

```json
{ "created_at": { "gte": "2026-01-01", "lt": "2026-04-01" } }
```

Multiple fields in `filter` are AND-joined. OR logic is intentionally unsupported in v1. Products needing OR semantics should expose purpose-built endpoints or query parameters rather than extending the shared filter grammar.

#### Operator Validation

Each operator+type combination is validated:

- `gt`, `gte`, `lt`, `lte` are rejected on `String` and `Enum` fields.
- `contains` is rejected on `Numeric`, `Timestamp`, and `Bool` fields.
- `in` and `nin` enforce a maximum set size (configurable per resource, default 100).
- `is_null` is only accepted on fields explicitly marked nullable in the resource schema.

### 2.2 Sort

`sort` is an ordered array of sort directives. Each directive has a `field` (must be declared sortable in the resource schema) and a `direction` (`asc` or `desc`, defaulting to `asc`).

Maximum sort directives: 3. The system always appends a tiebreaker sort on the resource's primary key (`id`) if not already present, to guarantee cursor stability.

### 2.3 Cursor

Opaque, base64url-encoded token representing the position after the last returned result. Clients must treat cursors as opaque strings. The internal structure is defined in §5.

`cursor` is mutually exclusive with explicit offset-based pagination.

### 2.4 Limit

Integer, clamped to `[1, resource.MaxLimit]`. Defaults to `resource.DefaultLimit`. The response always includes the applied limit.

### 2.5 Fields

Optional array of field names for response projection. When provided, only the listed fields (plus `id`, which is always included) appear in the response. Invalid field names are rejected. When omitted, all default fields are returned.

Field selection controls the response shape. It does not control the SQL `SELECT` list directly — products may choose to always fetch all columns and project in the application layer, or may generate dynamic column lists. This is a product-level decision, not a library concern.

---

## 3. Resource Schema Definition

Each product registers searchable resources by defining a `ResourceSchema`. This is the only per-product code the library requires.

```go
package search

type FieldType int

const (
    String    FieldType = iota
    Numeric             // int, float, decimal
    Timestamp           // RFC 3339
    Bool
    Enum                // validated against AllowedValues
)

type FieldDef struct {
    // Type governs which operators are valid and how values are parsed.
    Type FieldType

    // Column is the SQL column name. Defaults to the field key if empty.
    Column string

    // Operators lists the permitted operators for this field.
    // If empty, all type-compatible operators are allowed.
    Operators []string

    // Sortable indicates whether this field can appear in sort directives.
    Sortable bool

    // Nullable indicates whether is_null operator is accepted.
    Nullable bool

    // AllowedValues constrains Enum fields. Ignored for other types.
    AllowedValues []string

    // Selectable indicates whether this field can appear in the fields
    // projection list. Defaults to true if unset.
    Selectable *bool

    // MaxInSize overrides the default maximum set size for in/nin operators
    // on this field. 0 means use the resource default.
    MaxInSize int
}

type ResourceSchema struct {
    // Fields maps API field names to their definitions.
    Fields map[string]FieldDef

    // MaxLimit is the ceiling for the limit parameter.
    MaxLimit int

    // DefaultLimit is applied when no limit is specified.
    DefaultLimit int

    // DefaultSort is applied when no sort is specified.
    DefaultSort []SortDirective

    // PrimaryKey is the column used as the cursor tiebreaker.
    // Defaults to "id".
    PrimaryKey string

    // MaxInSize is the default maximum set size for in/nin operators.
    // Defaults to 100.
    MaxInSize int

    // TableAlias is the SQL table alias used in generated fragments.
    // Products must use this alias in their surrounding query.
    TableAlias string
}
```

### 3.1 Example Registration

```go
var TaskSearchSchema = search.ResourceSchema{
    Fields: map[string]search.FieldDef{
        "status":      {Type: search.Enum, Operators: []string{"eq", "neq", "in"}, Sortable: true, AllowedValues: []string{"open", "in_progress", "done", "archived"}},
        "assignee_id": {Type: search.String, Operators: []string{"eq", "in", "is_null"}, Nullable: true},
        "priority":    {Type: search.Numeric, Operators: []string{"eq", "gte", "lte", "in"}, Sortable: true},
        "title":       {Type: search.String, Operators: []string{"contains"}},
        "created_at":  {Type: search.Timestamp, Sortable: true},
        "updated_at":  {Type: search.Timestamp, Sortable: true},
    },
    MaxLimit:     100,
    DefaultLimit: 25,
    DefaultSort:  []search.SortDirective{{Field: "created_at", Dir: search.Desc}},
    TableAlias:   "t",
}
```

### 3.2 API Version Gating

Products may maintain multiple schema versions keyed by API version date. The library does not manage this mapping — it receives whichever `ResourceSchema` the product selects based on the resolved API version. A product might implement this as:

```go
func taskSearchSchemaForVersion(v apiversion.Version) search.ResourceSchema {
    schema := baseTaskSearchSchema
    if v.OnOrAfter("2026-06-15") {
        schema.Fields["priority"] = updatedPriorityField
    }
    return schema
}
```

---

## 4. Validation

`search.Validate(req SearchRequest, schema ResourceSchema) (ValidatedSearch, []ValidationError)`

Validation is exhaustive — all errors are collected before returning, not fail-fast. This gives API consumers a single round-trip to fix all issues.

### 4.1 Validation Rules

**Filter validation:**

- Field name exists in schema.
- Operator is valid for the field's type and permitted operator list.
- Value type is compatible (e.g., numeric string parses to number, timestamp parses to RFC 3339).
- `in`/`nin` set size is within bounds.
- `is_null` is only used on nullable fields and its value is a boolean.
- Enum values are in the `AllowedValues` set.

**Sort validation:**

- Field exists in schema and is marked `Sortable`.
- Direction is `asc` or `desc`.
- Maximum 3 sort directives.
- No duplicate fields in sort.

**Cursor validation:**

- If present, decodes successfully (base64url).
- Internal structure matches the current sort configuration (sort fields must match the cursor's sort signature). A cursor generated with one sort order is invalid for a different sort order.

**Limit validation:**

- Integer in `[1, MaxLimit]`.

**Fields validation:**

- Every field name exists in schema and is selectable.

### 4.2 ValidationError

```go
type ValidationError struct {
    // Field is the dotted path to the invalid element.
    // e.g., "filter.status.eq", "sort[1].field", "limit"
    Field   string `json:"field"`

    // Code is a machine-readable error code.
    Code    string `json:"code"`

    // Message is a human-readable description.
    Message string `json:"message"`
}
```

Error codes are stable strings: `unknown_field`, `invalid_operator`, `invalid_value`, `invalid_type`, `exceeds_max`, `invalid_cursor`, `unsortable_field`, `duplicate_sort_field`, `not_nullable`.

### 4.3 ValidatedSearch

The output of validation is a `ValidatedSearch` struct — a normalized, type-safe representation of the search request. The SQL builder only accepts `ValidatedSearch`, never raw `SearchRequest`. This enforces the parse-don't-validate principle: once validated, the search is known-good.

```go
type ValidatedSearch struct {
    Filters  []ValidatedFilter
    Sort     []SortDirective // includes tiebreaker
    Cursor   *DecodedCursor  // nil if not provided
    Limit    int
    Fields   []string        // nil means all defaults
}

type ValidatedFilter struct {
    Field    string
    Column   string    // resolved SQL column name
    Operator string
    Value    any       // parsed to correct Go type
}

type SortDirective struct {
    Field     string    `json:"field"`
    Column    string    `json:"-"`     // resolved SQL column
    Dir       SortDir   `json:"direction"`
}

type SortDir string

const (
    Asc  SortDir = "asc"
    Desc SortDir = "desc"
)
```

---

## 5. Cursor Pagination

### 5.1 Encoding

Cursors use keyset pagination (seek method). The cursor encodes the values of the sort columns and the primary key of the last returned row.

Internal structure (before base64url encoding):

```json
{
  "v": 1,
  "s": [
    {"f": "created_at", "d": "desc", "val": "2026-03-15T10:30:00Z"},
    {"f": "id", "d": "asc", "val": "task_abc123"}
  ]
}
```

The `v` field is a schema version for the cursor format itself, allowing future changes without breaking existing cursors. The JSON is then base64url-encoded (no padding).

### 5.2 Query Generation

For a sort of `[created_at DESC, id ASC]` with a cursor, the builder generates a `WHERE` clause using row-value comparison:

```sql
WHERE (t.created_at, t.id) < ($N, $N+1)
```

For mixed-direction sorts, the builder decomposes into the equivalent expanded form:

```sql
WHERE (t.created_at < $N)
   OR (t.created_at = $N AND t.id > $N+1)
```

### 5.3 Cursor Validation

A cursor is rejected if:

- It fails to decode (malformed base64 or JSON).
- Its sort signature (field names + directions) does not match the request's sort configuration. This prevents using a cursor from one sort order with a different one.
- Its `v` field indicates an unsupported cursor version.

### 5.4 Response Envelope

```go
type SearchResponse[T any] struct {
    Data      []T     `json:"data"`
    NextCursor string `json:"next_cursor,omitempty"`
    HasMore   bool    `json:"has_more"`
    Limit     int     `json:"limit"`
}
```

`HasMore` is determined by fetching `limit + 1` rows and checking whether the extra row exists. The extra row is never included in `Data`. `NextCursor` is only present when `HasMore` is true.

### 5.5 Helper Functions

```go
// EncodeCursor builds a cursor from the last row in a result set.
// sortFields defines which fields to extract and their directions.
func EncodeCursor(lastRow map[string]any, sort []SortDirective) string

// DecodeCursor parses and validates a cursor token.
func DecodeCursor(token string) (*DecodedCursor, error)

// CursorMatchesSort checks whether a decoded cursor is compatible
// with the given sort directives.
func CursorMatchesSort(c *DecodedCursor, sort []SortDirective) bool
```

---

## 6. SQL Builder

`search.BuildSQL(vs ValidatedSearch, argOffset int) SQLFragment`

The builder accepts only `ValidatedSearch` (never raw input) and an `argOffset` indicating the starting positional parameter index. Products prepend their own fixed clauses (tenant scoping, soft-delete, access control) with parameters `$1` through `$argOffset-1`, then append the search fragment.

### 6.1 SQLFragment

```go
type SQLFragment struct {
    // Where is the AND-joined clause string, including the leading "AND"
    // if any filters are present. Empty string if no filters.
    // e.g., "AND t.status = $2 AND t.created_at >= $3"
    Where string

    // CursorWhere is the cursor seek clause, including leading "AND"
    // if a cursor is present. Empty string if no cursor.
    CursorWhere string

    // OrderBy is the comma-separated ORDER BY expression.
    // e.g., "t.created_at DESC, t.id ASC"
    OrderBy string

    // Limit is the resolved limit + 1 (for has_more detection).
    Limit int

    // Args contains the parameter values in positional order,
    // corresponding to $argOffset, $argOffset+1, etc.
    Args []any

    // ArgCount is the total number of positional parameters
    // contributed by this fragment.
    ArgCount int
}
```

### 6.2 Operator-to-SQL Mapping

| Operator   | SQL                              | Notes                            |
|------------|----------------------------------|----------------------------------|
| `eq`       | `col = $N`                       |                                  |
| `neq`      | `col != $N`                      |                                  |
| `gt`       | `col > $N`                       |                                  |
| `gte`      | `col >= $N`                      |                                  |
| `lt`       | `col < $N`                       |                                  |
| `lte`      | `col <= $N`                      |                                  |
| `in`       | `col = ANY($N)`                  | `$N` is a pgx array parameter.  |
| `nin`      | `col != ALL($N)`                 | `$N` is a pgx array parameter.  |
| `contains` | `col ILIKE $N`                   | Value wrapped in `%..%`. Input `%` and `_` are escaped. |
| `is_null`  | `col IS NULL` / `col IS NOT NULL`| No parameter; literal SQL.       |

### 6.3 SQL Injection Prevention

- Column names are resolved from the `ResourceSchema` at validation time. The SQL builder uses the pre-validated `Column` field, never user input directly. Column names are drawn from a fixed allowlist (the schema definition).
- All values are emitted as positional parameters (`$N`), never interpolated.
- The `contains` operator escapes `%`, `_`, and `\` in the input value before wrapping in `%..%`.

### 6.4 Product Integration Pattern

```go
func (r *TaskRepository) Search(ctx context.Context, tenantID string, req search.ValidatedSearch) (*search.SearchResponse[Task], error) {
    frag := search.BuildSQL(req, 2) // $1 = tenant_id

    query := fmt.Sprintf(`
        SELECT t.id, t.title, t.status, t.priority, t.assignee_id,
               t.created_at, t.updated_at
        FROM tasks t
        WHERE t.tenant_id = $1
          AND t.deleted_at IS NULL
          %s
          %s
        ORDER BY %s
        LIMIT %d`,
        frag.Where,
        frag.CursorWhere,
        frag.OrderBy,
        frag.Limit,
    )

    args := append([]any{tenantID}, frag.Args...)
    rows, err := r.pool.Query(ctx, query, args...)
    if err != nil {
        return nil, fmt.Errorf("search tasks: %w", err)
    }
    defer rows.Close()

    tasks, err := pgx.CollectRows(rows, pgx.RowToStructByName[Task])
    if err != nil {
        return nil, fmt.Errorf("scan tasks: %w", err)
    }

    return search.BuildResponse(tasks, req)
}
```

### 6.5 BuildResponse Helper

```go
// BuildResponse handles the limit+1 trim, has_more detection, and
// cursor encoding for the last row.
func BuildResponse[T any](rows []T, req ValidatedSearch, extractFn func(T) map[string]any) (*SearchResponse[T], error)
```

The `extractFn` callback extracts sort-field values from the last row for cursor encoding. This is the one product-specific function required — it maps the product's row struct to the map of values the cursor needs.

---

## 7. Huma Integration

The package provides a registration helper that wires up the `POST /{resource}/search` operation with consistent OpenAPI documentation, request validation, and error handling.

```go
// RegisterSearchOperation registers a POST /{resource}/search endpoint
// with standard request/response types, validation, and OpenAPI metadata.
func RegisterSearchOperation[T any](
    api huma.API,
    path string,
    schema ResourceSchema,
    handler func(ctx context.Context, req *SearchRequest) (*SearchResponse[T], error),
    opts ...OperationOption,
)
```

### 7.1 What the Helper Provides

- Registers the Huma operation with appropriate tags, summary, and description.
- Binds the `SearchRequest` body type with full OpenAPI schema (including enum values from `FieldDef.AllowedValues`, field descriptions, and operator constraints).
- Calls `Validate` before invoking the product handler. If validation fails, returns a `422 Unprocessable Entity` with the standard error body (see §8).
- Wraps the `SearchResponse[T]` as the `200` response type.
- Adds a `400` response for malformed JSON and a `422` for validation errors.

### 7.2 OperationOption

Configuration overrides for the registration:

```go
type OperationOption func(*operationConfig)

func WithTag(tag string) OperationOption
func WithSummary(summary string) OperationOption
func WithDescription(desc string) OperationOption
func WithMiddleware(mw ...func(huma.Context, func(huma.Context))) OperationOption
```

### 7.3 Middleware Ordering

Products typically apply tenant resolution and authentication middleware at the router level. The search operation helper does not add its own auth middleware — it operates within whatever middleware stack the product has already configured. Products can inject additional per-operation middleware via `WithMiddleware`.

---

## 8. Error Responses

Search endpoints use the same error envelope as all Mataki APIs.

### 8.1 Validation Error (422)

```json
{
  "type": "validation_error",
  "message": "The search request contains invalid parameters.",
  "errors": [
    {
      "field": "filter.priority.eq",
      "code": "invalid_type",
      "message": "Expected numeric value, got string."
    },
    {
      "field": "sort[0].field",
      "code": "unsortable_field",
      "message": "Field 'title' is not sortable."
    }
  ]
}
```

### 8.2 Invalid Cursor (400)

```json
{
  "type": "invalid_request",
  "message": "The provided cursor is invalid or expired.",
  "errors": [
    {
      "field": "cursor",
      "code": "invalid_cursor",
      "message": "Cursor sort signature does not match the request sort order."
    }
  ]
}
```

---

## 9. Package Structure

```
platform/
├── search/
│   ├── types.go         # SearchRequest, FilterExpr, SortDirective, SearchResponse
│   ├── schema.go        # ResourceSchema, FieldDef, FieldType
│   ├── validate.go      # Validate(), ValidationError
│   ├── validated.go     # ValidatedSearch, ValidatedFilter (output types)
│   ├── builder.go       # BuildSQL(), SQLFragment
│   ├── cursor.go        # EncodeCursor(), DecodeCursor(), cursor types
│   ├── response.go      # BuildResponse() generic helper
│   ├── huma.go          # RegisterSearchOperation(), OperationOption
│   ├── errors.go        # Error codes, error constructors
│   └── search_test.go   # Unit tests (validation, builder, cursor round-trip)
```

---

## 10. Testing Strategy

### 10.1 Unit Tests (in the search package)

- **Validation:** exhaustive matrix of valid/invalid filter+operator+type combinations. Every error code path has a test. Multi-error accumulation is tested.
- **SQL builder:** verified output for single filters, compound filters, all operator types, empty filters, cursor clauses, mixed-direction sorts. Parameter indices are verified.
- **Cursor round-trip:** encode then decode, verify all values survive. Sort signature mismatch detection. Malformed input handling.
- **Contains escaping:** input values with `%`, `_`, and `\` produce correct escaped ILIKE patterns.

### 10.2 Integration Tests (in consuming products)

Products write integration tests against a real PostgreSQL instance to verify that generated SQL executes correctly and returns expected rows. The search package itself does not depend on a database.

### 10.3 Property-Based Tests

The validation and cursor modules are strong candidates for property-based testing (via `rapid` or similar):

- Any `ValidatedSearch` produced by `Validate` generates syntactically valid SQL.
- Any cursor produced by `EncodeCursor` round-trips through `DecodeCursor`.
- Any filter value that passes validation produces a query that does not error against PostgreSQL.

---

## 11. Performance Considerations

### 11.1 Index Alignment

The library does not create or manage indexes. Products are responsible for ensuring appropriate indexes exist for their searchable fields. The resource schema documentation should note which field combinations require composite indexes for acceptable search performance.

Recommended index pattern for cursor pagination:

```sql
CREATE INDEX idx_tasks_search_default
    ON tasks (tenant_id, created_at DESC, id ASC)
    WHERE deleted_at IS NULL;
```

### 11.2 Contains Operator

`ILIKE '%..%'` cannot use B-tree indexes. Products exposing `contains` on high-cardinality text fields should consider a `pg_trgm` GIN index:

```sql
CREATE INDEX idx_tasks_title_trgm
    ON tasks USING gin (title gin_trgm_ops);
```

The search package does not enforce this — it is a product-level infrastructure decision documented in the resource schema.

### 11.3 IN Set Size

Large `IN` sets degrade query plan quality. The default maximum of 100 is intentionally conservative. Products may lower this per-field via `FieldDef.MaxInSize`.

---

## 12. Future Considerations

These are explicitly out of scope for the initial implementation but are anticipated extensions. The design accommodates them without structural changes.

**OR logic.** Could be introduced as a `filter_any` sibling to `filter` (AND-joined within each, OR-joined between them), or as an explicit `$or` operator wrapping multiple filter groups. Deferred until a product has a concrete need.

**Aggregation / count.** A `POST /{resource}/search/count` variant returning only the total count matching the filters, without pagination or row data. The validation and SQL WHERE generation are reusable.

**Saved searches.** Products could persist `SearchRequest` bodies as saved views. The stable, versioned JSON schema makes this straightforward.
