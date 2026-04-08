package search

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// DecodedCursor is the internal representation of a cursor token.
type DecodedCursor struct {
	Version int               `json:"v"`
	Sort    []cursorSortEntry `json:"s"`
	Values  []any             `json:"-"` // populated during decode
}

type cursorSortEntry struct {
	Field     string `json:"f"`
	Direction string `json:"d"`
	Value     any    `json:"val"`
}

const cursorVersion = 1

// EncodeCursor builds a cursor from the last row in a result set.
func EncodeCursor(lastRow map[string]any, sort []SortDirective) string {
	if len(sort) == 0 || lastRow == nil {
		return ""
	}

	c := DecodedCursor{
		Version: cursorVersion,
		Sort:    make([]cursorSortEntry, len(sort)),
	}
	for i, s := range sort {
		c.Sort[i] = cursorSortEntry{
			Field:     s.Field,
			Direction: string(s.Dir),
			Value:     lastRow[s.Field],
		}
	}

	return encodeCursorToString(&c)
}

// EncodeCursorRaw encodes a DecodedCursor back to a token string.
// Primarily used in tests.
func EncodeCursorRaw(c *DecodedCursor) string {
	return encodeCursorToString(c)
}

func encodeCursorToString(c *DecodedCursor) string {
	data, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

// DecodeCursor parses a cursor token string.
func DecodeCursor(token string) (*DecodedCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: malformed encoding: %w", err)
	}

	var c DecodedCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("invalid cursor: malformed data: %w", err)
	}

	// Extract values from sort entries
	c.Values = make([]any, len(c.Sort))
	for i, s := range c.Sort {
		c.Values[i] = s.Value
	}

	return &c, nil
}

// CursorMatchesSort checks whether a decoded cursor is compatible
// with the given sort directives. The field names and directions must match.
func CursorMatchesSort(c *DecodedCursor, sort []SortDirective) bool {
	if len(c.Sort) != len(sort) {
		return false
	}
	for i, s := range sort {
		if c.Sort[i].Field != s.Field {
			return false
		}
		if c.Sort[i].Direction != string(s.Dir) {
			return false
		}
	}
	return true
}
