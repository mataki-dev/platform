// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package search

import (
	"fmt"
	"strings"
)

// SQLFragment contains the generated SQL clauses and parameters
// for a search query.
type SQLFragment struct {
	// Where is the AND-joined clause string, including leading "AND"
	// per filter. Empty string if no filters.
	Where string

	// CursorWhere is the cursor seek clause, including leading "AND".
	// Empty if no cursor.
	CursorWhere string

	// OrderBy is the comma-separated ORDER BY expression.
	OrderBy string

	// OffsetClause is "OFFSET N" when offset pagination is used.
	// Empty string if no offset.
	OffsetClause string

	// Limit is the resolved limit + 1 (for has_more detection).
	Limit int

	// Args contains the parameter values in positional order.
	Args []any

	// ArgCount is the total number of positional parameters.
	ArgCount int
}

// BuildSQL generates SQL clauses from a ValidatedSearch.
// argOffset is the starting positional parameter index ($argOffset).
// Uses "t" as the default table alias.
func BuildSQL(vs ValidatedSearch, argOffset int) SQLFragment {
	return buildSQL(vs, argOffset, "t", nil)
}

// BuildSQLWithAlias generates SQL clauses with a custom table alias.
func BuildSQLWithAlias(vs ValidatedSearch, argOffset int, alias string) SQLFragment {
	return buildSQL(vs, argOffset, alias, nil)
}

// BuildSQLWithFullText generates SQL clauses including full-text search support.
func BuildSQLWithFullText(vs ValidatedSearch, argOffset int, ft *FullTextConfig) SQLFragment {
	return buildSQL(vs, argOffset, "t", ft)
}

func buildSQL(vs ValidatedSearch, argOffset int, alias string, ft *FullTextConfig) SQLFragment {
	var frag SQLFragment
	var whereParts []string
	paramIdx := argOffset

	// Full-text search WHERE clause
	if vs.Query != "" && ft != nil {
		lang := resolveFullTextLanguage(ft)
		whereParts = append(whereParts,
			fmt.Sprintf("%s.%s @@ plainto_tsquery('%s', $%d)", alias, ft.Column, lang, paramIdx))
		frag.Args = append(frag.Args, vs.Query)
		paramIdx++
	}

	// Filter clauses
	for _, f := range vs.Filters {
		col := fmt.Sprintf("%s.%s", alias, f.Column)

		switch f.Operator {
		case "is_null":
			if f.Value.(bool) {
				whereParts = append(whereParts, col+" IS NULL")
			} else {
				whereParts = append(whereParts, col+" IS NOT NULL")
			}

		case "contains":
			whereParts = append(whereParts, fmt.Sprintf("%s ILIKE $%d", col, paramIdx))
			frag.Args = append(frag.Args, escapeContains(f.Value.(string)))
			paramIdx++

		case "in":
			whereParts = append(whereParts, fmt.Sprintf("%s = ANY($%d)", col, paramIdx))
			frag.Args = append(frag.Args, f.Value)
			paramIdx++

		case "nin":
			whereParts = append(whereParts, fmt.Sprintf("%s != ALL($%d)", col, paramIdx))
			frag.Args = append(frag.Args, f.Value)
			paramIdx++

		default:
			op := sqlOp(f.Operator)
			whereParts = append(whereParts, fmt.Sprintf("%s %s $%d", col, op, paramIdx))
			frag.Args = append(frag.Args, f.Value)
			paramIdx++
		}
	}

	// Build WHERE string
	if len(whereParts) > 0 {
		parts := make([]string, len(whereParts))
		for i, p := range whereParts {
			parts[i] = "AND " + p
		}
		frag.Where = strings.Join(parts, " ")
	}

	// Cursor WHERE
	if vs.Cursor != nil {
		cursorWhere, cursorArgs := buildCursorWhere(vs.Cursor, vs.Sort, alias, paramIdx)
		if cursorWhere != "" {
			frag.CursorWhere = "AND " + cursorWhere
			frag.Args = append(frag.Args, cursorArgs...)
			paramIdx += len(cursorArgs)
		}
	}

	// ORDER BY
	if vs.RelevanceSort && ft != nil {
		lang := resolveFullTextLanguage(ft)
		rankExpr := fmt.Sprintf("ts_rank(%s.%s, plainto_tsquery('%s', $%d))", alias, ft.Column, lang, paramIdx)
		frag.Args = append(frag.Args, vs.Query)
		paramIdx++
		frag.OrderBy = rankExpr + " DESC, " + alias + ".id ASC"
	} else {
		orderParts := make([]string, len(vs.Sort))
		for i, s := range vs.Sort {
			dir := strings.ToUpper(string(s.Dir))
			orderParts[i] = fmt.Sprintf("%s.%s %s", alias, s.Column, dir)
		}
		frag.OrderBy = strings.Join(orderParts, ", ")
	}

	// Offset
	if vs.Offset != nil {
		frag.OffsetClause = fmt.Sprintf("OFFSET %d", *vs.Offset)
	}

	// Limit (+1 for has_more detection)
	frag.Limit = vs.Limit + 1
	frag.ArgCount = paramIdx - argOffset

	return frag
}

// buildCursorWhere generates the cursor seek WHERE clause.
func buildCursorWhere(cursor *DecodedCursor, sort []SortDirective, alias string, startParam int) (string, []any) {
	if cursor == nil || len(cursor.Values) == 0 {
		return "", nil
	}

	// Check if all directions are the same
	allSame := true
	firstDir := sort[0].Dir
	for _, s := range sort[1:] {
		if s.Dir != firstDir {
			allSame = false
			break
		}
	}

	paramIdx := startParam
	var args []any

	if allSame {
		// Row-value comparison
		cols := make([]string, len(sort))
		params := make([]string, len(sort))
		for i, s := range sort {
			cols[i] = fmt.Sprintf("%s.%s", alias, s.Column)
			params[i] = fmt.Sprintf("$%d", paramIdx)
			args = append(args, cursor.Values[i])
			paramIdx++
		}
		op := ">"
		if firstDir == Desc {
			op = "<"
		}
		return fmt.Sprintf("(%s) %s (%s)", strings.Join(cols, ", "), op, strings.Join(params, ", ")), args
	}

	// Mixed-direction: expanded boolean form
	var orParts []string
	for i := range sort {
		var andParts []string
		for j := 0; j < i; j++ {
			andParts = append(andParts, fmt.Sprintf("%s.%s = $%d", alias, sort[j].Column, paramIdx))
			args = append(args, cursor.Values[j])
			paramIdx++
		}
		op := ">"
		if sort[i].Dir == Desc {
			op = "<"
		}
		andParts = append(andParts, fmt.Sprintf("%s.%s %s $%d", alias, sort[i].Column, op, paramIdx))
		args = append(args, cursor.Values[i])
		paramIdx++

		orParts = append(orParts, "("+strings.Join(andParts, " AND ")+")")
	}

	return strings.Join(orParts, " OR "), args
}

func sqlOp(op string) string {
	switch op {
	case "eq":
		return "="
	case "neq":
		return "!="
	case "gt":
		return ">"
	case "gte":
		return ">="
	case "lt":
		return "<"
	case "lte":
		return "<="
	default:
		return "="
	}
}

// escapeContains escapes %, _, and \ for ILIKE and wraps in %.
func escapeContains(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return "%" + s + "%"
}