// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

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