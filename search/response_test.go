// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package search_test

import (
	"testing"

	"github.com/mataki-dev/platform/search"
)

type testRow struct {
	ID        string
	CreatedAt string
}

func testExtractFn(r testRow) map[string]any {
	return map[string]any{"created_at": r.CreatedAt, "id": r.ID}
}

func TestBuildResponse_HasMore(t *testing.T) {
	sort := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Desc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	vs := search.ValidatedSearch{
		Sort:  sort,
		Limit: 2,
	}

	rows := []testRow{
		{ID: "1", CreatedAt: "2026-03-15T10:00:00Z"},
		{ID: "2", CreatedAt: "2026-03-14T10:00:00Z"},
		{ID: "3", CreatedAt: "2026-03-13T10:00:00Z"},
	}

	resp, err := search.BuildResponse(rows, vs, testExtractFn)
	if err != nil {
		t.Fatalf("BuildResponse error: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("Data len = %d, want 2", len(resp.Data))
	}
	if !resp.HasMore {
		t.Error("expected HasMore=true")
	}
	if resp.NextCursor == "" {
		t.Error("expected non-empty NextCursor")
	}
	if resp.Limit != 2 {
		t.Errorf("Limit = %d, want 2", resp.Limit)
	}
}

func TestBuildResponse_NoMore(t *testing.T) {
	sort := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Desc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	vs := search.ValidatedSearch{
		Sort:  sort,
		Limit: 5,
	}

	rows := []testRow{
		{ID: "1", CreatedAt: "2026-03-15T10:00:00Z"},
		{ID: "2", CreatedAt: "2026-03-14T10:00:00Z"},
	}

	resp, err := search.BuildResponse(rows, vs, testExtractFn)
	if err != nil {
		t.Fatalf("BuildResponse error: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("Data len = %d, want 2", len(resp.Data))
	}
	if resp.HasMore {
		t.Error("expected HasMore=false")
	}
	if resp.NextCursor != "" {
		t.Errorf("expected empty NextCursor, got %q", resp.NextCursor)
	}
}

func TestBuildResponse_EmptyRows(t *testing.T) {
	vs := search.ValidatedSearch{
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 25,
	}

	resp, err := search.BuildResponse([]testRow{}, vs, testExtractFn)
	if err != nil {
		t.Fatalf("BuildResponse error: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("Data len = %d, want 0", len(resp.Data))
	}
	if resp.HasMore {
		t.Error("expected HasMore=false")
	}
}

func TestBuildResponse_RelevanceSort_NoCursor(t *testing.T) {
	vs := search.ValidatedSearch{
		Limit:         2,
		RelevanceSort: true,
	}

	rows := []testRow{
		{ID: "1", CreatedAt: "2026-03-15T10:00:00Z"},
		{ID: "2", CreatedAt: "2026-03-14T10:00:00Z"},
		{ID: "3", CreatedAt: "2026-03-13T10:00:00Z"},
	}

	resp, err := search.BuildResponse(rows, vs, testExtractFn)
	if err != nil {
		t.Fatalf("BuildResponse error: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("Data len = %d, want 2", len(resp.Data))
	}
	if !resp.HasMore {
		t.Error("expected HasMore=true")
	}
	if resp.NextCursor != "" {
		t.Errorf("expected empty NextCursor for relevance sort, got %q", resp.NextCursor)
	}
}