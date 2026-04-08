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
