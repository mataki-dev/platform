// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

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