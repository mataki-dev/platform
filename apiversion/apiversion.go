// Package apiversion provides API version middleware for Chi and Huma.
//
// Set a date-based version string once at startup:
//
//	apiversion.SetCurrentVersion("2026-04-07")
//
// Register the Huma middleware:
//
//	api.UseMiddleware(apiversion.HumaMiddleware())
//
// The middleware reads the API-Version request header (falling back to
// the current version), stores it in context, and echoes it in the
// response. Supports Deprecation and Sunset headers for version
// lifecycle management.
package apiversion

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

type ctxKey struct{}

var current atomic.Pointer[string]

// SetCurrentVersion sets the global API version used as a fallback when no
// API-Version header is provided.
func SetCurrentVersion(v string) {
	current.Store(&v)
}

// Current returns the current global API version, or an empty string if not set.
func Current() string {
	if p := current.Load(); p != nil {
		return *p
	}
	return ""
}

// Middleware is an HTTP middleware that captures the API-Version header (or falls
// back to the current global version) and stores it in the request context.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := r.Header.Get("API-Version")
		if v == "" {
			v = Current()
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, v)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FromContext extracts the API version from the request context, or returns the
// current global version if not set in the context.
func FromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey{}).(string); ok && v != "" {
		return v
	}
	return Current()
}

// WriteVersionHeaders writes the API-Version, Deprecation, and Sunset response
// headers. The Deprecation header is set only if the requested version is
// greater than or equal to the deprecation date (lexicographically). The Sunset
// header is always set when an until date is provided, even if the request
// predates deprecation (advance notice to clients).
func WriteVersionHeaders(w http.ResponseWriter, requested, deprecated, until string) {
	w.Header().Set("API-Version", requested)
	if deprecated != "" && requested >= deprecated {
		// YYYY-MM-DD strings compare lexicographically, which is chronologically
		// correct for canonical date strings.
		w.Header().Set("Deprecation", "true")
	}
	// Sunset is set whenever until is known, even for pre-deprecation
	// requests — advance warning to clients.
	if until != "" {
		if t, err := time.Parse("2006-01-02", until); err == nil {
			w.Header().Set("Sunset", t.UTC().Format(http.TimeFormat))
		}
	}
}

// HumaMiddleware returns a Huma middleware that captures the API-Version header,
// falls back to the current global version, stores it in context, and sets the
// API-Version response header.
func HumaMiddleware() func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		v := ctx.Header("API-Version")
		if v == "" {
			v = Current()
		}
		ctx = huma.WithValue(ctx, ctxKey{}, v)
		ctx.SetHeader("API-Version", v)
		next(ctx)
	}
}
