// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"net/http"
)

// ActorKind identifies who initiated a request, on whose behalf a
// service is acting.
type ActorKind string

const (
	ActorService ActorKind = "service"
	ActorUser    ActorKind = "user"
	ActorSystem  ActorKind = "system"
)

// OnBehalfOf carries tenant/user context to forward to a downstream
// service. Receivers trust these values because the bearer token has
// already proven the caller is an authorized service.
//
// Role and scopes are NOT declared here — the receiver resolves them
// from the verified caller identity via its Lookup.
type OnBehalfOf struct {
	Actor     ActorKind
	OrgID     string
	UserID    string
	RequestID string
}

// Headers returns the on-behalf-of headers for attaching to an outbound
// HTTP request. Actor is always included (defaulting to "service");
// other fields are included only when non-empty.
func (o OnBehalfOf) Headers() http.Header {
	h := http.Header{}
	actor := o.Actor
	if actor == "" {
		actor = ActorService
	}
	h.Set("X-On-Behalf-Of-Actor", string(actor))
	if o.OrgID != "" {
		h.Set("X-On-Behalf-Of-Org", o.OrgID)
	}
	if o.UserID != "" {
		h.Set("X-On-Behalf-Of-User", o.UserID)
	}
	if o.RequestID != "" {
		h.Set("X-Request-Id", o.RequestID)
	}
	return h
}

// ReadOnBehalfOf parses an inbound request's headers into an OnBehalfOf
// struct. Actor defaults to ActorService when the header is missing or
// empty.
func ReadOnBehalfOf(h http.Header) OnBehalfOf {
	actor := ActorKind(h.Get("X-On-Behalf-Of-Actor"))
	if actor == "" {
		actor = ActorService
	}
	return OnBehalfOf{
		Actor:     actor,
		OrgID:     h.Get("X-On-Behalf-Of-Org"),
		UserID:    h.Get("X-On-Behalf-Of-User"),
		RequestID: h.Get("X-Request-Id"),
	}
}

type oboCtxKey struct{}

// WithOBO returns a new context carrying the given OnBehalfOf. Outbound
// transports look it up to override their default OBO per-request.
func WithOBO(ctx context.Context, o OnBehalfOf) context.Context {
	return context.WithValue(ctx, oboCtxKey{}, o)
}

// OBOFromContext extracts an OnBehalfOf set by WithOBO. ok is false if
// none was set.
func OBOFromContext(ctx context.Context) (OnBehalfOf, bool) {
	o, ok := ctx.Value(oboCtxKey{}).(OnBehalfOf)
	return o, ok
}
