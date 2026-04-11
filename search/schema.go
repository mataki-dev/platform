// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package search

// FieldType defines the data type of a searchable field.
type FieldType int

const (
	String    FieldType = iota
	Numeric             // int, float, decimal
	Timestamp           // RFC 3339
	Bool
	Enum // validated against AllowedValues
)

// FieldDef defines a single searchable field in a resource schema.
type FieldDef struct {
	// Type governs which operators are valid and how values are parsed.
	Type FieldType

	// Column is the SQL column name. Defaults to the field key if empty.
	Column string

	// Operators lists the permitted operators for this field.
	// If empty, all type-compatible operators are allowed.
	Operators []string

	// Sortable indicates whether this field can appear in sort directives.
	Sortable bool

	// Nullable indicates whether is_null operator is accepted.
	Nullable bool

	// AllowedValues constrains Enum fields. Ignored for other types.
	AllowedValues []string

	// Selectable indicates whether this field can appear in the fields
	// projection list. Defaults to true if unset.
	Selectable *bool

	// MaxInSize overrides the default maximum set size for in/nin operators
	// on this field. 0 means use the resource default.
	MaxInSize int
}

// FullTextConfig configures full-text search for a resource.
type FullTextConfig struct {
	// Column is the pre-computed tsvector column name.
	Column string

	// Language is the text search configuration for tsquery parsing.
	// Defaults to "english".
	Language string
}

// ResourceSchema defines the searchable fields and constraints for a resource.
type ResourceSchema struct {
	// Fields maps API field names to their definitions.
	Fields map[string]FieldDef

	// MaxLimit is the ceiling for the limit parameter.
	MaxLimit int

	// DefaultLimit is applied when no limit is specified.
	DefaultLimit int

	// DefaultSort is applied when no sort is specified.
	DefaultSort []SortDirective

	// PrimaryKey is the column used as the cursor tiebreaker.
	// Defaults to "id".
	PrimaryKey string

	// MaxInSize is the default maximum set size for in/nin operators.
	// Defaults to 100.
	MaxInSize int

	// MaxOffset is the ceiling for offset-based pagination.
	// Defaults to 10000.
	MaxOffset int

	// TableAlias is the SQL table alias used in generated fragments.
	TableAlias string

	// FullText configures full-text search via the "query" field.
	// When nil, the "query" field is rejected in requests.
	FullText *FullTextConfig
}

func isFieldSelectable(f FieldDef) bool {
	if f.Selectable == nil {
		return true
	}
	return *f.Selectable
}

func resolveColumn(fieldKey string, f FieldDef) string {
	if f.Column != "" {
		return f.Column
	}
	return fieldKey
}

func defaultMaxInSize(f FieldDef, schema ResourceSchema) int {
	if f.MaxInSize > 0 {
		return f.MaxInSize
	}
	if schema.MaxInSize > 0 {
		return schema.MaxInSize
	}
	return 100
}

func resolveFullTextLanguage(ft *FullTextConfig) string {
	if ft.Language != "" {
		return ft.Language
	}
	return "english"
}