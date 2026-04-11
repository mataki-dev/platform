# errors

Consistent error handling across all Mataki APIs. Five semantic error types
map to HTTP status codes and render through a standard JSON envelope.

## Install

    import "github.com/mataki-dev/platform/errors"

## Design

Every Mataki API returns errors in a uniform shape: a machine-readable `type`,
a human-readable `message`, and an optional `errors` array for field-level
validation failures. The `errors` package enforces this contract at the type
level. There are exactly five error types — `NotFound`, `Conflict`,
`Forbidden`, `InvalidInput`, and `Internal` — because they cover the common
4xx/5xx HTTP space without over-specializing. Adding more types would create
ambiguity about which to reach for; collapsing them further would lose the
HTTP status fidelity that clients depend on.

The package follows parse-don't-validate: every constructor requires a message
and returns a fully-formed `Error` interface. There is no zero-value error, no
code to set after the fact, no way to produce a `*SemanticError` without a
type. This eliminates a class of bugs where errors travel up the call stack
with missing or default fields.

`WithCause` wraps an original error so that `errors.Is` and `errors.As`
unwrapping works correctly for callers that inspect error chains, and so that
structured loggers can record the root cause. The cause is never included in
API responses. `WithDetail` attaches arbitrary key-value context for the same
logging purposes — it is also never returned to API consumers. Both options
are additive: callers opt in only when the context is useful.

## Error Types

| Constructor | Code | HTTP | Use |
|---|---|---|---|
| `NewNotFound` | `not_found` | 404 | Resource doesn't exist |
| `NewConflict` | `conflict` | 409 | Unique constraint / duplicate |
| `NewForbidden` | `forbidden` | 403 | Authorization failure |
| `NewInvalidInput` | `invalid_input` | 422 | Validation / bad input |
| `NewInternal` | `internal` | 500 | Unexpected errors |

## Constructors and Options

```go
// Preserve the original error for errors.Is / errors.As
err := errors.NewNotFound("user not found", errors.WithCause(originalErr))

// Attach structured context (for logging, never exposed in API responses)
err := errors.NewConflict("duplicate email",
    errors.WithDetail("constraint", "users_email_key"),
)
```

All five constructors accept the same variadic `Option` arguments:

```go
func NewNotFound(msg string, opts ...Option) Error
func NewConflict(msg string, opts ...Option) Error
func NewForbidden(msg string, opts ...Option) Error
func NewInvalidInput(msg string, opts ...Option) Error
func NewInternal(msg string, opts ...Option) Error

func WithCause(err error) Option
func WithDetail(key string, val any) Option
```

## Infrastructure Mappers

### PostgreSQL

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
```

`MapPgError` returns `nil` for unrecognized errors intentionally. Products
must consciously decide whether an unknown PostgreSQL error is `Internal` or
requires specific handling. Swallowing it silently would hide unexpected
database conditions.

### HTTP

```go
// HTTP -- maps upstream service responses to semantic errors.
err := errors.MapHTTPStatus(resp.StatusCode, "upstream failed")
```

Maps 404→`NotFound`, 403→`Forbidden`, 409→`Conflict`, 422→`InvalidInput`,
5xx→`Internal`, and other 4xx→`InvalidInput`.

## Huma Integration

Register the error handler once at startup so all unhandled errors render
through the standard envelope:

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

The `errors` array is present only for `InvalidInput` responses that carry
multiple field-level issues. Other error types (`NotFound`, `Conflict`,
`Forbidden`, `Internal`) omit it. Unrecognized errors passed to the handler
become 500 `Internal`.

## Testing

    go test -race ./errors/...
