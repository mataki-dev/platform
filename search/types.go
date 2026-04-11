// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package search

// SearchRequest is the JSON body of a POST /{resource}/search endpoint.
type SearchRequest struct {
	Query  string                    `json:"query,omitempty" doc:"Free-text search query matched against indexed text fields."`
	Filter map[string]map[string]any `json:"filter,omitempty" doc:"Field-level filter conditions. Each key is a field name; the value is a map of operator to value (e.g. {\"status\": {\"eq\": \"active\"}})."`
	Sort   []SortDirectiveInput      `json:"sort,omitempty" doc:"Sort order for the results. Multiple directives are applied in order."`
	Cursor string                    `json:"cursor,omitempty" doc:"Opaque cursor for fetching the next page. Returned as next_cursor in the previous response."`
	Offset *int                      `json:"offset,omitempty" doc:"Number of results to skip (offset-based pagination). Cannot be combined with cursor."`
	Limit  *int                      `json:"limit,omitempty" doc:"Maximum number of results to return per page."`
	Fields []string                  `json:"fields,omitempty" doc:"Subset of fields to include in each result object. Omit to return all fields."`
}

// SortDirectiveInput is the user-facing sort directive from the JSON body.
type SortDirectiveInput struct {
	Field     string `json:"field" doc:"Field name to sort by. Must be a sortable field for the resource."`
	Direction string `json:"direction,omitempty" doc:"Sort direction: 'asc' for ascending, 'desc' for descending. Defaults to 'asc'."`
}

// SearchResponse is the standard response envelope for search endpoints.
type SearchResponse[T any] struct {
	Data       []T    `json:"data" doc:"List of matching results for this page."`
	NextCursor string `json:"next_cursor,omitempty" doc:"Opaque cursor to pass as cursor in the next request to fetch the following page. Absent when there are no more results."`
	HasMore    bool   `json:"has_more" doc:"Whether there are additional results beyond this page."`
	Limit      int    `json:"limit" doc:"The effective page size used for this response."`
}