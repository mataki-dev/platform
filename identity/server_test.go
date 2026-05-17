// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddleware_MissingAuthHeader(t *testing.T) {
	mw := New(VerifierFunc(func(context.Context, string) (*Identity, error) {
		t.Error("verifier should not run when header is missing")
		return nil, nil
	}))
	called := false
	h := mw.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", rec.Code)
	}
	if called {
		t.Error("downstream handler should not be called")
	}
}

func TestMiddleware_MalformedAuthHeader(t *testing.T) {
	mw := New(VerifierFunc(func(context.Context, string) (*Identity, error) {
		t.Error("verifier should not run on malformed header")
		return nil, nil
	}))
	h := mw.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic foo")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", rec.Code)
	}
}

func TestMiddleware_ChainFallThroughReturns401(t *testing.T) {
	mw := New(Chain{
		VerifierFunc(func(context.Context, string) (*Identity, error) {
			return nil, nil
		}),
	})
	h := mw.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer x")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", rec.Code)
	}
}

func TestMiddleware_VerifierErrorReturns401(t *testing.T) {
	mw := New(VerifierFunc(func(context.Context, string) (*Identity, error) {
		return nil, errors.New("signature bad")
	}))
	h := mw.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer x")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", rec.Code)
	}
}

func TestMiddleware_SuccessStashesIdentityOnContext(t *testing.T) {
	wantID := &Identity{
		Issuer:  "https://accounts.google.com",
		Subject: "12345",
		Email:   "svc@example.iam.gserviceaccount.com",
	}
	mw := New(VerifierFunc(func(context.Context, string) (*Identity, error) {
		return wantID, nil
	}))
	var got *Identity
	h := mw.Handler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		id, ok := FromContext(r.Context())
		if !ok {
			t.Error("expected identity on context")
		}
		got = id
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer x")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d want 200", rec.Code)
	}
	if got != wantID {
		t.Errorf("got %+v want %+v", got, wantID)
	}
}

func TestMiddleware_SuccessParsesOBOFromHeaders(t *testing.T) {
	mw := New(VerifierFunc(func(context.Context, string) (*Identity, error) {
		return &Identity{}, nil
	}))
	var gotOBO OnBehalfOf
	h := mw.Handler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		id, _ := FromContext(r.Context())
		gotOBO = id.OBO
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("X-On-Behalf-Of-Actor", "user")
	req.Header.Set("X-On-Behalf-Of-Org", "org_abc")
	req.Header.Set("X-On-Behalf-Of-User", "user_xyz")
	h.ServeHTTP(rec, req)
	if gotOBO.Actor != ActorUser {
		t.Errorf("actor: got %q want user", gotOBO.Actor)
	}
	if gotOBO.OrgID != "org_abc" {
		t.Errorf("org: got %q", gotOBO.OrgID)
	}
	if gotOBO.UserID != "user_xyz" {
		t.Errorf("user: got %q", gotOBO.UserID)
	}
}

func TestMiddleware_ErrorBodyIsJSON(t *testing.T) {
	mw := New(VerifierFunc(func(context.Context, string) (*Identity, error) {
		return nil, errors.New("bad")
	}))
	h := mw.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer x")
	h.ServeHTTP(rec, req)
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type: got %q want application/json", ct)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), `"type"`) || !strings.Contains(string(body), `"message"`) {
		t.Errorf("body does not look like SemanticError envelope: %q", string(body))
	}
}
