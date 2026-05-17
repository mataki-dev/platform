// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"net/http"
	"testing"
)

func TestOnBehalfOf_Headers_DefaultActorOnly(t *testing.T) {
	o := OnBehalfOf{}
	h := o.Headers()
	if got := h.Get("X-On-Behalf-Of-Actor"); got != "service" {
		t.Errorf("expected default actor=service, got %q", got)
	}
	if got := h.Get("X-On-Behalf-Of-Org"); got != "" {
		t.Errorf("expected no org header, got %q", got)
	}
	if got := h.Get("X-On-Behalf-Of-User"); got != "" {
		t.Errorf("expected no user header, got %q", got)
	}
	if got := h.Get("X-Request-Id"); got != "" {
		t.Errorf("expected no request id header, got %q", got)
	}
}

func TestOnBehalfOf_Headers_AllFields(t *testing.T) {
	o := OnBehalfOf{
		Actor:     ActorUser,
		OrgID:     "org_123",
		UserID:    "user_abc",
		RequestID: "req_xyz",
	}
	h := o.Headers()
	if got := h.Get("X-On-Behalf-Of-Actor"); got != "user" {
		t.Errorf("actor: got %q want user", got)
	}
	if got := h.Get("X-On-Behalf-Of-Org"); got != "org_123" {
		t.Errorf("org: got %q want org_123", got)
	}
	if got := h.Get("X-On-Behalf-Of-User"); got != "user_abc" {
		t.Errorf("user: got %q want user_abc", got)
	}
	if got := h.Get("X-Request-Id"); got != "req_xyz" {
		t.Errorf("request id: got %q want req_xyz", got)
	}
}

func TestReadOnBehalfOf_RoundTrip(t *testing.T) {
	original := OnBehalfOf{
		Actor:     ActorUser,
		OrgID:     "org_123",
		UserID:    "user_abc",
		RequestID: "req_xyz",
	}
	h := original.Headers()
	roundTripped := ReadOnBehalfOf(h)
	if roundTripped != original {
		t.Errorf("round trip mismatch: got %+v want %+v", roundTripped, original)
	}
}

func TestReadOnBehalfOf_DefaultsActorWhenMissing(t *testing.T) {
	h := http.Header{}
	o := ReadOnBehalfOf(h)
	if o.Actor != ActorService {
		t.Errorf("expected default actor=service, got %q", o.Actor)
	}
}

func TestOBOContextRoundTrip(t *testing.T) {
	o := OnBehalfOf{Actor: ActorUser, OrgID: "org_xyz"}
	ctx := WithOBO(context.Background(), o)
	got, ok := OBOFromContext(ctx)
	if !ok {
		t.Fatal("expected OBO to be present in context")
	}
	if got != o {
		t.Errorf("got %+v want %+v", got, o)
	}
}

func TestOBOFromContext_EmptyWhenAbsent(t *testing.T) {
	_, ok := OBOFromContext(context.Background())
	if ok {
		t.Error("expected ok=false when OBO absent")
	}
}
