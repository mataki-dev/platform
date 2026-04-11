// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package ingest

import (
	"time"

	"github.com/mataki-dev/platform/search"
	"github.com/mataki-dev/platform/strongbox"
)

// secretSearchSchema defines the searchable fields for secrets.
var secretSearchSchema = search.ResourceSchema{
	Fields: map[string]search.FieldDef{
		"ref": {
			Type:      search.String,
			Column:    "ref",
			Sortable:  true,
			Operators: []string{"eq", "contains", "in"},
		},
		"version": {
			Type:     search.Numeric,
			Column:   "version",
			Sortable: true,
		},
		"created_at": {
			Type:     search.Timestamp,
			Column:   "created_at",
			Sortable: true,
		},
		"updated_at": {
			Type:     search.Timestamp,
			Column:   "updated_at",
			Sortable: true,
		},
		"expires_at": {
			Type:     search.Timestamp,
			Column:   "expires_at",
			Sortable: true,
			Nullable: true,
		},
	},
	DefaultSort:  []search.SortDirective{{Field: "ref", Column: "ref", Dir: search.Asc}},
	DefaultLimit: 100,
	MaxLimit:     1000,
	PrimaryKey:   "ref",
}

// SearchSecretsResponse is the response from POST /{environment}/secrets/search.
type SearchSecretsResponse struct {
	Secrets []IngestHeader `json:"secrets"`
	Cursor  string         `json:"cursor,omitempty"`
	HasMore bool           `json:"has_more"`
	Limit   int            `json:"limit"`
}

// toListOptions translates a ValidatedSearch into strongbox.ListOptions.
func toListOptions(vs search.ValidatedSearch) strongbox.ListOptions {
	opts := strongbox.ListOptions{
		Limit: vs.Limit,
	}

	// Extract prefix from a "ref" "contains" filter.
	for _, f := range vs.Filters {
		if f.Field == "ref" && f.Operator == "contains" {
			if s, ok := f.Value.(string); ok {
				opts.Prefix = s
			}
		}
	}

	// Map first sort directive.
	if len(vs.Sort) > 0 {
		opts.SortField = vs.Sort[0].Column
		opts.SortDir = string(vs.Sort[0].Dir)
	}

	return opts
}

// headerFromStrongbox converts a strongbox.SecretHeader to an IngestHeader.
func headerFromStrongbox(h strongbox.SecretHeader) IngestHeader {
	ih := IngestHeader{
		Key:       string(h.Ref),
		Version:   h.Version,
		Metadata:  h.Metadata,
		CreatedAt: h.CreatedAt,
		UpdatedAt: h.UpdatedAt,
		ExpiresAt: h.ExpiresAt,
	}
	if ih.CreatedAt.IsZero() {
		ih.CreatedAt = time.Time{}
	}
	return ih
}