package search

// SearchRequest is the JSON body of a POST /{resource}/search endpoint.
type SearchRequest struct {
	Query  string                    `json:"query,omitempty"`
	Filter map[string]map[string]any `json:"filter,omitempty"`
	Sort   []SortDirectiveInput      `json:"sort,omitempty"`
	Cursor string                    `json:"cursor,omitempty"`
	Offset *int                      `json:"offset,omitempty"`
	Limit  *int                      `json:"limit,omitempty"`
	Fields []string                  `json:"fields,omitempty"`
}

// SortDirectiveInput is the user-facing sort directive from the JSON body.
type SortDirectiveInput struct {
	Field     string `json:"field"`
	Direction string `json:"direction,omitempty"`
}

// SearchResponse is the standard response envelope for search endpoints.
type SearchResponse[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
	Limit      int    `json:"limit"`
}
