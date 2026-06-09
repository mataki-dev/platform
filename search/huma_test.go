// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package search

import "testing"

func TestWithMetadata_MergesOperationMetadata(t *testing.T) {
	cfg := &operationConfig{}
	WithMetadata(map[string]any{"requires_permission": "keys:read"})(cfg)
	WithMetadata(map[string]any{"audit_event": "keys.search"})(cfg)

	if cfg.metadata["requires_permission"] != "keys:read" {
		t.Fatalf("requires_permission = %v", cfg.metadata["requires_permission"])
	}
	if cfg.metadata["audit_event"] != "keys.search" {
		t.Fatalf("audit_event = %v", cfg.metadata["audit_event"])
	}
}
