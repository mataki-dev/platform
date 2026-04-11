// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

// Package audit provides AuditLogger implementations for strongbox.
package audit

import (
	"context"

	"github.com/mataki-dev/platform/strongbox"
)

type noopLogger struct{}

func (noopLogger) Log(context.Context, strongbox.AuditEvent) {}

// NewNoop returns an AuditLogger that silently discards all events.
func NewNoop() strongbox.AuditLogger { return noopLogger{} }