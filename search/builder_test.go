package search_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mataki-dev/platform/search"
)

func TestBuildSQL_NoFilters(t *testing.T) {
	vs := search.ValidatedSearch{
		Sort:  []search.SortDirective{{Field: "created_at", Column: "created_at", Dir: search.Desc}, {Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 25,
	}
	frag := search.BuildSQL(vs, 1)

	if frag.Where != "" {
		t.Errorf("Where = %q, want empty", frag.Where)
	}
	if frag.OrderBy != "t.created_at DESC, t.id ASC" {
		t.Errorf("OrderBy = %q, want %q", frag.OrderBy, "t.created_at DESC, t.id ASC")
	}
	if frag.Limit != 26 {
		t.Errorf("Limit = %d, want 26", frag.Limit)
	}
	if len(frag.Args) != 0 {
		t.Errorf("Args len = %d, want 0", len(frag.Args))
	}
}

func TestBuildSQL_SingleEqFilter(t *testing.T) {
	vs := search.ValidatedSearch{
		Filters: []search.ValidatedFilter{
			{Field: "status", Column: "status", Operator: "eq", Value: "open"},
		},
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 25,
	}
	frag := search.BuildSQL(vs, 2)

	if frag.Where != "AND t.status = $2" {
		t.Errorf("Where = %q, want %q", frag.Where, "AND t.status = $2")
	}
	if len(frag.Args) != 1 {
		t.Fatalf("Args len = %d, want 1", len(frag.Args))
	}
	if frag.Args[0] != "open" {
		t.Errorf("Args[0] = %v, want %q", frag.Args[0], "open")
	}
	if frag.ArgCount != 1 {
		t.Errorf("ArgCount = %d, want 1", frag.ArgCount)
	}
}

func TestBuildSQL_MultipleFilters(t *testing.T) {
	ts, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	vs := search.ValidatedSearch{
		Filters: []search.ValidatedFilter{
			{Field: "status", Column: "status", Operator: "eq", Value: "open"},
			{Field: "created_at", Column: "created_at", Operator: "gte", Value: ts},
		},
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 25,
	}
	frag := search.BuildSQL(vs, 1)

	if !strings.Contains(frag.Where, "AND t.status = $1") {
		t.Errorf("Where missing status filter: %q", frag.Where)
	}
	if !strings.Contains(frag.Where, "AND t.created_at >= $2") {
		t.Errorf("Where missing created_at filter: %q", frag.Where)
	}
	if len(frag.Args) != 2 {
		t.Errorf("Args len = %d, want 2", len(frag.Args))
	}
}

func TestBuildSQL_AllOperators(t *testing.T) {
	tests := []struct {
		op       string
		value    any
		wantSQL  string
		wantArgs int
	}{
		{"eq", "val", "AND t.col = $1", 1},
		{"neq", "val", "AND t.col != $1", 1},
		{"gt", float64(5), "AND t.col > $1", 1},
		{"gte", float64(5), "AND t.col >= $1", 1},
		{"lt", float64(5), "AND t.col < $1", 1},
		{"lte", float64(5), "AND t.col <= $1", 1},
		{"in", []any{"a", "b"}, "AND t.col = ANY($1)", 1},
		{"nin", []any{"a", "b"}, "AND t.col != ALL($1)", 1},
		{"contains", "test", "AND t.col ILIKE $1", 1},
	}

	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			vs := search.ValidatedSearch{
				Filters: []search.ValidatedFilter{
					{Field: "col", Column: "col", Operator: tt.op, Value: tt.value},
				},
				Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
				Limit: 10,
			}
			frag := search.BuildSQL(vs, 1)
			if frag.Where != tt.wantSQL {
				t.Errorf("Where = %q, want %q", frag.Where, tt.wantSQL)
			}
			if len(frag.Args) != tt.wantArgs {
				t.Errorf("Args len = %d, want %d", len(frag.Args), tt.wantArgs)
			}
		})
	}
}

func TestBuildSQL_IsNull(t *testing.T) {
	vs := search.ValidatedSearch{
		Filters: []search.ValidatedFilter{
			{Field: "assignee", Column: "assignee_id", Operator: "is_null", Value: true},
		},
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 10,
	}
	frag := search.BuildSQL(vs, 1)
	if frag.Where != "AND t.assignee_id IS NULL" {
		t.Errorf("Where = %q, want %q", frag.Where, "AND t.assignee_id IS NULL")
	}
	if len(frag.Args) != 0 {
		t.Errorf("Args len = %d, want 0", len(frag.Args))
	}
}

func TestBuildSQL_IsNotNull(t *testing.T) {
	vs := search.ValidatedSearch{
		Filters: []search.ValidatedFilter{
			{Field: "assignee", Column: "assignee_id", Operator: "is_null", Value: false},
		},
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 10,
	}
	frag := search.BuildSQL(vs, 1)
	if frag.Where != "AND t.assignee_id IS NOT NULL" {
		t.Errorf("Where = %q, want %q", frag.Where, "AND t.assignee_id IS NOT NULL")
	}
}

func TestBuildSQL_ContainsEscaping(t *testing.T) {
	vs := search.ValidatedSearch{
		Filters: []search.ValidatedFilter{
			{Field: "title", Column: "title", Operator: "contains", Value: "100% off_sale\\deal"},
		},
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 10,
	}
	frag := search.BuildSQL(vs, 1)
	arg := frag.Args[0].(string)
	expected := "%100\\% off\\_sale\\\\deal%"
	if arg != expected {
		t.Errorf("contains arg = %q, want %q", arg, expected)
	}
}

func TestBuildSQL_OrderByMultipleDirections(t *testing.T) {
	vs := search.ValidatedSearch{
		Sort: []search.SortDirective{
			{Field: "created_at", Column: "created_at", Dir: search.Desc},
			{Field: "priority", Column: "priority", Dir: search.Asc},
			{Field: "id", Column: "id", Dir: search.Asc},
		},
		Limit: 25,
	}
	frag := search.BuildSQL(vs, 1)
	if frag.OrderBy != "t.created_at DESC, t.priority ASC, t.id ASC" {
		t.Errorf("OrderBy = %q", frag.OrderBy)
	}
}

func TestBuildSQL_LimitPlusOne(t *testing.T) {
	vs := search.ValidatedSearch{
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 10,
	}
	frag := search.BuildSQL(vs, 1)
	if frag.Limit != 11 {
		t.Errorf("Limit = %d, want 11", frag.Limit)
	}
}

func TestBuildSQL_FullTextWhere(t *testing.T) {
	ft := &search.FullTextConfig{Column: "search_vector", Language: "english"}
	vs := search.ValidatedSearch{
		Query:         "hello world",
		RelevanceSort: true,
		Limit:         25,
	}
	frag := search.BuildSQLWithFullText(vs, 1, ft)

	if !strings.Contains(frag.Where, "t.search_vector @@ plainto_tsquery('english', $1)") {
		t.Errorf("Where missing tsquery clause: %q", frag.Where)
	}
	if frag.Args[0] != "hello world" {
		t.Errorf("Args[0] = %v, want %q", frag.Args[0], "hello world")
	}
}

func TestBuildSQL_FullTextRelevanceOrder(t *testing.T) {
	ft := &search.FullTextConfig{Column: "search_vector", Language: "english"}
	vs := search.ValidatedSearch{
		Query:         "hello",
		RelevanceSort: true,
		Limit:         25,
	}
	frag := search.BuildSQLWithFullText(vs, 1, ft)

	if !strings.Contains(frag.OrderBy, "ts_rank(t.search_vector, plainto_tsquery('english', $") {
		t.Errorf("OrderBy missing ts_rank: %q", frag.OrderBy)
	}
	if !strings.HasSuffix(frag.OrderBy, " DESC, t.id ASC") {
		t.Errorf("OrderBy missing id tiebreaker: %q", frag.OrderBy)
	}
}

func TestBuildSQL_FullTextWithExplicitSort(t *testing.T) {
	ft := &search.FullTextConfig{Column: "search_vector", Language: "english"}
	vs := search.ValidatedSearch{
		Query:         "hello",
		RelevanceSort: false,
		Sort:          []search.SortDirective{{Field: "created_at", Column: "created_at", Dir: search.Desc}, {Field: "id", Column: "id", Dir: search.Asc}},
		Limit:         25,
	}
	frag := search.BuildSQLWithFullText(vs, 1, ft)

	if !strings.Contains(frag.Where, "t.search_vector @@ plainto_tsquery") {
		t.Errorf("Where missing tsquery clause: %q", frag.Where)
	}
	if strings.Contains(frag.OrderBy, "ts_rank") {
		t.Errorf("OrderBy should not contain ts_rank when explicit sort: %q", frag.OrderBy)
	}
	if frag.OrderBy != "t.created_at DESC, t.id ASC" {
		t.Errorf("OrderBy = %q, want %q", frag.OrderBy, "t.created_at DESC, t.id ASC")
	}
}

func TestBuildSQL_Offset(t *testing.T) {
	offset := 50
	vs := search.ValidatedSearch{
		Sort:   []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit:  25,
		Offset: &offset,
	}
	frag := search.BuildSQL(vs, 1)
	if frag.OffsetClause != "OFFSET 50" {
		t.Errorf("OffsetClause = %q, want %q", frag.OffsetClause, "OFFSET 50")
	}
}

func TestBuildSQL_CustomTableAlias(t *testing.T) {
	vs := search.ValidatedSearch{
		Filters: []search.ValidatedFilter{
			{Field: "status", Column: "status", Operator: "eq", Value: "open"},
		},
		Sort:  []search.SortDirective{{Field: "id", Column: "id", Dir: search.Asc}},
		Limit: 10,
	}
	frag := search.BuildSQLWithAlias(vs, 1, "tbl")
	if frag.Where != "AND tbl.status = $1" {
		t.Errorf("Where = %q, want %q", frag.Where, "AND tbl.status = $1")
	}
}
