package search

import (
	"fmt"
	"slices"
	"strconv"
	"time"
)

// operatorsForType returns the set of operators valid for a given field type.
func operatorsForType(ft FieldType) []string {
	switch ft {
	case String:
		return []string{"eq", "neq", "in", "nin", "contains", "is_null"}
	case Numeric:
		return []string{"eq", "neq", "gt", "gte", "lt", "lte", "in", "nin", "is_null"}
	case Timestamp:
		return []string{"eq", "neq", "gt", "gte", "lt", "lte", "is_null"}
	case Bool:
		return []string{"eq", "neq", "is_null"}
	case Enum:
		return []string{"eq", "neq", "in", "nin", "is_null"}
	default:
		return nil
	}
}

// Validate validates a SearchRequest against a ResourceSchema.
// Validation is exhaustive — all errors are collected, not fail-fast.
func Validate(req SearchRequest, schema ResourceSchema) (ValidatedSearch, []ValidationError) {
	var errs []ValidationError
	vs := ValidatedSearch{}

	// --- Query ---
	if req.Query != "" {
		if schema.FullText == nil {
			errs = append(errs, ValidationError{
				Field:   "query",
				Code:    ErrUnsupportedField,
				Message: "This resource does not support full-text search.",
			})
		} else {
			vs.Query = req.Query
		}
	}

	// --- Cursor + Offset mutual exclusion ---
	if req.Cursor != "" && req.Offset != nil {
		errs = append(errs, ValidationError{
			Field:   "cursor",
			Code:    ErrMutuallyExclusive,
			Message: "cursor and offset are mutually exclusive.",
		})
	}

	// --- Filters ---
	for fieldName, ops := range req.Filter {
		fieldDef, ok := schema.Fields[fieldName]
		if !ok {
			errs = append(errs, ValidationError{
				Field:   "filter." + fieldName,
				Code:    ErrUnknownField,
				Message: fmt.Sprintf("Unknown field %q.", fieldName),
			})
			continue
		}

		for opName, rawVal := range ops {
			fieldPath := "filter." + fieldName + "." + opName

			// Check operator is valid for the type
			typeOps := operatorsForType(fieldDef.Type)
			if !slices.Contains(typeOps, opName) {
				errs = append(errs, ValidationError{
					Field:   fieldPath,
					Code:    ErrInvalidOperator,
					Message: fmt.Sprintf("Operator %q is not valid for type %v.", opName, fieldDef.Type),
				})
				continue
			}

			// Check operator is in field's allowed list (if specified)
			if len(fieldDef.Operators) > 0 && !slices.Contains(fieldDef.Operators, opName) {
				errs = append(errs, ValidationError{
					Field:   fieldPath,
					Code:    ErrInvalidOperator,
					Message: fmt.Sprintf("Operator %q is not permitted on field %q.", opName, fieldName),
				})
				continue
			}

			// is_null special handling
			if opName == "is_null" {
				if !fieldDef.Nullable {
					errs = append(errs, ValidationError{
						Field:   fieldPath,
						Code:    ErrNotNullable,
						Message: fmt.Sprintf("Field %q is not nullable.", fieldName),
					})
					continue
				}
				boolVal, ok := rawVal.(bool)
				if !ok {
					errs = append(errs, ValidationError{
						Field:   fieldPath,
						Code:    ErrInvalidType,
						Message: "is_null value must be a boolean.",
					})
					continue
				}
				vs.Filters = append(vs.Filters, ValidatedFilter{
					Field:    fieldName,
					Column:   resolveColumn(fieldName, fieldDef),
					Operator: opName,
					Value:    boolVal,
				})
				continue
			}

			// in/nin: validate array and size
			if opName == "in" || opName == "nin" {
				arr, ok := rawVal.([]any)
				if !ok {
					errs = append(errs, ValidationError{
						Field:   fieldPath,
						Code:    ErrInvalidType,
						Message: fmt.Sprintf("Operator %q requires an array value.", opName),
					})
					continue
				}
				maxSize := defaultMaxInSize(fieldDef, schema)
				if len(arr) > maxSize {
					errs = append(errs, ValidationError{
						Field:   fieldPath,
						Code:    ErrExceedsMax,
						Message: fmt.Sprintf("Array size %d exceeds maximum %d.", len(arr), maxSize),
					})
					continue
				}
				// Validate each element
				parsedVals := make([]any, 0, len(arr))
				valid := true
				for _, elem := range arr {
					parsed, err := parseValue(fieldDef, elem)
					if err != nil {
						errs = append(errs, ValidationError{
							Field:   fieldPath,
							Code:    ErrInvalidType,
							Message: err.Error(),
						})
						valid = false
						break
					}
					// Enum validation per element
					if fieldDef.Type == Enum {
						s, _ := parsed.(string)
						if !slices.Contains(fieldDef.AllowedValues, s) {
							errs = append(errs, ValidationError{
								Field:   fieldPath,
								Code:    ErrInvalidValue,
								Message: fmt.Sprintf("Value %q is not one of the allowed values.", s),
							})
							valid = false
							break
						}
					}
					parsedVals = append(parsedVals, parsed)
				}
				if !valid {
					continue
				}
				vs.Filters = append(vs.Filters, ValidatedFilter{
					Field:    fieldName,
					Column:   resolveColumn(fieldName, fieldDef),
					Operator: opName,
					Value:    parsedVals,
				})
				continue
			}

			// Scalar operators: parse and validate value
			parsed, err := parseValue(fieldDef, rawVal)
			if err != nil {
				errs = append(errs, ValidationError{
					Field:   fieldPath,
					Code:    ErrInvalidType,
					Message: err.Error(),
				})
				continue
			}

			// Enum value validation
			if fieldDef.Type == Enum {
				s, _ := parsed.(string)
				if !slices.Contains(fieldDef.AllowedValues, s) {
					errs = append(errs, ValidationError{
						Field:   fieldPath,
						Code:    ErrInvalidValue,
						Message: fmt.Sprintf("Value %q is not one of the allowed values.", s),
					})
					continue
				}
			}

			vs.Filters = append(vs.Filters, ValidatedFilter{
				Field:    fieldName,
				Column:   resolveColumn(fieldName, fieldDef),
				Operator: opName,
				Value:    parsed,
			})
		}
	}

	// --- Sort ---
	if len(req.Sort) > 3 {
		errs = append(errs, ValidationError{
			Field:   "sort",
			Code:    ErrExceedsMax,
			Message: "Maximum 3 sort directives allowed.",
		})
	} else {
		seenSortFields := make(map[string]bool)
		for i, sd := range req.Sort {
			fieldDef, ok := schema.Fields[sd.Field]
			if !ok {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("sort[%d].field", i),
					Code:    ErrUnknownField,
					Message: fmt.Sprintf("Unknown field %q.", sd.Field),
				})
				continue
			}
			if !fieldDef.Sortable {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("sort[%d].field", i),
					Code:    ErrUnsortableField,
					Message: fmt.Sprintf("Field %q is not sortable.", sd.Field),
				})
				continue
			}
			if seenSortFields[sd.Field] {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("sort[%d].field", i),
					Code:    ErrDuplicateSortField,
					Message: fmt.Sprintf("Duplicate sort field %q.", sd.Field),
				})
				continue
			}

			dir := SortDir(sd.Direction)
			if sd.Direction == "" {
				dir = Asc
			} else if dir != Asc && dir != Desc {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("sort[%d].direction", i),
					Code:    ErrInvalidValue,
					Message: fmt.Sprintf("Direction must be %q or %q.", Asc, Desc),
				})
				continue
			}

			seenSortFields[sd.Field] = true
			vs.Sort = append(vs.Sort, SortDirective{
				Field:  sd.Field,
				Column: resolveColumn(sd.Field, fieldDef),
				Dir:    dir,
			})
		}
	}

	// Apply default sort if none provided
	if len(vs.Sort) == 0 && len(req.Sort) == 0 {
		if req.Query != "" && schema.FullText != nil {
			// Relevance sort — handled by builder
			vs.RelevanceSort = true
		} else {
			vs.Sort = append(vs.Sort, schema.DefaultSort...)
		}
	}

	// Append tiebreaker if not already present
	pk := schema.PrimaryKey
	if pk == "" {
		pk = "id"
	}
	if !vs.RelevanceSort {
		hasPK := false
		for _, s := range vs.Sort {
			if s.Field == pk {
				hasPK = true
				break
			}
		}
		if !hasPK {
			vs.Sort = append(vs.Sort, SortDirective{
				Field:  pk,
				Column: pk,
				Dir:    Asc,
			})
		}
	}

	// --- Cursor ---
	if req.Cursor != "" && !vs.RelevanceSort {
		decoded, err := DecodeCursor(req.Cursor)
		if err != nil {
			errs = append(errs, ValidationError{
				Field:   "cursor",
				Code:    ErrInvalidCursor,
				Message: "Cursor is malformed or invalid.",
			})
		} else if decoded.Version != cursorVersion {
			errs = append(errs, ValidationError{
				Field:   "cursor",
				Code:    ErrInvalidCursor,
				Message: "Unsupported cursor version.",
			})
		} else if !CursorMatchesSort(decoded, vs.Sort) {
			errs = append(errs, ValidationError{
				Field:   "cursor",
				Code:    ErrInvalidCursor,
				Message: "Cursor sort signature does not match the request sort order.",
			})
		} else {
			vs.Cursor = decoded
		}
	}

	// --- Cursor vs relevance sort ---
	if req.Cursor != "" && vs.RelevanceSort {
		errs = append(errs, ValidationError{
			Field:   "cursor",
			Code:    ErrInvalidCursor,
			Message: "Cursor pagination is not supported with relevance sorting. Use offset instead.",
		})
	}

	// --- Limit ---
	if req.Limit != nil {
		vs.Limit = *req.Limit
	} else {
		vs.Limit = schema.DefaultLimit
	}
	if vs.Limit < 1 {
		vs.Limit = 1
	}
	if schema.MaxLimit > 0 && vs.Limit > schema.MaxLimit {
		vs.Limit = schema.MaxLimit
	}

	// --- Offset ---
	if req.Offset != nil {
		offset := *req.Offset
		maxOffset := schema.MaxOffset
		if maxOffset == 0 {
			maxOffset = 10000
		}
		if offset < 0 {
			offset = 0
		}
		if offset > maxOffset {
			offset = maxOffset
		}
		vs.Offset = &offset
	}

	// --- Fields ---
	for i, f := range req.Fields {
		// "id" is always included
		if f == "id" {
			continue
		}
		fieldDef, ok := schema.Fields[f]
		if !ok {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("fields[%d]", i),
				Code:    ErrUnknownField,
				Message: fmt.Sprintf("Unknown field %q.", f),
			})
			continue
		}
		if !isFieldSelectable(fieldDef) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("fields[%d]", i),
				Code:    ErrInvalidValue,
				Message: fmt.Sprintf("Field %q is not selectable.", f),
			})
			continue
		}
	}
	if len(req.Fields) > 0 && len(errs) == 0 {
		vs.Fields = req.Fields
	}

	return vs, errs
}

// parseValue parses a raw JSON value into the correct Go type for the field.
func parseValue(field FieldDef, raw any) (any, error) {
	switch field.Type {
	case String, Enum:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("expected string value, got %T", raw)
		}
		return s, nil
	case Numeric:
		switch v := raw.(type) {
		case float64:
			return v, nil
		case int:
			return float64(v), nil
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("expected numeric value, got %q", v)
			}
			return f, nil
		default:
			return nil, fmt.Errorf("expected numeric value, got %T", raw)
		}
	case Timestamp:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("expected RFC 3339 timestamp string, got %T", raw)
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("expected RFC 3339 timestamp, got %q", s)
		}
		return t, nil
	case Bool:
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("expected boolean value, got %T", raw)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("unsupported field type %d", field.Type)
	}
}
