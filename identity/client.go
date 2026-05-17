// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

// NewTransport wraps base (default: http.DefaultTransport) with an
// auto-attaching Google ID token (Authorization: Bearer ...) and
// on-behalf-of headers. The token source uses the GCP metadata server
// on Cloud Run and ADC otherwise; oauth2.TokenSource handles caching
// and refresh.
//
// OBO precedence on each outbound request:
//  1. OBOFromContext(req.Context()), if set
//  2. defaultOBO supplied to NewTransport
//
// Headers are computed via OnBehalfOf.Headers() — Actor is always set
// (defaulting to "service"); Org / User / RequestID are included only
// when non-empty.
//
// Existing Authorization headers on the request are overwritten to
// prevent header smuggling.
func NewTransport(
	ctx context.Context,
	audience string,
	defaultOBO OnBehalfOf,
	base http.RoundTripper,
) (http.RoundTripper, error) {
	ts, err := idtoken.NewTokenSource(ctx, audience)
	if err != nil {
		return nil, fmt.Errorf("identity: new token source: %w", err)
	}
	return newTransport(ts, defaultOBO, base), nil
}

// NewClient is a convenience that returns an *http.Client configured
// with NewTransport. The returned client has no explicit timeout;
// callers should set one if needed.
func NewClient(
	ctx context.Context,
	audience string,
	defaultOBO OnBehalfOf,
) (*http.Client, error) {
	rt, err := NewTransport(ctx, audience, defaultOBO, http.DefaultTransport)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: rt}, nil
}

// newTransport is the test-friendly construction path: it takes an
// oauth2.TokenSource directly so tests can stub token issuance.
func newTransport(ts oauth2.TokenSource, defaultOBO OnBehalfOf, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &identityTransport{ts: ts, defaultOBO: defaultOBO, base: base}
}

type identityTransport struct {
	ts         oauth2.TokenSource
	defaultOBO OnBehalfOf
	base       http.RoundTripper
}

func (t *identityTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tok, err := t.ts.Token()
	if err != nil {
		return nil, fmt.Errorf("identity: fetch token: %w", err)
	}

	// Clone the request to avoid mutating the caller's instance.
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", "Bearer "+tok.AccessToken)

	obo := t.defaultOBO
	if ctxOBO, ok := OBOFromContext(req.Context()); ok {
		obo = ctxOBO
	}
	for k, v := range obo.Headers() {
		cloned.Header[k] = v
	}

	return t.base.RoundTrip(cloned)
}
