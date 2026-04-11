// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

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