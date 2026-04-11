// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

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

func (e *SemanticError) Code() string   { return e.code }
func (e *SemanticError) HTTPStatus() int { return e.httpStatus }
func (e *SemanticError) Message() string { return e.message }

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