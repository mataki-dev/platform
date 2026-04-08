# Mataki Platform Library Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the shared platform library (`errors` and `search` packages) that all Mataki products depend on for consistent error handling and search endpoint infrastructure.

**Architecture:** Two packages — `errors` provides semantic error types, infrastructure mappers (pgx, HTTP), and Huma integration; `search` provides request parsing, validation, SQL fragment generation, cursor pagination, full-text search, and Huma operation registration. `search` depends on `errors`. Both are pure library code with no database dependency.

**Tech Stack:** Go 1.24+, Huma v2, pgx v5, jackc/pgconn (for PG error codes)

**Spec:** `docs/superpowers/specs/2026-04-08-platform-library-design.md`
**Original search spec:** `docs/mataki-platform-search-spec.md`

---

## File Map

```
platform/
├── go.mod
├── errors/
│   ├── errors.go          # Error interface, concrete types, constructors, Options
│   ├── errors_test.go     # Tests for error types, constructors, options
│   ├── pg.go              # MapPgError()
│   ├── pg_test.go         # Tests for PG error mapping
│   ├── http.go            # MapHTTPStatus()
│   ├── http_test.go       # Tests for HTTP status mapping
│   ├── huma.go            # NewHumaErrorHandler()
│   └── huma_test.go       # Tests for Huma error handler
├── search/
│   ├── schema.go          # ResourceSchema, FieldDef, FieldType, FullTextConfig
│   ├── types.go           # SearchRequest, SearchResponse, FilterExpr
│   ├── validated.go       # ValidatedSearch, ValidatedFilter, SortDirective
│   ├── validate.go        # Validate()
│   ├── validate_test.go   # Validation tests
│   ├── builder.go         # BuildSQL(), SQLFragment
│   ├── builder_test.go    # SQL builder tests
│   ├── cursor.go          # EncodeCursor, DecodeCursor, DecodedCursor
│   ├── cursor_test.go     # Cursor round-trip tests
│   ├── response.go        # BuildResponse[T]()
│   ├── response_test.go   # Response helper tests
│   ├── huma.go            # RegisterSearchOperation(), OperationOption
│   └── errors.go          # Search-specific error codes, ValidationError
```

---

### Task 1: Project Initialization

**Files:**
- Create: `go.mod`

- [ ] **Step 1: Initialize Go module**

Run:
```bash
cd /Users/sethyates/mataki/mataki-dev/platform
go mod init github.com/mataki-dev/platform
```

Expected: `go.mod` created with module path `github.com/mataki-dev/platform`.

- [ ] **Step 2: Add dependencies**

Run:
```bash
go get github.com/jackc/pgx/v5
go get github.com/danielgtaylor/huma/v2
```

Expected: `go.mod` and `go.sum` updated with pgx v5 and huma v2 dependencies.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "Initialize Go module with pgx and huma dependencies"
```

---

### Task 2: Error Types and Constructors

**Files:**
- Create: `errors/errors.go`
- Create: `errors/errors_test.go`

- [ ] **Step 1: Write tests for error types and constructors**

Create `errors/errors_test.go`:

```go
package errors_test

import (
	stderrors "errors"
	"testing"

	"github.com/mataki-dev/platform/errors"
)

func TestNewNotFound(t *testing.T) {
	err := errors.NewNotFound("user not found")

	if err.Code() != "not_found" {
		t.Errorf("Code() = %q, want %q", err.Code(), "not_found")
	}
	if err.HTTPStatus() != 404 {
		t.Errorf("HTTPStatus() = %d, want %d", err.HTTPStatus(), 404)
	}
	if err.Message() != "user not found" {
		t.Errorf("Message() = %q, want %q", err.Message(), "user not found")
	}
	if err.Error() != "not_found: user not found" {
		t.Errorf("Error() = %q, want %q", err.Error(), "not_found: user not found")
	}
}

func TestNewConflict(t *testing.T) {
	err := errors.NewConflict("duplicate email")

	if err.Code() != "conflict" {
		t.Errorf("Code() = %q, want %q", err.Code(), "conflict")
	}
	if err.HTTPStatus() != 409 {
		t.Errorf("HTTPStatus() = %d, want %d", err.HTTPStatus(), 409)
	}
	if err.Message() != "duplicate email" {
		t.Errorf("Message() = %q, want %q", err.Message(), "duplicate email")
	}
}

func TestNewForbidden(t *testing.T) {
	err := errors.NewForbidden("access denied")

	if err.Code() != "forbidden" {
		t.Errorf("Code() = %q, want %q", err.Code(), "forbidden")
	}
	if err.HTTPStatus() != 403 {
		t.Errorf("HTTPStatus() = %d, want %d", err.HTTPStatus(), 403)
	}
}

func TestNewInvalidInput(t *testing.T) {
	err := errors.NewInvalidInput("bad request data")

	if err.Code() != "invalid_input" {
		t.Errorf("Code() = %q, want %q", err.Code(), "invalid_input")
	}
	if err.HTTPStatus() != 422 {
		t.Errorf("HTTPStatus() = %d, want %d", err.HTTPStatus(), 422)
	}
}

func TestNewInternal(t *testing.T) {
	err := errors.NewInternal("something broke")

	if err.Code() != "internal" {
		t.Errorf("Code() = %q, want %q", err.Code(), "internal")
	}
	if err.HTTPStatus() != 500 {
		t.Errorf("HTTPStatus() = %d, want %d", err.HTTPStatus(), 500)
	}
}

func TestWithCause(t *testing.T) {
	cause := stderrors.New("underlying problem")
	err := errors.NewInternal("wrapper", errors.WithCause(cause))

	// Must support errors.Is unwrapping
	if !stderrors.Is(err, cause) {
		t.Error("errors.Is should find the cause")
	}

	// Must support errors.As unwrapping
	var target *errors.SemanticError
	if !stderrors.As(err, &target) {
		t.Error("errors.As should unwrap to *SemanticError")
	}
}

func TestWithDetail(t *testing.T) {
	err := errors.NewConflict("duplicate",
		errors.WithDetail("field", "email"),
		errors.WithDetail("value", "test@example.com"),
	)

	details := err.Details()
	if details["field"] != "email" {
		t.Errorf("detail field = %q, want %q", details["field"], "email")
	}
	if details["value"] != "test@example.com" {
		t.Errorf("detail value = %q, want %q", details["value"], "test@example.com")
	}
}

func TestErrorImplementsErrorInterface(t *testing.T) {
	var _ error = errors.NewNotFound("test")
	var _ errors.Error = errors.NewNotFound("test")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./errors/...`
Expected: Compilation failure — package `errors` does not exist yet.

- [ ] **Step 3: Implement error types and constructors**

Create `errors/errors.go`:

```go
package errors

// Error is the semantic error interface all Mataki errors implement.
type Error interface {
	error
	Code() string
	HTTPStatus() int
	Message() string
	Details() map[string]any
}

// SemanticError is the concrete implementation of Error.
type SemanticError struct {
	code       string
	httpStatus int
	message    string
	details    map[string]any
	cause      error
}

func (e *SemanticError) Error() string {
	return e.code + ": " + e.message
}

func (e *SemanticError) Code() string       { return e.code }
func (e *SemanticError) HTTPStatus() int     { return e.httpStatus }
func (e *SemanticError) Message() string     { return e.message }

func (e *SemanticError) Details() map[string]any {
	if e.details == nil {
		return nil
	}
	cp := make(map[string]any, len(e.details))
	for k, v := range e.details {
		cp[k] = v
	}
	return cp
}

func (e *SemanticError) Unwrap() error { return e.cause }

// Option configures a SemanticError.
type Option func(*SemanticError)

// WithCause wraps an underlying error for errors.Is/errors.As unwrapping.
func WithCause(err error) Option {
	return func(e *SemanticError) {
		e.cause = err
	}
}

// WithDetail attaches a key-value pair of context.
func WithDetail(key string, val any) Option {
	return func(e *SemanticError) {
		if e.details == nil {
			e.details = make(map[string]any)
		}
		e.details[key] = val
	}
}

func newError(code string, httpStatus int, msg string, opts []Option) *SemanticError {
	e := &SemanticError{
		code:       code,
		httpStatus: httpStatus,
		message:    msg,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func NewNotFound(msg string, opts ...Option) *SemanticError {
	return newError("not_found", 404, msg, opts)
}

func NewConflict(msg string, opts ...Option) *SemanticError {
	return newError("conflict", 409, msg, opts)
}

func NewForbidden(msg string, opts ...Option) *SemanticError {
	return newError("forbidden", 403, msg, opts)
}

func NewInvalidInput(msg string, opts ...Option) *SemanticError {
	return newError("invalid_input", 422, msg, opts)
}

func NewInternal(msg string, opts ...Option) *SemanticError {
	return newError("internal", 500, msg, opts)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./errors/... -v`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add errors/errors.go errors/errors_test.go
git commit -m "Add semantic error types with constructors and options"
```

---

### Task 3: PostgreSQL Error Mapper

**Files:**
- Create: `errors/pg.go`
- Create: `errors/pg_test.go`

- [ ] **Step 1: Write tests for PG error mapping**

Create `errors/pg_test.go`:

```go
package errors_test

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mataki-dev/platform/errors"
)

func TestMapPgError_NoRows(t *testing.T) {
	err := errors.MapPgError(pgx.ErrNoRows)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Code() != "not_found" {
		t.Errorf("Code() = %q, want %q", err.Code(), "not_found")
	}
	if err.HTTPStatus() != 404 {
		t.Errorf("HTTPStatus() = %d, want %d", err.HTTPStatus(), 404)
	}
}

func TestMapPgError_UniqueViolation(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:           "23505",
		Message:        "duplicate key value violates unique constraint",
		ConstraintName: "users_email_key",
	}
	err := errors.MapPgError(pgErr)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Code() != "conflict" {
		t.Errorf("Code() = %q, want %q", err.Code(), "conflict")
	}
	if err.HTTPStatus() != 409 {
		t.Errorf("HTTPStatus() = %d, want %d", err.HTTPStatus(), 409)
	}
	details := err.Details()
	if details["constraint"] != "users_email_key" {
		t.Errorf("detail constraint = %q, want %q", details["constraint"], "users_email_key")
	}
}

func TestMapPgError_ForeignKeyViolation(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:           "23503",
		Message:        "violates foreign key constraint",
		ConstraintName: "tasks_project_id_fkey",
	}
	err := errors.MapPgError(pgErr)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Code() != "conflict" {
		t.Errorf("Code() = %q, want %q", err.Code(), "conflict")
	}
	details := err.Details()
	if details["constraint"] != "tasks_project_id_fkey" {
		t.Errorf("detail constraint = %q, want %q", details["constraint"], "tasks_project_id_fkey")
	}
}

func TestMapPgError_CheckViolation(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:    "23514",
		Message: "violates check constraint",
	}
	err := errors.MapPgError(pgErr)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Code() != "invalid_input" {
		t.Errorf("Code() = %q, want %q", err.Code(), "invalid_input")
	}
}

func TestMapPgError_NotNullViolation(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:    "23502",
		Message: "violates not-null constraint",
	}
	err := errors.MapPgError(pgErr)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Code() != "invalid_input" {
		t.Errorf("Code() = %q, want %q", err.Code(), "invalid_input")
	}
}

func TestMapPgError_UnrecognizedReturnsNil(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:    "42601", // syntax error
		Message: "syntax error at or near",
	}
	err := errors.MapPgError(pgErr)
	if err != nil {
		t.Errorf("expected nil for unrecognized PG error, got %v", err)
	}
}

func TestMapPgError_NonPgErrorReturnsNil(t *testing.T) {
	err := errors.MapPgError(fmt.Errorf("random error"))
	if err != nil {
		t.Errorf("expected nil for non-PG error, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./errors/... -run TestMapPg`
Expected: Compilation failure — `MapPgError` not defined.

- [ ] **Step 3: Implement PG mapper**

Create `errors/pg.go`:

```go
package errors

import (
	stderrors "errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// MapPgError maps known pgx/pgconn errors to semantic errors.
// Returns nil if the error is not recognized — the caller must decide
// how to handle unrecognized errors.
func MapPgError(err error) *SemanticError {
	if stderrors.Is(err, pgx.ErrNoRows) {
		return NewNotFound("resource not found", WithCause(err))
	}

	var pgErr *pgconn.PgError
	if !stderrors.As(err, &pgErr) {
		return nil
	}

	switch pgErr.Code {
	case "23505": // unique_violation
		return NewConflict(pgErr.Message,
			WithCause(err),
			WithDetail("constraint", pgErr.ConstraintName),
		)
	case "23503": // foreign_key_violation
		return NewConflict(pgErr.Message,
			WithCause(err),
			WithDetail("constraint", pgErr.ConstraintName),
		)
	case "23514": // check_violation
		return NewInvalidInput(pgErr.Message,
			WithCause(err),
			WithDetail("constraint", pgErr.ConstraintName),
		)
	case "23502": // not_null_violation
		return NewInvalidInput(pgErr.Message,
			WithCause(err),
			WithDetail("column", pgErr.ColumnName),
		)
	default:
		return nil
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./errors/... -run TestMapPg -v`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add errors/pg.go errors/pg_test.go
git commit -m "Add PostgreSQL error mapper for pgx/pgconn"
```

---

### Task 4: HTTP Status Mapper

**Files:**
- Create: `errors/http.go`
- Create: `errors/http_test.go`

- [ ] **Step 1: Write tests for HTTP status mapping**

Create `errors/http_test.go`:

```go
package errors_test

import (
	"testing"

	"github.com/mataki-dev/platform/errors"
)

func TestMapHTTPStatus(t *testing.T) {
	tests := []struct {
		status     int
		msg        string
		wantCode   string
		wantStatus int
	}{
		{404, "not found", "not_found", 404},
		{403, "forbidden", "forbidden", 403},
		{409, "conflict", "conflict", 409},
		{422, "invalid", "invalid_input", 422},
		{400, "bad request", "invalid_input", 400},
		{401, "unauthorized", "invalid_input", 401},
		{429, "rate limited", "invalid_input", 429},
		{500, "internal", "internal", 500},
		{502, "bad gateway", "internal", 502},
		{503, "unavailable", "internal", 503},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			err := errors.MapHTTPStatus(tt.status, tt.msg)
			if err.Code() != tt.wantCode {
				t.Errorf("Code() = %q, want %q", err.Code(), tt.wantCode)
			}
			if err.HTTPStatus() != tt.wantStatus {
				t.Errorf("HTTPStatus() = %d, want %d", err.HTTPStatus(), tt.wantStatus)
			}
			if err.Message() != tt.msg {
				t.Errorf("Message() = %q, want %q", err.Message(), tt.msg)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./errors/... -run TestMapHTTP`
Expected: Compilation failure — `MapHTTPStatus` not defined.

- [ ] **Step 3: Implement HTTP mapper**

Create `errors/http.go`:

```go
package errors

// MapHTTPStatus maps an HTTP status code from an upstream service
// to a semantic error.
func MapHTTPStatus(status int, msg string) *SemanticError {
	switch {
	case status == 404:
		return NewNotFound(msg)
	case status == 403:
		return NewForbidden(msg)
	case status == 409:
		return NewConflict(msg)
	case status == 422:
		return NewInvalidInput(msg)
	case status >= 500:
		return NewInternal(msg)
	case status >= 400:
		return newError("invalid_input", status, msg, nil)
	default:
		return NewInternal(msg)
	}
}
```

Note: for 4xx codes other than the specific ones mapped above, we preserve the original HTTP status code rather than forcing 422, so a 400 from upstream renders as 400, and a 401 as 401. The semantic code is still `invalid_input`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./errors/... -run TestMapHTTP -v`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add errors/http.go errors/http_test.go
git commit -m "Add HTTP status code mapper"
```

---

### Task 5: Huma Error Handler

**Files:**
- Create: `errors/huma.go`
- Create: `errors/huma_test.go`

- [ ] **Step 1: Write tests for Huma error handler**

Create `errors/huma_test.go`:

```go
package errors_test

import (
	"encoding/json"
	"testing"

	"github.com/mataki-dev/platform/errors"
)

func TestErrorBody_SemanticError(t *testing.T) {
	err := errors.NewNotFound("task not found")
	body := errors.NewErrorBody(err)
	data, _ := json.Marshal(body)

	var m map[string]any
	json.Unmarshal(data, &m)

	if m["type"] != "not_found" {
		t.Errorf("type = %q, want %q", m["type"], "not_found")
	}
	if m["message"] != "task not found" {
		t.Errorf("message = %q, want %q", m["message"], "task not found")
	}
	// errors array should be absent for non-validation errors
	if _, ok := m["errors"]; ok {
		t.Error("errors array should be absent for NotFound")
	}
}

func TestErrorBody_InvalidInputWithFieldErrors(t *testing.T) {
	fieldErrors := []errors.FieldError{
		{Field: "filter.status.eq", Code: "invalid_type", Message: "Expected string, got number."},
		{Field: "sort[0].field", Code: "unsortable_field", Message: "Field 'title' is not sortable."},
	}
	err := errors.NewInvalidInput("The search request contains invalid parameters.",
		errors.WithFieldErrors(fieldErrors...),
	)
	body := errors.NewErrorBody(err)
	data, _ := json.Marshal(body)

	var m map[string]any
	json.Unmarshal(data, &m)

	if m["type"] != "invalid_input" {
		t.Errorf("type = %q, want %q", m["type"], "invalid_input")
	}
	errs, ok := m["errors"].([]any)
	if !ok {
		t.Fatal("errors should be an array")
	}
	if len(errs) != 2 {
		t.Fatalf("errors len = %d, want 2", len(errs))
	}
	first := errs[0].(map[string]any)
	if first["field"] != "filter.status.eq" {
		t.Errorf("first error field = %q, want %q", first["field"], "filter.status.eq")
	}
	if first["code"] != "invalid_type" {
		t.Errorf("first error code = %q, want %q", first["code"], "invalid_type")
	}
}

func TestErrorBody_NonSemanticError(t *testing.T) {
	err := errors.NewErrorBody(nil)
	data, _ := json.Marshal(err)

	var m map[string]any
	json.Unmarshal(data, &m)

	if m["type"] != "internal" {
		t.Errorf("type = %q, want %q", m["type"], "internal")
	}
}

func TestHumaStatusError(t *testing.T) {
	err := errors.NewNotFound("task not found")
	humaErr := errors.ToHumaError(err)

	if humaErr.GetStatus() != 404 {
		t.Errorf("GetStatus() = %d, want %d", humaErr.GetStatus(), 404)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./errors/... -run "TestErrorBody|TestHumaStatus"`
Expected: Compilation failure.

- [ ] **Step 3: Implement Huma error handler**

Create `errors/huma.go`:

```go
package errors

import (
	"encoding/json"

	"github.com/danielgtaylor/huma/v2"
)

// FieldError represents a single field-level validation error
// in the Mataki error envelope.
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WithFieldErrors attaches field-level validation errors to an InvalidInput error.
func WithFieldErrors(errs ...FieldError) Option {
	return func(e *SemanticError) {
		if e.details == nil {
			e.details = make(map[string]any)
		}
		e.details["field_errors"] = errs
	}
}

// ErrorBody is the standard Mataki API error envelope.
type ErrorBody struct {
	Type    string       `json:"type"`
	Message string       `json:"message"`
	Errors  []FieldError `json:"errors,omitempty"`
}

// NewErrorBody creates an ErrorBody from a semantic Error.
// If err is nil, returns a generic internal error body.
func NewErrorBody(err *SemanticError) ErrorBody {
	if err == nil {
		return ErrorBody{Type: "internal", Message: "An unexpected error occurred."}
	}
	body := ErrorBody{
		Type:    err.Code(),
		Message: err.Message(),
	}
	if err.details != nil {
		if fe, ok := err.details["field_errors"].([]FieldError); ok {
			body.Errors = fe
		}
	}
	return body
}

// humaStatusError wraps a SemanticError to implement huma.StatusError.
type humaStatusError struct {
	err  *SemanticError
	body ErrorBody
}

func (e *humaStatusError) Error() string  { return e.err.Error() }
func (e *humaStatusError) GetStatus() int { return e.err.HTTPStatus() }

func (e *humaStatusError) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.body)
}

// Ensure humaStatusError implements huma.StatusError.
var _ huma.StatusError = (*humaStatusError)(nil)

// ToHumaError converts a SemanticError to a huma.StatusError for use
// in Huma handlers.
func ToHumaError(err *SemanticError) huma.StatusError {
	return &humaStatusError{
		err:  err,
		body: NewErrorBody(err),
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./errors/... -v`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add errors/huma.go errors/huma_test.go
git commit -m "Add Huma error handler with standard error envelope"
```

---

### Task 6: Search Schema and Types

**Files:**
- Create: `search/schema.go`
- Create: `search/types.go`
- Create: `search/validated.go`
- Create: `search/errors.go`

These are type definitions with no behavior — no tests needed until validation (Task 7).

- [ ] **Step 1: Create schema types**

Create `search/schema.go`:

```go
package search

// FieldType defines the data type of a searchable field.
type FieldType int

const (
	String    FieldType = iota
	Numeric             // int, float, decimal
	Timestamp           // RFC 3339
	Bool
	Enum // validated against AllowedValues
)

// FieldDef defines a single searchable field in a resource schema.
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

// FullTextConfig configures full-text search for a resource.
type FullTextConfig struct {
	// Column is the pre-computed tsvector column name.
	Column string

	// Language is the text search configuration for tsquery parsing.
	// Defaults to "english".
	Language string
}

// ResourceSchema defines the searchable fields and constraints for a resource.
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

	// MaxOffset is the ceiling for offset-based pagination.
	// Defaults to 10000.
	MaxOffset int

	// TableAlias is the SQL table alias used in generated fragments.
	TableAlias string

	// FullText configures full-text search via the "query" field.
	// When nil, the "query" field is rejected in requests.
	FullText *FullTextConfig
}

// isFieldSelectable returns whether a field can appear in the fields projection list.
func isFieldSelectable(f FieldDef) bool {
	if f.Selectable == nil {
		return true
	}
	return *f.Selectable
}

// resolveColumn returns the SQL column for a field. Defaults to the field key.
func resolveColumn(fieldKey string, f FieldDef) string {
	if f.Column != "" {
		return f.Column
	}
	return fieldKey
}

// defaultMaxInSize returns the effective max IN set size for a field.
func defaultMaxInSize(f FieldDef, schema ResourceSchema) int {
	if f.MaxInSize > 0 {
		return f.MaxInSize
	}
	if schema.MaxInSize > 0 {
		return schema.MaxInSize
	}
	return 100
}

// resolveFullTextLanguage returns the effective language config.
func resolveFullTextLanguage(ft *FullTextConfig) string {
	if ft.Language != "" {
		return ft.Language
	}
	return "english"
}
```

- [ ] **Step 2: Create request/response types**

Create `search/types.go`:

```go
package search

import "encoding/json"

// SearchRequest is the JSON body of a POST /{resource}/search endpoint.
type SearchRequest struct {
	Query  string                           `json:"query,omitempty"`
	Filter map[string]map[string]any        `json:"filter,omitempty"`
	Sort   []SortDirectiveInput             `json:"sort,omitempty"`
	Cursor string                           `json:"cursor,omitempty"`
	Offset *int                             `json:"offset,omitempty"`
	Limit  *int                             `json:"limit,omitempty"`
	Fields []string                         `json:"fields,omitempty"`
}

// SortDirectiveInput is the user-facing sort directive from the JSON body.
type SortDirectiveInput struct {
	Field     string `json:"field"`
	Direction string `json:"direction,omitempty"`
}

// SearchResponse is the standard response envelope for search endpoints.
type SearchResponse[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
	Limit      int    `json:"limit"`
}

// RawFilterValue holds an unparsed filter value from JSON.
// Used internally during validation to handle type coercion.
type RawFilterValue struct {
	json.RawMessage
}
```

- [ ] **Step 3: Create validated output types**

Create `search/validated.go`:

```go
package search

// ValidatedSearch is the output of Validate(). The SQL builder only
// accepts this type, enforcing parse-don't-validate.
type ValidatedSearch struct {
	Filters       []ValidatedFilter
	Sort          []SortDirective // includes tiebreaker
	Cursor        *DecodedCursor  // nil if not provided
	Offset        *int            // nil if not provided
	Limit         int
	Fields        []string // nil means all defaults
	Query         string   // non-empty if full-text search
	RelevanceSort bool     // true when sorting by ts_rank
}

// ValidatedFilter is a single filter clause after validation.
type ValidatedFilter struct {
	Field    string
	Column   string // resolved SQL column name
	Operator string
	Value    any    // parsed to correct Go type
}

// SortDirective is a validated sort directive with resolved column.
type SortDirective struct {
	Field  string  `json:"field"`
	Column string  `json:"-"`
	Dir    SortDir `json:"direction"`
}

// SortDir is the sort direction.
type SortDir string

const (
	Asc  SortDir = "asc"
	Desc SortDir = "desc"
)
```

- [ ] **Step 4: Create search error codes**

Create `search/errors.go`:

```go
package search

// ValidationError represents a single validation issue in a search request.
type ValidationError struct {
	// Field is the dotted path to the invalid element.
	// e.g., "filter.status.eq", "sort[1].field", "limit"
	Field   string `json:"field"`

	// Code is a machine-readable error code.
	Code    string `json:"code"`

	// Message is a human-readable description.
	Message string `json:"message"`
}

// Stable error codes.
const (
	ErrUnknownField     = "unknown_field"
	ErrInvalidOperator  = "invalid_operator"
	ErrInvalidValue     = "invalid_value"
	ErrInvalidType      = "invalid_type"
	ErrExceedsMax       = "exceeds_max"
	ErrInvalidCursor    = "invalid_cursor"
	ErrUnsortableField  = "unsortable_field"
	ErrDuplicateSortField = "duplicate_sort_field"
	ErrNotNullable      = "not_nullable"
	ErrUnsupportedField = "unsupported_field"
	ErrMutuallyExclusive = "mutually_exclusive"
)
```

- [ ] **Step 5: Verify compilation**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go build ./search/...`
Expected: Compilation succeeds. (The `DecodedCursor` type doesn't exist yet — it will be created in Task 9. For now, change `validated.go` to use `any` as a placeholder: `Cursor any` — we'll update it in Task 9.)

Note: Before compiling, temporarily set `Cursor` field type to `any` in `validated.go`. Task 9 will define `DecodedCursor` and update this.

- [ ] **Step 6: Commit**

```bash
git add search/schema.go search/types.go search/validated.go search/errors.go
git commit -m "Add search schema, request/response types, and validation error codes"
```

---

### Task 7: Search Validation

**Files:**
- Create: `search/validate.go`
- Create: `search/validate_test.go`

This is the largest task. Validation covers: filters (field existence, operator+type compatibility, value parsing, enum checking, in/nin size, is_null on nullable), sort (sortable, direction, max 3, no duplicates), limit (clamped), fields (selectable), query (requires FullText config), cursor+offset mutual exclusion, and cursor+relevance incompatibility.

- [ ] **Step 1: Write tests for filter validation**

Create `search/validate_test.go`:

```go
package search_test

import (
	"testing"

	"github.com/mataki-dev/platform/search"
)

// testSchema is a shared schema for validation tests.
var testSchema = search.ResourceSchema{
	Fields: map[string]search.FieldDef{
		"status": {
			Type:          search.Enum,
			Operators:     []string{"eq", "neq", "in"},
			Sortable:      true,
			AllowedValues: []string{"open", "in_progress", "done"},
		},
		"priority": {
			Type:     search.Numeric,
			Sortable: true,
		},
		"title": {
			Type:      search.String,
			Operators: []string{"contains"},
		},
		"created_at": {
			Type:     search.Timestamp,
			Sortable: true,
		},
		"assignee_id": {
			Type:     search.String,
			Nullable: true,
		},
		"is_active": {
			Type: search.Bool,
		},
	},
	MaxLimit:     100,
	DefaultLimit: 25,
	DefaultSort:  []search.SortDirective{{Field: "created_at", Column: "created_at", Dir: search.Desc}},
	PrimaryKey:   "id",
	TableAlias:   "t",
	MaxInSize:    100,
}

var testSchemaWithFTS = func() search.ResourceSchema {
	s := testSchema
	s.FullText = &search.FullTextConfig{Column: "search_vector", Language: "english"}
	return s
}()

func TestValidate_EmptyRequest(t *testing.T) {
	vs, errs := search.Validate(search.SearchRequest{}, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Limit != 25 {
		t.Errorf("Limit = %d, want 25", vs.Limit)
	}
	if len(vs.Sort) == 0 {
		t.Fatal("expected default sort")
	}
	// Default sort + tiebreaker
	if vs.Sort[0].Field != "created_at" || vs.Sort[0].Dir != search.Desc {
		t.Errorf("default sort = %+v, want created_at desc", vs.Sort[0])
	}
}

func TestValidate_FilterUnknownField(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"nonexistent": {"eq": "foo"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.nonexistent", search.ErrUnknownField)
}

func TestValidate_FilterInvalidOperatorForType(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"status": {"gt": "open"}, // gt not valid for Enum
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.status.gt", search.ErrInvalidOperator)
}

func TestValidate_FilterOperatorNotInAllowedList(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"title": {"eq": "hello"}, // title only allows "contains"
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.title.eq", search.ErrInvalidOperator)
}

func TestValidate_FilterContainsOnNumeric(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"priority": {"contains": "high"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.priority.contains", search.ErrInvalidOperator)
}

func TestValidate_FilterGtOnString(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"title": {"gt": "abc"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.title.gt", search.ErrInvalidOperator)
}

func TestValidate_FilterIsNullOnNonNullable(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"status": {"is_null": true},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.status.is_null", search.ErrNotNullable)
}

func TestValidate_FilterIsNullOnNullableField(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"assignee_id": {"is_null": true},
		},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(vs.Filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(vs.Filters))
	}
	if vs.Filters[0].Operator != "is_null" {
		t.Errorf("operator = %q, want %q", vs.Filters[0].Operator, "is_null")
	}
}

func TestValidate_FilterIsNullNonBoolValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"assignee_id": {"is_null": "yes"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.assignee_id.is_null", search.ErrInvalidType)
}

func TestValidate_FilterEnumInvalidValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"status": {"eq": "invalid_status"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.status.eq", search.ErrInvalidValue)
}

func TestValidate_FilterEnumValidValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"status": {"eq": "open"},
		},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(vs.Filters) != 1 {
		t.Fatal("expected 1 filter")
	}
	if vs.Filters[0].Value != "open" {
		t.Errorf("value = %v, want %q", vs.Filters[0].Value, "open")
	}
}

func TestValidate_FilterTimestampInvalidValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"created_at": {"gte": "not-a-timestamp"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.created_at.gte", search.ErrInvalidType)
}

func TestValidate_FilterTimestampValidValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"created_at": {"gte": "2026-01-01T00:00:00Z"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_FilterNumericInvalidValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"priority": {"eq": "not-a-number"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.priority.eq", search.ErrInvalidType)
}

func TestValidate_FilterNumericValidValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"priority": {"gte": float64(5)},
		},
	}
	_, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_FilterMultipleOperatorsOnSameField(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"created_at": {"gte": "2026-01-01T00:00:00Z", "lt": "2026-04-01T00:00:00Z"},
		},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(vs.Filters) != 2 {
		t.Errorf("expected 2 filters, got %d", len(vs.Filters))
	}
}

func TestValidate_FilterInExceedsMaxSize(t *testing.T) {
	vals := make([]any, 101)
	for i := range vals {
		vals[i] = "val"
	}
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"status": {"in": vals},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.status.in", search.ErrExceedsMax)
}

func TestValidate_SortUnsortableField(t *testing.T) {
	req := search.SearchRequest{
		Sort: []search.SortDirectiveInput{
			{Field: "title", Direction: "asc"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "sort[0].field", search.ErrUnsortableField)
}

func TestValidate_SortInvalidDirection(t *testing.T) {
	req := search.SearchRequest{
		Sort: []search.SortDirectiveInput{
			{Field: "created_at", Direction: "sideways"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "sort[0].direction", search.ErrInvalidValue)
}

func TestValidate_SortDuplicateField(t *testing.T) {
	req := search.SearchRequest{
		Sort: []search.SortDirectiveInput{
			{Field: "created_at", Direction: "asc"},
			{Field: "created_at", Direction: "desc"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "sort[1].field", search.ErrDuplicateSortField)
}

func TestValidate_SortMaxDirectives(t *testing.T) {
	req := search.SearchRequest{
		Sort: []search.SortDirectiveInput{
			{Field: "created_at"},
			{Field: "priority"},
			{Field: "status"},
			{Field: "created_at"}, // 4th — over limit
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "sort", search.ErrExceedsMax)
}

func TestValidate_SortDefaultDirection(t *testing.T) {
	req := search.SearchRequest{
		Sort: []search.SortDirectiveInput{
			{Field: "created_at"}, // no direction specified
		},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Sort[0].Dir != search.Asc {
		t.Errorf("Dir = %q, want %q", vs.Sort[0].Dir, search.Asc)
	}
}

func TestValidate_SortAppendsTiebreaker(t *testing.T) {
	req := search.SearchRequest{
		Sort: []search.SortDirectiveInput{
			{Field: "created_at", Direction: "desc"},
		},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(vs.Sort) != 2 {
		t.Fatalf("expected 2 sort directives (incl tiebreaker), got %d", len(vs.Sort))
	}
	last := vs.Sort[len(vs.Sort)-1]
	if last.Field != "id" || last.Dir != search.Asc {
		t.Errorf("tiebreaker = %+v, want id asc", last)
	}
}

func TestValidate_LimitDefaults(t *testing.T) {
	req := search.SearchRequest{}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Limit != 25 {
		t.Errorf("Limit = %d, want 25", vs.Limit)
	}
}

func TestValidate_LimitClampedToMax(t *testing.T) {
	limit := 999
	req := search.SearchRequest{Limit: &limit}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Limit != 100 {
		t.Errorf("Limit = %d, want 100 (clamped)", vs.Limit)
	}
}

func TestValidate_LimitClampedToMin(t *testing.T) {
	limit := 0
	req := search.SearchRequest{Limit: &limit}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Limit != 1 {
		t.Errorf("Limit = %d, want 1 (clamped)", vs.Limit)
	}
}

func TestValidate_FieldsInvalid(t *testing.T) {
	req := search.SearchRequest{
		Fields: []string{"id", "nonexistent"},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "fields[1]", search.ErrUnknownField)
}

func TestValidate_FieldsValid(t *testing.T) {
	req := search.SearchRequest{
		Fields: []string{"status", "created_at"},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(vs.Fields) != 2 {
		t.Errorf("Fields len = %d, want 2", len(vs.Fields))
	}
}

func TestValidate_QueryWithoutFullTextConfig(t *testing.T) {
	req := search.SearchRequest{Query: "hello"}
	_, errs := search.Validate(req, testSchema) // testSchema has no FullText
	assertHasError(t, errs, "query", search.ErrUnsupportedField)
}

func TestValidate_QueryWithFullTextConfig(t *testing.T) {
	req := search.SearchRequest{Query: "hello"}
	vs, errs := search.Validate(req, testSchemaWithFTS)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Query != "hello" {
		t.Errorf("Query = %q, want %q", vs.Query, "hello")
	}
	if !vs.RelevanceSort {
		t.Error("expected RelevanceSort=true when query present and no explicit sort")
	}
}

func TestValidate_QueryWithExplicitSort(t *testing.T) {
	req := search.SearchRequest{
		Query: "hello",
		Sort:  []search.SortDirectiveInput{{Field: "created_at", Direction: "desc"}},
	}
	vs, errs := search.Validate(req, testSchemaWithFTS)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.RelevanceSort {
		t.Error("expected RelevanceSort=false when explicit sort provided")
	}
}

func TestValidate_CursorAndOffsetMutuallyExclusive(t *testing.T) {
	offset := 10
	req := search.SearchRequest{
		Cursor: "some-cursor",
		Offset: &offset,
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "cursor", search.ErrMutuallyExclusive)
}

func TestValidate_CursorWithRelevanceSortRejected(t *testing.T) {
	req := search.SearchRequest{
		Query:  "hello",
		Cursor: "some-cursor",
	}
	_, errs := search.Validate(req, testSchemaWithFTS)
	assertHasError(t, errs, "cursor", search.ErrInvalidCursor)
}

func TestValidate_MultipleErrors(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"nonexistent": {"eq": "foo"},
			"status":      {"gt": "open"},
		},
		Sort: []search.SortDirectiveInput{
			{Field: "title"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	if len(errs) < 3 {
		t.Errorf("expected at least 3 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidate_BoolEqValid(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"is_active": {"eq": true},
		},
	}
	_, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_BoolInvalidOperator(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"is_active": {"gt": true},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.is_active.gt", search.ErrInvalidOperator)
}

// assertHasError checks that at least one validation error matches
// the given field and code.
func assertHasError(t *testing.T, errs []search.ValidationError, field, code string) {
	t.Helper()
	for _, e := range errs {
		if e.Field == field && e.Code == code {
			return
		}
	}
	t.Errorf("expected error with field=%q code=%q, got %v", field, code, errs)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./search/... -run TestValidate`
Expected: Compilation failure — `Validate` not defined.

- [ ] **Step 3: Implement validation**

Create `search/validate.go`:

```go
package search

import (
	"fmt"
	"slices"
	"strconv"
	"time"
)

// operatorsForType returns the set of operators valid for a given field type.
func operatorsForType(ft FieldType) []string {
	switch ft {
	case String:
		return []string{"eq", "neq", "in", "nin", "contains", "is_null"}
	case Numeric:
		return []string{"eq", "neq", "gt", "gte", "lt", "lte", "in", "nin", "is_null"}
	case Timestamp:
		return []string{"eq", "neq", "gt", "gte", "lt", "lte", "is_null"}
	case Bool:
		return []string{"eq", "neq", "is_null"}
	case Enum:
		return []string{"eq", "neq", "in", "nin", "is_null"}
	default:
		return nil
	}
}

// Validate validates a SearchRequest against a ResourceSchema.
// Validation is exhaustive — all errors are collected, not fail-fast.
func Validate(req SearchRequest, schema ResourceSchema) (ValidatedSearch, []ValidationError) {
	var errs []ValidationError
	vs := ValidatedSearch{}

	// --- Query ---
	if req.Query != "" {
		if schema.FullText == nil {
			errs = append(errs, ValidationError{
				Field:   "query",
				Code:    ErrUnsupportedField,
				Message: "This resource does not support full-text search.",
			})
		} else {
			vs.Query = req.Query
		}
	}

	// --- Cursor + Offset mutual exclusion ---
	if req.Cursor != "" && req.Offset != nil {
		errs = append(errs, ValidationError{
			Field:   "cursor",
			Code:    ErrMutuallyExclusive,
			Message: "cursor and offset are mutually exclusive.",
		})
	}

	// --- Filters ---
	for fieldName, ops := range req.Filter {
		fieldDef, ok := schema.Fields[fieldName]
		if !ok {
			errs = append(errs, ValidationError{
				Field:   "filter." + fieldName,
				Code:    ErrUnknownField,
				Message: fmt.Sprintf("Unknown field %q.", fieldName),
			})
			continue
		}

		for opName, rawVal := range ops {
			fieldPath := "filter." + fieldName + "." + opName

			// Check operator is valid for the type
			typeOps := operatorsForType(fieldDef.Type)
			if !slices.Contains(typeOps, opName) {
				errs = append(errs, ValidationError{
					Field:   fieldPath,
					Code:    ErrInvalidOperator,
					Message: fmt.Sprintf("Operator %q is not valid for type %v.", opName, fieldDef.Type),
				})
				continue
			}

			// Check operator is in field's allowed list (if specified)
			if len(fieldDef.Operators) > 0 && !slices.Contains(fieldDef.Operators, opName) {
				errs = append(errs, ValidationError{
					Field:   fieldPath,
					Code:    ErrInvalidOperator,
					Message: fmt.Sprintf("Operator %q is not permitted on field %q.", opName, fieldName),
				})
				continue
			}

			// is_null special handling
			if opName == "is_null" {
				if !fieldDef.Nullable {
					errs = append(errs, ValidationError{
						Field:   fieldPath,
						Code:    ErrNotNullable,
						Message: fmt.Sprintf("Field %q is not nullable.", fieldName),
					})
					continue
				}
				boolVal, ok := rawVal.(bool)
				if !ok {
					errs = append(errs, ValidationError{
						Field:   fieldPath,
						Code:    ErrInvalidType,
						Message: "is_null value must be a boolean.",
					})
					continue
				}
				vs.Filters = append(vs.Filters, ValidatedFilter{
					Field:    fieldName,
					Column:   resolveColumn(fieldName, fieldDef),
					Operator: opName,
					Value:    boolVal,
				})
				continue
			}

			// in/nin: validate array and size
			if opName == "in" || opName == "nin" {
				arr, ok := rawVal.([]any)
				if !ok {
					errs = append(errs, ValidationError{
						Field:   fieldPath,
						Code:    ErrInvalidType,
						Message: fmt.Sprintf("Operator %q requires an array value.", opName),
					})
					continue
				}
				maxSize := defaultMaxInSize(fieldDef, schema)
				if len(arr) > maxSize {
					errs = append(errs, ValidationError{
						Field:   fieldPath,
						Code:    ErrExceedsMax,
						Message: fmt.Sprintf("Array size %d exceeds maximum %d.", len(arr), maxSize),
					})
					continue
				}
				// Validate each element
				parsedVals := make([]any, 0, len(arr))
				valid := true
				for _, elem := range arr {
					parsed, err := parseValue(fieldDef, elem)
					if err != nil {
						errs = append(errs, ValidationError{
							Field:   fieldPath,
							Code:    ErrInvalidType,
							Message: err.Error(),
						})
						valid = false
						break
					}
					// Enum validation per element
					if fieldDef.Type == Enum {
						s, _ := parsed.(string)
						if !slices.Contains(fieldDef.AllowedValues, s) {
							errs = append(errs, ValidationError{
								Field:   fieldPath,
								Code:    ErrInvalidValue,
								Message: fmt.Sprintf("Value %q is not one of the allowed values.", s),
							})
							valid = false
							break
						}
					}
					parsedVals = append(parsedVals, parsed)
				}
				if !valid {
					continue
				}
				vs.Filters = append(vs.Filters, ValidatedFilter{
					Field:    fieldName,
					Column:   resolveColumn(fieldName, fieldDef),
					Operator: opName,
					Value:    parsedVals,
				})
				continue
			}

			// Scalar operators: parse and validate value
			parsed, err := parseValue(fieldDef, rawVal)
			if err != nil {
				errs = append(errs, ValidationError{
					Field:   fieldPath,
					Code:    ErrInvalidType,
					Message: err.Error(),
				})
				continue
			}

			// Enum value validation
			if fieldDef.Type == Enum {
				s, _ := parsed.(string)
				if !slices.Contains(fieldDef.AllowedValues, s) {
					errs = append(errs, ValidationError{
						Field:   fieldPath,
						Code:    ErrInvalidValue,
						Message: fmt.Sprintf("Value %q is not one of the allowed values.", s),
					})
					continue
				}
			}

			vs.Filters = append(vs.Filters, ValidatedFilter{
				Field:    fieldName,
				Column:   resolveColumn(fieldName, fieldDef),
				Operator: opName,
				Value:    parsed,
			})
		}
	}

	// --- Sort ---
	if len(req.Sort) > 3 {
		errs = append(errs, ValidationError{
			Field:   "sort",
			Code:    ErrExceedsMax,
			Message: "Maximum 3 sort directives allowed.",
		})
	} else {
		seenSortFields := make(map[string]bool)
		for i, sd := range req.Sort {
			fieldDef, ok := schema.Fields[sd.Field]
			if !ok {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("sort[%d].field", i),
					Code:    ErrUnknownField,
					Message: fmt.Sprintf("Unknown field %q.", sd.Field),
				})
				continue
			}
			if !fieldDef.Sortable {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("sort[%d].field", i),
					Code:    ErrUnsortableField,
					Message: fmt.Sprintf("Field %q is not sortable.", sd.Field),
				})
				continue
			}
			if seenSortFields[sd.Field] {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("sort[%d].field", i),
					Code:    ErrDuplicateSortField,
					Message: fmt.Sprintf("Duplicate sort field %q.", sd.Field),
				})
				continue
			}

			dir := SortDir(sd.Direction)
			if sd.Direction == "" {
				dir = Asc
			} else if dir != Asc && dir != Desc {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("sort[%d].direction", i),
					Code:    ErrInvalidValue,
					Message: fmt.Sprintf("Direction must be %q or %q.", Asc, Desc),
				})
				continue
			}

			seenSortFields[sd.Field] = true
			vs.Sort = append(vs.Sort, SortDirective{
				Field:  sd.Field,
				Column: resolveColumn(sd.Field, fieldDef),
				Dir:    dir,
			})
		}
	}

	// Apply default sort if none provided
	if len(vs.Sort) == 0 && len(req.Sort) == 0 {
		if req.Query != "" && schema.FullText != nil {
			// Relevance sort — handled by builder
			vs.RelevanceSort = true
		} else {
			vs.Sort = append(vs.Sort, schema.DefaultSort...)
		}
	}

	// Append tiebreaker if not already present
	pk := schema.PrimaryKey
	if pk == "" {
		pk = "id"
	}
	if !vs.RelevanceSort {
		hasPK := false
		for _, s := range vs.Sort {
			if s.Field == pk {
				hasPK = true
				break
			}
		}
		if !hasPK {
			vs.Sort = append(vs.Sort, SortDirective{
				Field:  pk,
				Column: pk,
				Dir:    Asc,
			})
		}
	}

	// --- Cursor vs relevance sort ---
	if req.Cursor != "" && vs.RelevanceSort {
		errs = append(errs, ValidationError{
			Field:   "cursor",
			Code:    ErrInvalidCursor,
			Message: "Cursor pagination is not supported with relevance sorting. Use offset instead.",
		})
	}

	// --- Limit ---
	if req.Limit != nil {
		vs.Limit = *req.Limit
	} else {
		vs.Limit = schema.DefaultLimit
	}
	if vs.Limit < 1 {
		vs.Limit = 1
	}
	if schema.MaxLimit > 0 && vs.Limit > schema.MaxLimit {
		vs.Limit = schema.MaxLimit
	}

	// --- Offset ---
	if req.Offset != nil {
		offset := *req.Offset
		maxOffset := schema.MaxOffset
		if maxOffset == 0 {
			maxOffset = 10000
		}
		if offset < 0 {
			offset = 0
		}
		if offset > maxOffset {
			offset = maxOffset
		}
		vs.Offset = &offset
	}

	// --- Fields ---
	for i, f := range req.Fields {
		// "id" is always included
		if f == "id" {
			continue
		}
		fieldDef, ok := schema.Fields[f]
		if !ok {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("fields[%d]", i),
				Code:    ErrUnknownField,
				Message: fmt.Sprintf("Unknown field %q.", f),
			})
			continue
		}
		if !isFieldSelectable(fieldDef) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("fields[%d]", i),
				Code:    ErrInvalidValue,
				Message: fmt.Sprintf("Field %q is not selectable.", f),
			})
			continue
		}
	}
	if len(req.Fields) > 0 && len(errs) == 0 {
		vs.Fields = req.Fields
	}

	return vs, errs
}

// parseValue parses a raw JSON value into the correct Go type for the field.
func parseValue(field FieldDef, raw any) (any, error) {
	switch field.Type {
	case String, Enum:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("expected string value, got %T", raw)
		}
		return s, nil
	case Numeric:
		switch v := raw.(type) {
		case float64:
			return v, nil
		case int:
			return float64(v), nil
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("expected numeric value, got %q", v)
			}
			return f, nil
		default:
			return nil, fmt.Errorf("expected numeric value, got %T", raw)
		}
	case Timestamp:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("expected RFC 3339 timestamp string, got %T", raw)
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("expected RFC 3339 timestamp, got %q", s)
		}
		return t, nil
	case Bool:
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("expected boolean value, got %T", raw)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("unsupported field type %d", field.Type)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./search/... -run TestValidate -v`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add search/validate.go search/validate_test.go
git commit -m "Add search request validation with exhaustive error collection"
```

---

### Task 8: SQL Builder

**Files:**
- Create: `search/builder.go`
- Create: `search/builder_test.go`

- [ ] **Step 1: Write tests for SQL builder**

Create `search/builder_test.go`:

```go
package search_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mataki-dev/platform/search"
)

func TestBuildSQL_NoFilters(t *testing.T) {
	vs := search.ValidatedSearch{
		Sort:  []search.SortDirective{{Field: "created_at", Column: "created_at", Dir: search.Desc}, {Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 25,
	}
	frag := search.BuildSQL(vs, 1)

	if frag.Where != "" {
		t.Errorf("Where = %q, want empty", frag.Where)
	}
	if frag.OrderBy != "t.created_at DESC, t.id ASC" {
		t.Errorf("OrderBy = %q, want %q", frag.OrderBy, "t.created_at DESC, t.id ASC")
	}
	if frag.Limit != 26 { // limit+1
		t.Errorf("Limit = %d, want 26", frag.Limit)
	}
	if len(frag.Args) != 0 {
		t.Errorf("Args len = %d, want 0", len(frag.Args))
	}
}

func TestBuildSQL_SingleEqFilter(t *testing.T) {
	vs := search.ValidatedSearch{
		Filters: []search.ValidatedFilter{
			{Field: "status", Column: "status", Operator: "eq", Value: "open"},
		},
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 25,
	}
	frag := search.BuildSQL(vs, 2) // $1 is tenant_id

	if frag.Where != "AND t.status = $2" {
		t.Errorf("Where = %q, want %q", frag.Where, "AND t.status = $2")
	}
	if len(frag.Args) != 1 {
		t.Fatalf("Args len = %d, want 1", len(frag.Args))
	}
	if frag.Args[0] != "open" {
		t.Errorf("Args[0] = %v, want %q", frag.Args[0], "open")
	}
	if frag.ArgCount != 1 {
		t.Errorf("ArgCount = %d, want 1", frag.ArgCount)
	}
}

func TestBuildSQL_MultipleFilters(t *testing.T) {
	ts, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	vs := search.ValidatedSearch{
		Filters: []search.ValidatedFilter{
			{Field: "status", Column: "status", Operator: "eq", Value: "open"},
			{Field: "created_at", Column: "created_at", Operator: "gte", Value: ts},
		},
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 25,
	}
	frag := search.BuildSQL(vs, 1)

	if !strings.Contains(frag.Where, "AND t.status = $1") {
		t.Errorf("Where missing status filter: %q", frag.Where)
	}
	if !strings.Contains(frag.Where, "AND t.created_at >= $2") {
		t.Errorf("Where missing created_at filter: %q", frag.Where)
	}
	if len(frag.Args) != 2 {
		t.Errorf("Args len = %d, want 2", len(frag.Args))
	}
}

func TestBuildSQL_AllOperators(t *testing.T) {
	tests := []struct {
		op       string
		value    any
		wantSQL  string
		wantArgs int
	}{
		{"eq", "val", "AND t.col = $1", 1},
		{"neq", "val", "AND t.col != $1", 1},
		{"gt", float64(5), "AND t.col > $1", 1},
		{"gte", float64(5), "AND t.col >= $1", 1},
		{"lt", float64(5), "AND t.col < $1", 1},
		{"lte", float64(5), "AND t.col <= $1", 1},
		{"in", []any{"a", "b"}, "AND t.col = ANY($1)", 1},
		{"nin", []any{"a", "b"}, "AND t.col != ALL($1)", 1},
		{"contains", "test", "AND t.col ILIKE $1", 1},
	}

	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			vs := search.ValidatedSearch{
				Filters: []search.ValidatedFilter{
					{Field: "col", Column: "col", Operator: tt.op, Value: tt.value},
				},
				Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
				Limit: 10,
			}
			frag := search.BuildSQL(vs, 1)
			if frag.Where != tt.wantSQL {
				t.Errorf("Where = %q, want %q", frag.Where, tt.wantSQL)
			}
			if len(frag.Args) != tt.wantArgs {
				t.Errorf("Args len = %d, want %d", len(frag.Args), tt.wantArgs)
			}
		})
	}
}

func TestBuildSQL_IsNull(t *testing.T) {
	vs := search.ValidatedSearch{
		Filters: []search.ValidatedFilter{
			{Field: "assignee", Column: "assignee_id", Operator: "is_null", Value: true},
		},
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 10,
	}
	frag := search.BuildSQL(vs, 1)
	if frag.Where != "AND t.assignee_id IS NULL" {
		t.Errorf("Where = %q, want %q", frag.Where, "AND t.assignee_id IS NULL")
	}
	if len(frag.Args) != 0 {
		t.Errorf("Args len = %d, want 0 (is_null uses no param)", len(frag.Args))
	}
}

func TestBuildSQL_IsNotNull(t *testing.T) {
	vs := search.ValidatedSearch{
		Filters: []search.ValidatedFilter{
			{Field: "assignee", Column: "assignee_id", Operator: "is_null", Value: false},
		},
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 10,
	}
	frag := search.BuildSQL(vs, 1)
	if frag.Where != "AND t.assignee_id IS NOT NULL" {
		t.Errorf("Where = %q, want %q", frag.Where, "AND t.assignee_id IS NOT NULL")
	}
}

func TestBuildSQL_ContainsEscaping(t *testing.T) {
	vs := search.ValidatedSearch{
		Filters: []search.ValidatedFilter{
			{Field: "title", Column: "title", Operator: "contains", Value: "100% off_sale\\deal"},
		},
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 10,
	}
	frag := search.BuildSQL(vs, 1)
	// The value should have %, _, \ escaped
	arg := frag.Args[0].(string)
	expected := "%100\\% off\\_sale\\\\deal%"
	if arg != expected {
		t.Errorf("contains arg = %q, want %q", arg, expected)
	}
}

func TestBuildSQL_OrderByMultipleDirections(t *testing.T) {
	vs := search.ValidatedSearch{
		Sort: []search.SortDirective{
			{Field: "created_at", Column: "created_at", Dir: search.Desc},
			{Field: "priority", Column: "priority", Dir: search.Asc},
			{Field: "id", Column: "id", Dir: search.Asc},
		},
		Limit: 25,
	}
	frag := search.BuildSQL(vs, 1)
	if frag.OrderBy != "t.created_at DESC, t.priority ASC, t.id ASC" {
		t.Errorf("OrderBy = %q", frag.OrderBy)
	}
}

func TestBuildSQL_LimitPlusOne(t *testing.T) {
	vs := search.ValidatedSearch{
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 10,
	}
	frag := search.BuildSQL(vs, 1)
	if frag.Limit != 11 {
		t.Errorf("Limit = %d, want 11 (limit+1 for has_more)", frag.Limit)
	}
}

func TestBuildSQL_FullTextWhere(t *testing.T) {
	ft := &search.FullTextConfig{Column: "search_vector", Language: "english"}
	vs := search.ValidatedSearch{
		Query:         "hello world",
		RelevanceSort: true,
		Limit:         25,
	}
	frag := search.BuildSQLWithFullText(vs, 1, ft)

	if !strings.Contains(frag.Where, "t.search_vector @@ plainto_tsquery('english', $1)") {
		t.Errorf("Where missing tsquery clause: %q", frag.Where)
	}
	if frag.Args[0] != "hello world" {
		t.Errorf("Args[0] = %v, want %q", frag.Args[0], "hello world")
	}
}

func TestBuildSQL_FullTextRelevanceOrder(t *testing.T) {
	ft := &search.FullTextConfig{Column: "search_vector", Language: "english"}
	vs := search.ValidatedSearch{
		Query:         "hello",
		RelevanceSort: true,
		Limit:         25,
	}
	frag := search.BuildSQLWithFullText(vs, 1, ft)

	if !strings.Contains(frag.OrderBy, "ts_rank(t.search_vector, plainto_tsquery('english', $") {
		t.Errorf("OrderBy missing ts_rank: %q", frag.OrderBy)
	}
	if !strings.HasSuffix(frag.OrderBy, " DESC, t.id ASC") {
		t.Errorf("OrderBy missing id tiebreaker: %q", frag.OrderBy)
	}
}

func TestBuildSQL_FullTextWithExplicitSort(t *testing.T) {
	ft := &search.FullTextConfig{Column: "search_vector", Language: "english"}
	vs := search.ValidatedSearch{
		Query:         "hello",
		RelevanceSort: false, // explicit sort overrides relevance
		Sort:          []search.SortDirective{{Field: "created_at", Column: "created_at", Dir: search.Desc}, {Field: "id", Column: "id", Dir: search.Asc}},
		Limit:         25,
	}
	frag := search.BuildSQLWithFullText(vs, 1, ft)

	// Should still have the tsquery WHERE clause
	if !strings.Contains(frag.Where, "t.search_vector @@ plainto_tsquery") {
		t.Errorf("Where missing tsquery clause: %q", frag.Where)
	}
	// Should NOT have ts_rank in ORDER BY
	if strings.Contains(frag.OrderBy, "ts_rank") {
		t.Errorf("OrderBy should not contain ts_rank when explicit sort: %q", frag.OrderBy)
	}
	if frag.OrderBy != "t.created_at DESC, t.id ASC" {
		t.Errorf("OrderBy = %q, want %q", frag.OrderBy, "t.created_at DESC, t.id ASC")
	}
}

func TestBuildSQL_Offset(t *testing.T) {
	offset := 50
	vs := search.ValidatedSearch{
		Sort:   []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit:  25,
		Offset: &offset,
	}
	frag := search.BuildSQL(vs, 1)
	if frag.OffsetClause != "OFFSET 50" {
		t.Errorf("OffsetClause = %q, want %q", frag.OffsetClause, "OFFSET 50")
	}
}

func TestBuildSQL_CustomTableAlias(t *testing.T) {
	vs := search.ValidatedSearch{
		Filters: []search.ValidatedFilter{
			{Field: "status", Column: "status", Operator: "eq", Value: "open"},
		},
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 10,
	}
	frag := search.BuildSQLWithAlias(vs, 1, "tbl")
	if frag.Where != "AND tbl.status = $1" {
		t.Errorf("Where = %q, want %q", frag.Where, "AND tbl.status = $1")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./search/... -run TestBuildSQL`
Expected: Compilation failure — `BuildSQL` not defined.

- [ ] **Step 3: Implement SQL builder**

Create `search/builder.go`:

```go
package search

import (
	"fmt"
	"strings"
)

// SQLFragment contains the generated SQL clauses and parameters
// for a search query.
type SQLFragment struct {
	// Where is the AND-joined clause string, including leading "AND"
	// per filter. Empty string if no filters.
	Where string

	// CursorWhere is the cursor seek clause, including leading "AND".
	// Empty if no cursor.
	CursorWhere string

	// OrderBy is the comma-separated ORDER BY expression.
	OrderBy string

	// OffsetClause is "OFFSET N" when offset pagination is used.
	// Empty string if no offset.
	OffsetClause string

	// Limit is the resolved limit + 1 (for has_more detection).
	Limit int

	// Args contains the parameter values in positional order.
	Args []any

	// ArgCount is the total number of positional parameters.
	ArgCount int
}

// BuildSQL generates SQL clauses from a ValidatedSearch.
// argOffset is the starting positional parameter index ($argOffset).
// Uses "t" as the default table alias.
func BuildSQL(vs ValidatedSearch, argOffset int) SQLFragment {
	return buildSQL(vs, argOffset, "t", nil)
}

// BuildSQLWithAlias generates SQL clauses with a custom table alias.
func BuildSQLWithAlias(vs ValidatedSearch, argOffset int, alias string) SQLFragment {
	return buildSQL(vs, argOffset, alias, nil)
}

// BuildSQLWithFullText generates SQL clauses including full-text search support.
func BuildSQLWithFullText(vs ValidatedSearch, argOffset int, ft *FullTextConfig) SQLFragment {
	return buildSQL(vs, argOffset, "t", ft)
}

func buildSQL(vs ValidatedSearch, argOffset int, alias string, ft *FullTextConfig) SQLFragment {
	var frag SQLFragment
	var whereParts []string
	paramIdx := argOffset

	// Full-text search WHERE clause
	if vs.Query != "" && ft != nil {
		lang := resolveFullTextLanguage(ft)
		whereParts = append(whereParts,
			fmt.Sprintf("%s.%s @@ plainto_tsquery('%s', $%d)", alias, ft.Column, lang, paramIdx))
		frag.Args = append(frag.Args, vs.Query)
		paramIdx++
	}

	// Filter clauses
	for _, f := range vs.Filters {
		col := fmt.Sprintf("%s.%s", alias, f.Column)

		switch f.Operator {
		case "is_null":
			if f.Value.(bool) {
				whereParts = append(whereParts, col+" IS NULL")
			} else {
				whereParts = append(whereParts, col+" IS NOT NULL")
			}
			// No parameter for is_null

		case "contains":
			whereParts = append(whereParts, fmt.Sprintf("%s ILIKE $%d", col, paramIdx))
			frag.Args = append(frag.Args, escapeContains(f.Value.(string)))
			paramIdx++

		case "in":
			whereParts = append(whereParts, fmt.Sprintf("%s = ANY($%d)", col, paramIdx))
			frag.Args = append(frag.Args, f.Value)
			paramIdx++

		case "nin":
			whereParts = append(whereParts, fmt.Sprintf("%s != ALL($%d)", col, paramIdx))
			frag.Args = append(frag.Args, f.Value)
			paramIdx++

		default:
			op := sqlOp(f.Operator)
			whereParts = append(whereParts, fmt.Sprintf("%s %s $%d", col, op, paramIdx))
			frag.Args = append(frag.Args, f.Value)
			paramIdx++
		}
	}

	// Build WHERE string
	if len(whereParts) > 0 {
		parts := make([]string, len(whereParts))
		for i, p := range whereParts {
			parts[i] = "AND " + p
		}
		frag.Where = strings.Join(parts, " ")
	}

	// Cursor WHERE
	if vs.Cursor != nil {
		cursorWhere, cursorArgs := buildCursorWhere(vs.Cursor, vs.Sort, alias, paramIdx)
		if cursorWhere != "" {
			frag.CursorWhere = "AND " + cursorWhere
			frag.Args = append(frag.Args, cursorArgs...)
			paramIdx += len(cursorArgs)
		}
	}

	// ORDER BY
	if vs.RelevanceSort && ft != nil {
		lang := resolveFullTextLanguage(ft)
		rankExpr := fmt.Sprintf("ts_rank(%s.%s, plainto_tsquery('%s', $%d))", alias, ft.Column, lang, paramIdx)
		frag.Args = append(frag.Args, vs.Query)
		paramIdx++
		frag.OrderBy = rankExpr + " DESC, " + alias + ".id ASC"
	} else {
		orderParts := make([]string, len(vs.Sort))
		for i, s := range vs.Sort {
			dir := strings.ToUpper(string(s.Dir))
			orderParts[i] = fmt.Sprintf("%s.%s %s", alias, s.Column, dir)
		}
		frag.OrderBy = strings.Join(orderParts, ", ")
	}

	// Offset
	if vs.Offset != nil {
		frag.OffsetClause = fmt.Sprintf("OFFSET %d", *vs.Offset)
	}

	// Limit (+1 for has_more detection)
	frag.Limit = vs.Limit + 1
	frag.ArgCount = paramIdx - argOffset

	return frag
}

// buildCursorWhere generates the cursor seek WHERE clause.
// For same-direction sorts, uses row-value comparison.
// For mixed-direction sorts, uses expanded boolean form.
func buildCursorWhere(cursor *DecodedCursor, sort []SortDirective, alias string, startParam int) (string, []any) {
	if cursor == nil || len(cursor.Values) == 0 {
		return "", nil
	}

	// Check if all directions are the same
	allSame := true
	firstDir := sort[0].Dir
	for _, s := range sort[1:] {
		if s.Dir != firstDir {
			allSame = false
			break
		}
	}

	paramIdx := startParam
	var args []any

	if allSame {
		// Row-value comparison
		cols := make([]string, len(sort))
		params := make([]string, len(sort))
		for i, s := range sort {
			cols[i] = fmt.Sprintf("%s.%s", alias, s.Column)
			params[i] = fmt.Sprintf("$%d", paramIdx)
			args = append(args, cursor.Values[i])
			paramIdx++
		}
		op := ">"
		if firstDir == Desc {
			op = "<"
		}
		return fmt.Sprintf("(%s) %s (%s)", strings.Join(cols, ", "), op, strings.Join(params, ", ")), args
	}

	// Mixed-direction: expanded boolean form
	// (a < $1) OR (a = $1 AND b > $2) OR (a = $1 AND b = $2 AND c < $3) ...
	var orParts []string
	for i := range sort {
		var andParts []string
		// All preceding fields must be equal
		for j := 0; j < i; j++ {
			andParts = append(andParts, fmt.Sprintf("%s.%s = $%d", alias, sort[j].Column, paramIdx))
			args = append(args, cursor.Values[j])
			paramIdx++
		}
		// Current field uses directional comparison
		op := ">"
		if sort[i].Dir == Desc {
			op = "<"
		}
		andParts = append(andParts, fmt.Sprintf("%s.%s %s $%d", alias, sort[i].Column, op, paramIdx))
		args = append(args, cursor.Values[i])
		paramIdx++

		orParts = append(orParts, "("+strings.Join(andParts, " AND ")+")")
	}

	return strings.Join(orParts, " OR "), args
}

func sqlOp(op string) string {
	switch op {
	case "eq":
		return "="
	case "neq":
		return "!="
	case "gt":
		return ">"
	case "gte":
		return ">="
	case "lt":
		return "<"
	case "lte":
		return "<="
	default:
		return "="
	}
}

// escapeContains escapes %, _, and \ for ILIKE and wraps in %.
func escapeContains(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return "%" + s + "%"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./search/... -run TestBuildSQL -v`
Expected: All tests PASS. (Note: cursor-related tests may need the cursor types from Task 9. If `DecodedCursor` is still `any`, the cursor tests won't compile yet — that's fine, they'll pass after Task 9.)

- [ ] **Step 5: Commit**

```bash
git add search/builder.go search/builder_test.go
git commit -m "Add SQL builder with operator mapping and full-text search support"
```

---

### Task 9: Cursor Pagination

**Files:**
- Create: `search/cursor.go`
- Create: `search/cursor_test.go`
- Modify: `search/validated.go` — update `Cursor` field type from `any` to `*DecodedCursor`

- [ ] **Step 1: Write tests for cursor encode/decode**

Create `search/cursor_test.go`:

```go
package search_test

import (
	"testing"

	"github.com/mataki-dev/platform/search"
)

func TestCursor_RoundTrip(t *testing.T) {
	sort := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Desc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	lastRow := map[string]any{
		"created_at": "2026-03-15T10:30:00Z",
		"id":         "task_abc123",
	}

	token := search.EncodeCursor(lastRow, sort)
	if token == "" {
		t.Fatal("expected non-empty cursor token")
	}

	decoded, err := search.DecodeCursor(token)
	if err != nil {
		t.Fatalf("DecodeCursor error: %v", err)
	}

	if len(decoded.Values) != 2 {
		t.Fatalf("Values len = %d, want 2", len(decoded.Values))
	}
	if decoded.Values[0] != "2026-03-15T10:30:00Z" {
		t.Errorf("Values[0] = %v, want %q", decoded.Values[0], "2026-03-15T10:30:00Z")
	}
	if decoded.Values[1] != "task_abc123" {
		t.Errorf("Values[1] = %v, want %q", decoded.Values[1], "task_abc123")
	}
}

func TestCursor_MatchesSort(t *testing.T) {
	sort := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Desc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	lastRow := map[string]any{
		"created_at": "2026-03-15T10:30:00Z",
		"id":         "task_abc123",
	}

	token := search.EncodeCursor(lastRow, sort)
	decoded, _ := search.DecodeCursor(token)

	if !search.CursorMatchesSort(decoded, sort) {
		t.Error("cursor should match the same sort")
	}

	// Different sort order
	differentSort := []search.SortDirective{
		{Field: "priority", Column: "priority", Dir: search.Asc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	if search.CursorMatchesSort(decoded, differentSort) {
		t.Error("cursor should not match different sort fields")
	}

	// Same fields, different direction
	differentDir := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Asc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	if search.CursorMatchesSort(decoded, differentDir) {
		t.Error("cursor should not match different sort directions")
	}
}

func TestDecodeCursor_MalformedBase64(t *testing.T) {
	_, err := search.DecodeCursor("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for malformed base64")
	}
}

func TestDecodeCursor_MalformedJSON(t *testing.T) {
	// Valid base64 but not valid JSON
	_, err := search.DecodeCursor("bm90LWpzb24")
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestDecodeCursor_UnsupportedVersion(t *testing.T) {
	// Encode a cursor, decode it, change version, re-encode manually
	sort := []search.SortDirective{
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	lastRow := map[string]any{"id": "123"}
	token := search.EncodeCursor(lastRow, sort)
	decoded, _ := search.DecodeCursor(token)
	decoded.Version = 999

	// Re-encode with bad version should still decode, but fail validation
	reEncoded := search.EncodeCursorRaw(decoded)
	decoded2, err := search.DecodeCursor(reEncoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if decoded2.Version != 999 {
		t.Errorf("Version = %d, want 999", decoded2.Version)
	}
}

func TestEncodeCursor_EmptySort(t *testing.T) {
	token := search.EncodeCursor(nil, nil)
	if token != "" {
		t.Errorf("expected empty token for nil sort, got %q", token)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./search/... -run TestCursor -v`
Expected: Compilation failure.

- [ ] **Step 3: Implement cursor types and functions**

Create `search/cursor.go`:

```go
package search

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// DecodedCursor is the internal representation of a cursor token.
type DecodedCursor struct {
	Version int              `json:"v"`
	Sort    []cursorSortEntry `json:"s"`
	Values  []any            `json:"-"` // populated during decode
}

type cursorSortEntry struct {
	Field     string `json:"f"`
	Direction string `json:"d"`
	Value     any    `json:"val"`
}

const cursorVersion = 1

// EncodeCursor builds a cursor from the last row in a result set.
func EncodeCursor(lastRow map[string]any, sort []SortDirective) string {
	if len(sort) == 0 || lastRow == nil {
		return ""
	}

	c := DecodedCursor{
		Version: cursorVersion,
		Sort:    make([]cursorSortEntry, len(sort)),
	}
	for i, s := range sort {
		c.Sort[i] = cursorSortEntry{
			Field:     s.Field,
			Direction: string(s.Dir),
			Value:     lastRow[s.Field],
		}
	}

	return encodeCursorToString(&c)
}

// EncodeCursorRaw encodes a DecodedCursor back to a token string.
// Primarily used in tests.
func EncodeCursorRaw(c *DecodedCursor) string {
	return encodeCursorToString(c)
}

func encodeCursorToString(c *DecodedCursor) string {
	data, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

// DecodeCursor parses a cursor token string.
func DecodeCursor(token string) (*DecodedCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: malformed encoding: %w", err)
	}

	var c DecodedCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("invalid cursor: malformed data: %w", err)
	}

	// Extract values from sort entries
	c.Values = make([]any, len(c.Sort))
	for i, s := range c.Sort {
		c.Values[i] = s.Value
	}

	return &c, nil
}

// CursorMatchesSort checks whether a decoded cursor is compatible
// with the given sort directives. The field names and directions must match.
func CursorMatchesSort(c *DecodedCursor, sort []SortDirective) bool {
	if len(c.Sort) != len(sort) {
		return false
	}
	for i, s := range sort {
		if c.Sort[i].Field != s.Field {
			return false
		}
		if c.Sort[i].Direction != string(s.Dir) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Update validated.go — change Cursor type from `any` to `*DecodedCursor`**

In `search/validated.go`, ensure the `Cursor` field is typed as `*DecodedCursor`:

```go
Cursor *DecodedCursor // nil if not provided
```

(If it was set to `any` as a placeholder in Task 6, update it now.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./search/... -run TestCursor -v`
Expected: All tests PASS.

- [ ] **Step 6: Commit**

```bash
git add search/cursor.go search/cursor_test.go search/validated.go
git commit -m "Add cursor encode/decode with sort signature validation"
```

---

### Task 10: Cursor Validation Integration

**Files:**
- Modify: `search/validate.go` — add cursor decoding and sort-match validation
- Modify: `search/validate_test.go` — add cursor validation tests

- [ ] **Step 1: Write cursor validation tests**

Add to `search/validate_test.go`:

```go
func TestValidate_CursorValid(t *testing.T) {
	sort := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Desc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	lastRow := map[string]any{
		"created_at": "2026-03-15T10:30:00Z",
		"id":         "task_abc",
	}
	token := search.EncodeCursor(lastRow, sort)

	req := search.SearchRequest{
		Cursor: token,
		Sort: []search.SortDirectiveInput{
			{Field: "created_at", Direction: "desc"},
		},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Cursor == nil {
		t.Fatal("expected decoded cursor")
	}
}

func TestValidate_CursorMalformed(t *testing.T) {
	req := search.SearchRequest{
		Cursor: "not-a-valid-cursor!!!",
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "cursor", search.ErrInvalidCursor)
}

func TestValidate_CursorSortMismatch(t *testing.T) {
	// Create cursor with created_at desc sort
	sort := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Desc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	lastRow := map[string]any{
		"created_at": "2026-03-15T10:30:00Z",
		"id":         "task_abc",
	}
	token := search.EncodeCursor(lastRow, sort)

	// Use cursor with a different sort
	req := search.SearchRequest{
		Cursor: token,
		Sort: []search.SortDirectiveInput{
			{Field: "priority", Direction: "asc"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "cursor", search.ErrInvalidCursor)
}
```

- [ ] **Step 2: Run new tests to verify they fail**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./search/... -run "TestValidate_Cursor" -v`
Expected: Tests fail — cursor validation not yet implemented in Validate.

- [ ] **Step 3: Add cursor validation to Validate()**

In `search/validate.go`, add cursor decoding after the sort section (before the "Cursor vs relevance sort" section). Find the existing cursor+relevance check and replace/extend it:

```go
	// --- Cursor ---
	if req.Cursor != "" {
		decoded, err := DecodeCursor(req.Cursor)
		if err != nil {
			errs = append(errs, ValidationError{
				Field:   "cursor",
				Code:    ErrInvalidCursor,
				Message: "Cursor is malformed or invalid.",
			})
		} else if decoded.Version != cursorVersion {
			errs = append(errs, ValidationError{
				Field:   "cursor",
				Code:    ErrInvalidCursor,
				Message: "Unsupported cursor version.",
			})
		} else if !CursorMatchesSort(decoded, vs.Sort) {
			errs = append(errs, ValidationError{
				Field:   "cursor",
				Code:    ErrInvalidCursor,
				Message: "Cursor sort signature does not match the request sort order.",
			})
		} else {
			vs.Cursor = decoded
		}
	}
```

This should replace or be placed before the existing `req.Cursor != "" && vs.RelevanceSort` check. Keep the relevance sort check as well.

- [ ] **Step 4: Run all validation tests**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./search/... -run TestValidate -v`
Expected: All tests PASS (including the new cursor tests and the existing `TestValidate_CursorWithRelevanceSortRejected`).

- [ ] **Step 5: Commit**

```bash
git add search/validate.go search/validate_test.go
git commit -m "Add cursor decoding and sort-match validation to Validate()"
```

---

### Task 11: Response Helper

**Files:**
- Create: `search/response.go`
- Create: `search/response_test.go`

- [ ] **Step 1: Write tests for BuildResponse**

Create `search/response_test.go`:

```go
package search_test

import (
	"testing"

	"github.com/mataki-dev/platform/search"
)

type testRow struct {
	ID        string
	CreatedAt string
}

func testExtractFn(r testRow) map[string]any {
	return map[string]any{"created_at": r.CreatedAt, "id": r.ID}
}

func TestBuildResponse_HasMore(t *testing.T) {
	sort := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Desc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	vs := search.ValidatedSearch{
		Sort:  sort,
		Limit: 2,
	}

	// 3 rows returned (limit+1) → has_more=true, data has 2 rows
	rows := []testRow{
		{ID: "1", CreatedAt: "2026-03-15T10:00:00Z"},
		{ID: "2", CreatedAt: "2026-03-14T10:00:00Z"},
		{ID: "3", CreatedAt: "2026-03-13T10:00:00Z"}, // extra row
	}

	resp, err := search.BuildResponse(rows, vs, testExtractFn)
	if err != nil {
		t.Fatalf("BuildResponse error: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("Data len = %d, want 2", len(resp.Data))
	}
	if !resp.HasMore {
		t.Error("expected HasMore=true")
	}
	if resp.NextCursor == "" {
		t.Error("expected non-empty NextCursor")
	}
	if resp.Limit != 2 {
		t.Errorf("Limit = %d, want 2", resp.Limit)
	}
}

func TestBuildResponse_NoMore(t *testing.T) {
	sort := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Desc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	vs := search.ValidatedSearch{
		Sort:  sort,
		Limit: 5,
	}

	// 2 rows returned (less than limit+1) → has_more=false
	rows := []testRow{
		{ID: "1", CreatedAt: "2026-03-15T10:00:00Z"},
		{ID: "2", CreatedAt: "2026-03-14T10:00:00Z"},
	}

	resp, err := search.BuildResponse(rows, vs, testExtractFn)
	if err != nil {
		t.Fatalf("BuildResponse error: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("Data len = %d, want 2", len(resp.Data))
	}
	if resp.HasMore {
		t.Error("expected HasMore=false")
	}
	if resp.NextCursor != "" {
		t.Errorf("expected empty NextCursor, got %q", resp.NextCursor)
	}
}

func TestBuildResponse_EmptyRows(t *testing.T) {
	vs := search.ValidatedSearch{
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 25,
	}

	resp, err := search.BuildResponse([]testRow{}, vs, testExtractFn)
	if err != nil {
		t.Fatalf("BuildResponse error: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("Data len = %d, want 0", len(resp.Data))
	}
	if resp.HasMore {
		t.Error("expected HasMore=false")
	}
}

func TestBuildResponse_RelevanceSort_NoCursor(t *testing.T) {
	vs := search.ValidatedSearch{
		Limit:         2,
		RelevanceSort: true,
	}

	rows := []testRow{
		{ID: "1", CreatedAt: "2026-03-15T10:00:00Z"},
		{ID: "2", CreatedAt: "2026-03-14T10:00:00Z"},
		{ID: "3", CreatedAt: "2026-03-13T10:00:00Z"},
	}

	resp, err := search.BuildResponse(rows, vs, testExtractFn)
	if err != nil {
		t.Fatalf("BuildResponse error: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("Data len = %d, want 2", len(resp.Data))
	}
	if !resp.HasMore {
		t.Error("expected HasMore=true")
	}
	// No cursor for relevance sort
	if resp.NextCursor != "" {
		t.Errorf("expected empty NextCursor for relevance sort, got %q", resp.NextCursor)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./search/... -run TestBuildResponse`
Expected: Compilation failure.

- [ ] **Step 3: Implement BuildResponse**

Create `search/response.go`:

```go
package search

// BuildResponse handles the limit+1 trim, has_more detection, and
// cursor encoding for the last row.
//
// extractFn maps a product's row struct to the map of values needed
// for cursor encoding (sort field values).
func BuildResponse[T any](rows []T, req ValidatedSearch, extractFn func(T) map[string]any) (*SearchResponse[T], error) {
	resp := &SearchResponse[T]{
		Limit: req.Limit,
	}

	if len(rows) > req.Limit {
		resp.HasMore = true
		rows = rows[:req.Limit] // trim the extra row
	}

	resp.Data = rows

	// Encode cursor from the last row (only for non-relevance sorts)
	if resp.HasMore && !req.RelevanceSort && len(rows) > 0 {
		lastRow := extractFn(rows[len(rows)-1])
		resp.NextCursor = EncodeCursor(lastRow, req.Sort)
	}

	// Ensure Data is never nil (always empty array in JSON)
	if resp.Data == nil {
		resp.Data = make([]T, 0)
	}

	return resp, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./search/... -run TestBuildResponse -v`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add search/response.go search/response_test.go
git commit -m "Add BuildResponse helper with limit+1 trim and cursor encoding"
```

---

### Task 12: Huma Search Operation Registration

**Files:**
- Create: `search/huma.go`

This is the Huma integration layer. It depends on huma/v2 types and wires up validation + the search handler. Testing this requires a running Huma API which is integration-test territory — we'll create the registration helper without unit tests and verify compilation.

- [ ] **Step 1: Implement RegisterSearchOperation**

Create `search/huma.go`:

```go
package search

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/mataki-dev/platform/errors"
)

// OperationOption configures a search operation registration.
type OperationOption func(*operationConfig)

type operationConfig struct {
	tag         string
	summary     string
	description string
	middleware  []func(ctx huma.Context, next func(huma.Context))
}

// WithTag sets the operation tag.
func WithTag(tag string) OperationOption {
	return func(c *operationConfig) { c.tag = tag }
}

// WithSummary sets the operation summary.
func WithSummary(summary string) OperationOption {
	return func(c *operationConfig) { c.summary = summary }
}

// WithDescription sets the operation description.
func WithDescription(desc string) OperationOption {
	return func(c *operationConfig) { c.description = desc }
}

// WithMiddleware adds per-operation middleware.
func WithMiddleware(mw ...func(ctx huma.Context, next func(huma.Context))) OperationOption {
	return func(c *operationConfig) { c.middleware = append(c.middleware, mw...) }
}

// SearchInput wraps SearchRequest as a Huma request body.
type SearchInput struct {
	Body SearchRequest
}

// SearchHandler is the function signature products implement.
type SearchHandler[T any] func(ctx context.Context, req ValidatedSearch) (*SearchResponse[T], error)

// RegisterSearchOperation registers a POST /{resource}/search endpoint
// with standard validation, error handling, and OpenAPI metadata.
func RegisterSearchOperation[T any](
	api huma.API,
	path string,
	schema ResourceSchema,
	handler SearchHandler[T],
	opts ...OperationOption,
) {
	cfg := &operationConfig{
		summary: fmt.Sprintf("Search %s", path),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	op := huma.Operation{
		Method:      http.MethodPost,
		Path:        path,
		Summary:     cfg.summary,
		Description: cfg.description,
		Middlewares: huma.Middlewares(cfg.middleware),
	}
	if cfg.tag != "" {
		op.Tags = []string{cfg.tag}
	}

	huma.Register(api, op, func(ctx context.Context, input *SearchInput) (*struct{ Body *SearchResponse[T] }, error) {
		validated, validationErrs := Validate(input.Body, schema)
		if len(validationErrs) > 0 {
			fieldErrs := make([]errors.FieldError, len(validationErrs))
			for i, ve := range validationErrs {
				fieldErrs[i] = errors.FieldError{
					Field:   ve.Field,
					Code:    ve.Code,
					Message: ve.Message,
				}
			}
			return nil, errors.ToHumaError(
				errors.NewInvalidInput(
					"The search request contains invalid parameters.",
					errors.WithFieldErrors(fieldErrs...),
				),
			)
		}

		resp, err := handler(ctx, validated)
		if err != nil {
			var semErr *errors.SemanticError
			if e, ok := err.(*errors.SemanticError); ok {
				semErr = e
			} else {
				semErr = errors.NewInternal("An unexpected error occurred.", errors.WithCause(err))
			}
			return nil, errors.ToHumaError(semErr)
		}

		return &struct{ Body *SearchResponse[T] }{Body: resp}, nil
	})
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go build ./search/...`
Expected: Compiles successfully.

- [ ] **Step 3: Commit**

```bash
git add search/huma.go
git commit -m "Add Huma search operation registration helper"
```

---

### Task 13: Full Test Suite Pass

**Files:** None — this task verifies everything works together.

- [ ] **Step 1: Run all tests**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test ./... -v`
Expected: All tests PASS across both packages.

- [ ] **Step 2: Run with race detector**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go test -race ./...`
Expected: No race conditions detected, all tests PASS.

- [ ] **Step 3: Verify clean build**

Run: `cd /Users/sethyates/mataki/mataki-dev/platform && go vet ./...`
Expected: No issues found.

- [ ] **Step 4: Commit (if any fixes were needed)**

Only if steps above required changes:
```bash
git add -A
git commit -m "Fix issues found during full test suite run"
```
