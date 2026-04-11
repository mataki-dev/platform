// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package audit

import (
	"context"
	"log/slog"

	"github.com/mataki-dev/platform/strongbox"
)

type slogLogger struct{ logger *slog.Logger }

// NewSlog returns an AuditLogger that emits structured log records via the
// provided slog.Logger. Secret values and ciphertext are never logged.
func NewSlog(logger *slog.Logger) strongbox.AuditLogger { return &slogLogger{logger: logger} }

func (l *slogLogger) Log(ctx context.Context, ev strongbox.AuditEvent) {
	attrs := []slog.Attr{
		slog.String("operation", ev.Operation),
		slog.String("client_id", string(ev.ClientID)),
		slog.String("tenant_id", string(ev.TenantID)),
		slog.Int("count", ev.Count),
		slog.Duration("duration", ev.Duration),
	}

	if ev.Ref != "" {
		attrs = append(attrs, slog.String("ref", string(ev.Ref)))
	}
	if len(ev.Refs) > 0 {
		refs := make([]string, len(ev.Refs))
		for i, r := range ev.Refs {
			refs[i] = string(r)
		}
		attrs = append(attrs, slog.Any("refs", refs))
	}
	if ev.Source != "" {
		attrs = append(attrs, slog.String("source", ev.Source))
	}
	if ev.Actor != "" {
		attrs = append(attrs, slog.String("actor", ev.Actor))
	}
	if ev.Error != "" {
		attrs = append(attrs, slog.String("error", ev.Error))
	}

	level := slog.LevelInfo
	if ev.Error != "" {
		level = slog.LevelError
	}

	l.logger.LogAttrs(ctx, level, "audit", attrs...)
}