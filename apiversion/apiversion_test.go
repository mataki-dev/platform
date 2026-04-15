package apiversion_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mataki-dev/platform/apiversion"
)

func TestSetCurrentVersionAndCurrent(t *testing.T) {
	apiversion.SetCurrentVersion("2025-01-01")
	got := apiversion.Current()
	if got != "2025-01-01" {
		t.Fatalf("Current() = %q, want %q", got, "2025-01-01")
	}
}

func TestFromContext_WithValueInContext(t *testing.T) {
	var captured string
	handler := apiversion.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = apiversion.FromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("API-Version", "2025-06-15")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if captured != "2025-06-15" {
		t.Fatalf("FromContext() = %q, want %q", captured, "2025-06-15")
	}
}

func TestFromContext_WithoutValueReturnsCurrent(t *testing.T) {
	apiversion.SetCurrentVersion("2025-03-01")
	got := apiversion.FromContext(context.Background())
	if got != "2025-03-01" {
		t.Fatalf("FromContext(empty ctx) = %q, want %q", got, "2025-03-01")
	}
}

func TestMiddleware_FallsBackToCurrent(t *testing.T) {
	apiversion.SetCurrentVersion("2025-04-01")

	var captured string
	handler := apiversion.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = apiversion.FromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No API-Version header
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if captured != "2025-04-01" {
		t.Fatalf("FromContext() = %q, want %q (current)", captured, "2025-04-01")
	}
}

func TestWriteVersionHeaders_SetsAPIVersion(t *testing.T) {
	w := httptest.NewRecorder()
	apiversion.WriteVersionHeaders(w, "2025-01-01", "", "")
	if got := w.Header().Get("API-Version"); got != "2025-01-01" {
		t.Fatalf("API-Version header = %q, want %q", got, "2025-01-01")
	}
}

func TestWriteVersionHeaders_Deprecation(t *testing.T) {
	t.Run("requested equals deprecated", func(t *testing.T) {
		w := httptest.NewRecorder()
		apiversion.WriteVersionHeaders(w, "2025-01-01", "2025-01-01", "")
		if got := w.Header().Get("Deprecation"); got != "true" {
			t.Fatalf("Deprecation header = %q, want %q", got, "true")
		}
	})

	t.Run("requested after deprecated", func(t *testing.T) {
		w := httptest.NewRecorder()
		apiversion.WriteVersionHeaders(w, "2025-06-01", "2025-01-01", "")
		if got := w.Header().Get("Deprecation"); got != "true" {
			t.Fatalf("Deprecation header = %q, want %q", got, "true")
		}
	})

	t.Run("requested before deprecated", func(t *testing.T) {
		w := httptest.NewRecorder()
		apiversion.WriteVersionHeaders(w, "2024-12-01", "2025-01-01", "")
		if got := w.Header().Get("Deprecation"); got != "" {
			t.Fatalf("Deprecation header should not be set, got %q", got)
		}
	})
}

func TestWriteVersionHeaders_Sunset(t *testing.T) {
	t.Run("set when until provided", func(t *testing.T) {
		w := httptest.NewRecorder()
		apiversion.WriteVersionHeaders(w, "2025-01-01", "", "2025-12-31")
		sunset := w.Header().Get("Sunset")
		if sunset == "" {
			t.Fatal("Sunset header should be set when until is provided")
		}
		if _, err := time.Parse(http.TimeFormat, sunset); err != nil {
			t.Fatalf("Sunset header %q is not a valid HTTP date: %v", sunset, err)
		}
	})

	t.Run("not set when until empty", func(t *testing.T) {
		w := httptest.NewRecorder()
		apiversion.WriteVersionHeaders(w, "2025-01-01", "2025-01-01", "")
		if got := w.Header().Get("Sunset"); got != "" {
			t.Fatalf("Sunset header should not be set, got %q", got)
		}
	})

	t.Run("set even before deprecation as advance notice", func(t *testing.T) {
		w := httptest.NewRecorder()
		apiversion.WriteVersionHeaders(w, "2024-01-01", "2025-01-01", "2025-06-01")
		sunset := w.Header().Get("Sunset")
		if sunset == "" {
			t.Fatal("Sunset header should be set as advance notice even before deprecation")
		}
		deprecation := w.Header().Get("Deprecation")
		if deprecation != "" {
			t.Fatal("Deprecation should not be set for pre-deprecation request")
		}
	})
}
