// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package search

import "github.com/danielgtaylor/huma/v2"

func (SearchRequest) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	s.Description = "Paginated search request with optional free-text query, field-level filters, sorting, and cursor- or offset-based pagination."
	return s
}

func (SortDirectiveInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	s.Description = "A sort directive specifying a field name and optional direction."
	return s
}

func (SearchResponse[T]) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	s.Description = "Paginated search response containing a page of results and a cursor for the next page."
	return s
}