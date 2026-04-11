// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package search

// ValidationError represents a single validation issue in a search request.
type ValidationError struct {
	// Field is the dotted path to the invalid element.
	// e.g., "filter.status.eq", "sort[1].field", "limit"
	Field string `json:"field"`

	// Code is a machine-readable error code.
	Code string `json:"code"`

	// Message is a human-readable description.
	Message string `json:"message"`
}

// Stable error codes.
const (
	ErrUnknownField       = "unknown_field"
	ErrInvalidOperator    = "invalid_operator"
	ErrInvalidValue       = "invalid_value"
	ErrInvalidType        = "invalid_type"
	ErrExceedsMax         = "exceeds_max"
	ErrInvalidCursor      = "invalid_cursor"
	ErrUnsortableField    = "unsortable_field"
	ErrDuplicateSortField = "duplicate_sort_field"
	ErrNotNullable        = "not_nullable"
	ErrUnsupportedField   = "unsupported_field"
	ErrMutuallyExclusive  = "mutually_exclusive"
)