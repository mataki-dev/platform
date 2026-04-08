package search

// BuildResponse handles the limit+1 trim, has_more detection, and
// cursor encoding for the last row.
//
// extractFn maps a product's row struct to the map of values needed
// for cursor encoding (sort field values).
func BuildResponse[T any](rows []T, req ValidatedSearch, extractFn func(T) map[string]any) (*SearchResponse[T], error) {
	resp := &SearchResponse[T]{
		Limit: req.Limit,
	}

	if len(rows) > req.Limit {
		resp.HasMore = true
		rows = rows[:req.Limit]
	}

	resp.Data = rows

	// Encode cursor from the last row (only for non-relevance sorts)
	if resp.HasMore && !req.RelevanceSort && len(rows) > 0 {
		lastRow := extractFn(rows[len(rows)-1])
		resp.NextCursor = EncodeCursor(lastRow, req.Sort)
	}

	// Ensure Data is never nil (always empty array in JSON)
	if resp.Data == nil {
		resp.Data = make([]T, 0)
	}

	return resp, nil
}
