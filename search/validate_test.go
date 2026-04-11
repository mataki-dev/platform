// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package search_test

import (
	"testing"

	"github.com/mataki-dev/platform/search"
)

// testSchema is a shared schema for validation tests.
var testSchema = search.ResourceSchema{
	Fields: map[string]search.FieldDef{
		"status": {
			Type:          search.Enum,
			Operators:     []string{"eq", "neq", "in"},
			Sortable:      true,
			AllowedValues: []string{"open", "in_progress", "done"},
		},
		"priority": {
			Type:     search.Numeric,
			Sortable: true,
		},
		"title": {
			Type:      search.String,
			Operators: []string{"contains"},
		},
		"created_at": {
			Type:     search.Timestamp,
			Sortable: true,
		},
		"assignee_id": {
			Type:     search.String,
			Nullable: true,
		},
		"is_active": {
			Type: search.Bool,
		},
	},
	MaxLimit:     100,
	DefaultLimit: 25,
	DefaultSort:  []search.SortDirective{{Field: "created_at", Column: "created_at", Dir: search.Desc}},
	PrimaryKey:   "id",
	TableAlias:   "t",
	MaxInSize:    100,
}

var testSchemaWithFTS = func() search.ResourceSchema {
	s := testSchema
	s.FullText = &search.FullTextConfig{Column: "search_vector", Language: "english"}
	return s
}()

func TestValidate_EmptyRequest(t *testing.T) {
	vs, errs := search.Validate(search.SearchRequest{}, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Limit != 25 {
		t.Errorf("Limit = %d, want 25", vs.Limit)
	}
	if len(vs.Sort) == 0 {
		t.Fatal("expected default sort")
	}
	// Default sort + tiebreaker
	if vs.Sort[0].Field != "created_at" || vs.Sort[0].Dir != search.Desc {
		t.Errorf("default sort = %+v, want created_at desc", vs.Sort[0])
	}
}

func TestValidate_FilterUnknownField(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"nonexistent": {"eq": "foo"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.nonexistent", search.ErrUnknownField)
}

func TestValidate_FilterInvalidOperatorForType(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"status": {"gt": "open"}, // gt not valid for Enum
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.status.gt", search.ErrInvalidOperator)
}

func TestValidate_FilterOperatorNotInAllowedList(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"title": {"eq": "hello"}, // title only allows "contains"
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.title.eq", search.ErrInvalidOperator)
}

func TestValidate_FilterContainsOnNumeric(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"priority": {"contains": "high"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.priority.contains", search.ErrInvalidOperator)
}

func TestValidate_FilterGtOnString(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"title": {"gt": "abc"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.title.gt", search.ErrInvalidOperator)
}

func TestValidate_FilterIsNullOnNonNullable(t *testing.T) {
	// priority has no Operators restriction, so is_null is type-valid
	// but the field is not marked Nullable
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"priority": {"is_null": true},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.priority.is_null", search.ErrNotNullable)
}

func TestValidate_FilterIsNullOnNullableField(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"assignee_id": {"is_null": true},
		},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(vs.Filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(vs.Filters))
	}
	if vs.Filters[0].Operator != "is_null" {
		t.Errorf("operator = %q, want %q", vs.Filters[0].Operator, "is_null")
	}
}

func TestValidate_FilterIsNullNonBoolValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"assignee_id": {"is_null": "yes"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.assignee_id.is_null", search.ErrInvalidType)
}

func TestValidate_FilterEnumInvalidValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"status": {"eq": "invalid_status"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.status.eq", search.ErrInvalidValue)
}

func TestValidate_FilterEnumValidValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"status": {"eq": "open"},
		},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(vs.Filters) != 1 {
		t.Fatal("expected 1 filter")
	}
	if vs.Filters[0].Value != "open" {
		t.Errorf("value = %v, want %q", vs.Filters[0].Value, "open")
	}
}

func TestValidate_FilterTimestampInvalidValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"created_at": {"gte": "not-a-timestamp"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.created_at.gte", search.ErrInvalidType)
}

func TestValidate_FilterTimestampValidValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"created_at": {"gte": "2026-01-01T00:00:00Z"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_FilterNumericInvalidValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"priority": {"eq": "not-a-number"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.priority.eq", search.ErrInvalidType)
}

func TestValidate_FilterNumericValidValue(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"priority": {"gte": float64(5)},
		},
	}
	_, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_FilterMultipleOperatorsOnSameField(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"created_at": {"gte": "2026-01-01T00:00:00Z", "lt": "2026-04-01T00:00:00Z"},
		},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(vs.Filters) != 2 {
		t.Errorf("expected 2 filters, got %d", len(vs.Filters))
	}
}

func TestValidate_FilterInExceedsMaxSize(t *testing.T) {
	vals := make([]any, 101)
	for i := range vals {
		vals[i] = "val"
	}
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"status": {"in": vals},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.status.in", search.ErrExceedsMax)
}

func TestValidate_SortUnsortableField(t *testing.T) {
	req := search.SearchRequest{
		Sort: []search.SortDirectiveInput{
			{Field: "title", Direction: "asc"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "sort[0].field", search.ErrUnsortableField)
}

func TestValidate_SortInvalidDirection(t *testing.T) {
	req := search.SearchRequest{
		Sort: []search.SortDirectiveInput{
			{Field: "created_at", Direction: "sideways"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "sort[0].direction", search.ErrInvalidValue)
}

func TestValidate_SortDuplicateField(t *testing.T) {
	req := search.SearchRequest{
		Sort: []search.SortDirectiveInput{
			{Field: "created_at", Direction: "asc"},
			{Field: "created_at", Direction: "desc"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "sort[1].field", search.ErrDuplicateSortField)
}

func TestValidate_SortMaxDirectives(t *testing.T) {
	req := search.SearchRequest{
		Sort: []search.SortDirectiveInput{
			{Field: "created_at"},
			{Field: "priority"},
			{Field: "status"},
			{Field: "created_at"}, // 4th — over limit
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "sort", search.ErrExceedsMax)
}

func TestValidate_SortDefaultDirection(t *testing.T) {
	req := search.SearchRequest{
		Sort: []search.SortDirectiveInput{
			{Field: "created_at"}, // no direction specified
		},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Sort[0].Dir != search.Asc {
		t.Errorf("Dir = %q, want %q", vs.Sort[0].Dir, search.Asc)
	}
}

func TestValidate_SortAppendsTiebreaker(t *testing.T) {
	req := search.SearchRequest{
		Sort: []search.SortDirectiveInput{
			{Field: "created_at", Direction: "desc"},
		},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(vs.Sort) != 2 {
		t.Fatalf("expected 2 sort directives (incl tiebreaker), got %d", len(vs.Sort))
	}
	last := vs.Sort[len(vs.Sort)-1]
	if last.Field != "id" || last.Dir != search.Asc {
		t.Errorf("tiebreaker = %+v, want id asc", last)
	}
}

func TestValidate_CursorValid(t *testing.T) {
	sort := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Desc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	lastRow := map[string]any{
		"created_at": "2026-03-15T10:30:00Z",
		"id":         "task_abc",
	}
	token := search.EncodeCursor(lastRow, sort)

	req := search.SearchRequest{
		Cursor: token,
		Sort: []search.SortDirectiveInput{
			{Field: "created_at", Direction: "desc"},
		},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Cursor == nil {
		t.Fatal("expected decoded cursor")
	}
}

func TestValidate_CursorMalformed(t *testing.T) {
	req := search.SearchRequest{
		Cursor: "not-a-valid-cursor!!!",
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "cursor", search.ErrInvalidCursor)
}

func TestValidate_CursorSortMismatch(t *testing.T) {
	sort := []search.SortDirective{
		{Field: "created_at", Column: "created_at", Dir: search.Desc},
		{Field: "id", Column: "id", Dir: search.Asc},
	}
	lastRow := map[string]any{
		"created_at": "2026-03-15T10:30:00Z",
		"id":         "task_abc",
	}
	token := search.EncodeCursor(lastRow, sort)

	req := search.SearchRequest{
		Cursor: token,
		Sort: []search.SortDirectiveInput{
			{Field: "priority", Direction: "asc"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "cursor", search.ErrInvalidCursor)
}

func TestValidate_LimitDefaults(t *testing.T) {
	req := search.SearchRequest{}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Limit != 25 {
		t.Errorf("Limit = %d, want 25", vs.Limit)
	}
}

func TestValidate_LimitClampedToMax(t *testing.T) {
	limit := 999
	req := search.SearchRequest{Limit: &limit}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Limit != 100 {
		t.Errorf("Limit = %d, want 100 (clamped)", vs.Limit)
	}
}

func TestValidate_LimitClampedToMin(t *testing.T) {
	limit := 0
	req := search.SearchRequest{Limit: &limit}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Limit != 1 {
		t.Errorf("Limit = %d, want 1 (clamped)", vs.Limit)
	}
}

func TestValidate_FieldsInvalid(t *testing.T) {
	req := search.SearchRequest{
		Fields: []string{"id", "nonexistent"},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "fields[1]", search.ErrUnknownField)
}

func TestValidate_FieldsValid(t *testing.T) {
	req := search.SearchRequest{
		Fields: []string{"status", "created_at"},
	}
	vs, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(vs.Fields) != 2 {
		t.Errorf("Fields len = %d, want 2", len(vs.Fields))
	}
}

func TestValidate_QueryWithoutFullTextConfig(t *testing.T) {
	req := search.SearchRequest{Query: "hello"}
	_, errs := search.Validate(req, testSchema) // testSchema has no FullText
	assertHasError(t, errs, "query", search.ErrUnsupportedField)
}

func TestValidate_QueryWithFullTextConfig(t *testing.T) {
	req := search.SearchRequest{Query: "hello"}
	vs, errs := search.Validate(req, testSchemaWithFTS)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.Query != "hello" {
		t.Errorf("Query = %q, want %q", vs.Query, "hello")
	}
	if !vs.RelevanceSort {
		t.Error("expected RelevanceSort=true when query present and no explicit sort")
	}
}

func TestValidate_QueryWithExplicitSort(t *testing.T) {
	req := search.SearchRequest{
		Query: "hello",
		Sort:  []search.SortDirectiveInput{{Field: "created_at", Direction: "desc"}},
	}
	vs, errs := search.Validate(req, testSchemaWithFTS)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if vs.RelevanceSort {
		t.Error("expected RelevanceSort=false when explicit sort provided")
	}
}

func TestValidate_CursorAndOffsetMutuallyExclusive(t *testing.T) {
	offset := 10
	req := search.SearchRequest{
		Cursor: "some-cursor",
		Offset: &offset,
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "cursor", search.ErrMutuallyExclusive)
}

func TestValidate_CursorWithRelevanceSortRejected(t *testing.T) {
	req := search.SearchRequest{
		Query:  "hello",
		Cursor: "some-cursor",
	}
	_, errs := search.Validate(req, testSchemaWithFTS)
	assertHasError(t, errs, "cursor", search.ErrInvalidCursor)
}

func TestValidate_MultipleErrors(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"nonexistent": {"eq": "foo"},
			"status":      {"gt": "open"},
		},
		Sort: []search.SortDirectiveInput{
			{Field: "title"},
		},
	}
	_, errs := search.Validate(req, testSchema)
	if len(errs) < 3 {
		t.Errorf("expected at least 3 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidate_BoolEqValid(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"is_active": {"eq": true},
		},
	}
	_, errs := search.Validate(req, testSchema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_BoolInvalidOperator(t *testing.T) {
	req := search.SearchRequest{
		Filter: map[string]map[string]any{
			"is_active": {"gt": true},
		},
	}
	_, errs := search.Validate(req, testSchema)
	assertHasError(t, errs, "filter.is_active.gt", search.ErrInvalidOperator)
}

// assertHasError checks that at least one validation error matches
// the given field and code.
func assertHasError(t *testing.T, errs []search.ValidationError, field, code string) {
	t.Helper()
	for _, e := range errs {
		if e.Field == field && e.Code == code {
			return
		}
	}
	t.Errorf("expected error with field=%q code=%q, got %v", field, code, errs)
}