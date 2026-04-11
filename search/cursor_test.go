// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package search_test

import (
	"testing"

	"github.com/mataki-dev/platform/search"
)

func TestCursor_RoundTrip(t *testing.T) {
	sort := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Desc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	lastRow := map[string]any{
		"created_at": "2026-03-15T10:30:00Z",
		"id":         "task_abc123",
	}

	token := search.EncodeCursor(lastRow, sort)
	if token == "" {
		t.Fatal("expected non-empty cursor token")
	}

	decoded, err := search.DecodeCursor(token)
	if err != nil {
		t.Fatalf("DecodeCursor error: %v", err)
	}

	if len(decoded.Values) != 2 {
		t.Fatalf("Values len = %d, want 2", len(decoded.Values))
	}
	if decoded.Values[0] != "2026-03-15T10:30:00Z" {
		t.Errorf("Values[0] = %v, want %q", decoded.Values[0], "2026-03-15T10:30:00Z")
	}
	if decoded.Values[1] != "task_abc123" {
		t.Errorf("Values[1] = %v, want %q", decoded.Values[1], "task_abc123")
	}
}

func TestCursor_MatchesSort(t *testing.T) {
	sort := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Desc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	lastRow := map[string]any{
		"created_at": "2026-03-15T10:30:00Z",
		"id":         "task_abc123",
	}

	token := search.EncodeCursor(lastRow, sort)
	decoded, _ := search.DecodeCursor(token)

	if !search.CursorMatchesSort(decoded, sort) {
		t.Error("cursor should match the same sort")
	}

	// Different sort order
	differentSort := []search.SortDirective{
		{Field: "priority", Column: "priority", Dir: search.Asc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	if search.CursorMatchesSort(decoded, differentSort) {
		t.Error("cursor should not match different sort fields")
	}

	// Same fields, different direction
	differentDir := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Asc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	if search.CursorMatchesSort(decoded, differentDir) {
		t.Error("cursor should not match different sort directions")
	}
}

func TestDecodeCursor_MalformedBase64(t *testing.T) {
	_, err := search.DecodeCursor("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for malformed base64")
	}
}

func TestDecodeCursor_MalformedJSON(t *testing.T) {
	_, err := search.DecodeCursor("bm90LWpzb24")
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestDecodeCursor_UnsupportedVersion(t *testing.T) {
	sort := []search.SortDirective{
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	lastRow := map[string]any{"id": "123"}
	token := search.EncodeCursor(lastRow, sort)
	decoded, _ := search.DecodeCursor(token)
	decoded.Version = 999

	reEncoded := search.EncodeCursorRaw(decoded)
	decoded2, err := search.DecodeCursor(reEncoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if decoded2.Version != 999 {
		t.Errorf("Version = %d, want 999", decoded2.Version)
	}
}

func TestEncodeCursor_EmptySort(t *testing.T) {
	token := search.EncodeCursor(nil, nil)
	if token != "" {
		t.Errorf("expected empty token for nil sort, got %q", token)
	}
}