// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package errors_test

import (
	"encoding/json"
	"fmt"
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

func TestNewHumaErrorHandler_SemanticError(t *testing.T) {
	handler := errors.NewHumaErrorHandler()
	semErr := errors.NewConflict("duplicate")
	humaErr := handler(semErr)

	if humaErr.GetStatus() != 409 {
		t.Errorf("GetStatus() = %d, want 409", humaErr.GetStatus())
	}
}

func TestNewHumaErrorHandler_NonSemanticError(t *testing.T) {
	handler := errors.NewHumaErrorHandler()
	humaErr := handler(fmt.Errorf("random failure"))

	if humaErr.GetStatus() != 500 {
		t.Errorf("GetStatus() = %d, want 500", humaErr.GetStatus())
	}
}