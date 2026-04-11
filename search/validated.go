// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package search

// ValidatedSearch is the output of Validate(). The SQL builder only
// accepts this type, enforcing parse-don't-validate.
type ValidatedSearch struct {
	Filters       []ValidatedFilter
	Sort          []SortDirective // includes tiebreaker
	Cursor        *DecodedCursor  // nil if not provided
	Offset        *int            // nil if not provided
	Limit         int
	Fields        []string // nil means all defaults
	Query         string   // non-empty if full-text search
	RelevanceSort bool     // true when sorting by ts_rank
}

// ValidatedFilter is a single filter clause after validation.
type ValidatedFilter struct {
	Field    string
	Column   string // resolved SQL column name
	Operator string
	Value    any    // parsed to correct Go type
}

// SortDirective is a validated sort directive with resolved column.
type SortDirective struct {
	Field  string  `json:"field"`
	Column string  `json:"-"`
	Dir    SortDir `json:"direction"`
}

// SortDir is the sort direction.
type SortDir string

const (
	Asc  SortDir = "asc"
	Desc SortDir = "desc"
)