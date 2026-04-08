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

// NewHumaErrorHandler returns a function that converts any error to a
// huma.StatusError. If the error is a *SemanticError, it renders with
// the correct status and envelope. Otherwise, it wraps as 500 Internal.
func NewHumaErrorHandler() func(err error) huma.StatusError {
	return func(err error) huma.StatusError {
		if se, ok := err.(*SemanticError); ok {
			return ToHumaError(se)
		}
		return ToHumaError(NewInternal("An unexpected error occurred.", WithCause(err)))
	}
}
