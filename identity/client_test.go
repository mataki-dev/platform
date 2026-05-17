// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type staticToken struct{ tok string }

func (s staticToken) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: s.tok, Expiry: time.Now().Add(time.Hour)}, nil
}

func TestTransport_AttachesBearerAndDefaultOBO(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r
	}))
	defer srv.Close()

	rt := newTransport(staticToken{"abc"}, OnBehalfOf{Actor: ActorService, OrgID: "default_org"}, http.DefaultTransport)
	c := &http.Client{Transport: rt}

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	_, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	if got.Header.Get("Authorization") != "Bearer abc" {
		t.Errorf("authorization: got %q", got.Header.Get("Authorization"))
	}
	if got.Header.Get("X-On-Behalf-Of-Actor") != "service" {
		t.Errorf("actor: got %q", got.Header.Get("X-On-Behalf-Of-Actor"))
	}
	if got.Header.Get("X-On-Behalf-Of-Org") != "default_org" {
		t.Errorf("org: got %q", got.Header.Get("X-On-Behalf-Of-Org"))
	}
}

func TestTransport_ContextOBOOverridesDefault(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r
	}))
	defer srv.Close()

	rt := newTransport(staticToken{"abc"}, OnBehalfOf{Actor: ActorService, OrgID: "default_org"}, http.DefaultTransport)
	c := &http.Client{Transport: rt}

	ctx := WithOBO(context.Background(), OnBehalfOf{
		Actor:  ActorUser,
		OrgID:  "override_org",
		UserID: "user_a",
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	if got.Header.Get("X-On-Behalf-Of-Actor") != "user" {
		t.Errorf("actor: got %q want user", got.Header.Get("X-On-Behalf-Of-Actor"))
	}
	if got.Header.Get("X-On-Behalf-Of-Org") != "override_org" {
		t.Errorf("org: got %q want override_org", got.Header.Get("X-On-Behalf-Of-Org"))
	}
	if got.Header.Get("X-On-Behalf-Of-User") != "user_a" {
		t.Errorf("user: got %q want user_a", got.Header.Get("X-On-Behalf-Of-User"))
	}
}

func TestTransport_NoOBOWhenAllEmpty(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r
	}))
	defer srv.Close()

	rt := newTransport(staticToken{"abc"}, OnBehalfOf{}, http.DefaultTransport)
	c := &http.Client{Transport: rt}

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	_, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	if got.Header.Get("X-On-Behalf-Of-Actor") != "service" {
		t.Errorf("actor: got %q want service", got.Header.Get("X-On-Behalf-Of-Actor"))
	}
	if got.Header.Get("X-On-Behalf-Of-Org") != "" {
		t.Errorf("expected no org header, got %q", got.Header.Get("X-On-Behalf-Of-Org"))
	}
	if got.Header.Get("X-On-Behalf-Of-User") != "" {
		t.Errorf("expected no user header, got %q", got.Header.Get("X-On-Behalf-Of-User"))
	}
}

func TestTransport_OverwritesExistingAuthorization(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r
	}))
	defer srv.Close()

	rt := newTransport(staticToken{"abc"}, OnBehalfOf{}, http.DefaultTransport)
	c := &http.Client{Transport: rt}

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("Authorization", "Bearer leaked")
	_, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	if got.Header.Get("Authorization") != "Bearer abc" {
		t.Errorf("expected token to overwrite existing header, got %q", got.Header.Get("Authorization"))
	}
}
