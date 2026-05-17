// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"errors"
)

// Identity is the resolved authentication context — what the package
// proves about the caller. Authorization (role/scope resolution, allow-
// listing, RBAC) is a separate concern handled by callers, not by this
// package.
type Identity struct {
	// Issuer is the verified `iss` claim (e.g.
	// "https://accounts.google.com").
	Issuer string
	// Subject is the verified `sub` claim — the issuer-stable caller ID.
	Subject string
	// Email is the `email` claim, when present (typical for Google
	// service accounts and Workspace users).
	Email string
	// Claims is the full verified claim set, so callers can extract
	// issuer-specific fields (hd, email_verified, custom claims) without
	// re-parsing the token.
	Claims map[string]any
	// OBO carries on-behalf-of context parsed from request headers.
	OBO OnBehalfOf
}

// Verifier verifies a bearer token and returns the caller's verified
// Identity. Contract:
//
//	(*Identity, nil) → token verified
//	(nil, nil)       → not my token (wrong issuer / malformed for me)
//	                    → caller falls through to the next verifier
//	(nil, err)       → my token but invalid → caller stops the chain
//
// Verifiers do NOT do authorization. A verifier returning a non-nil
// Identity means "I cryptographically proved this caller's identity";
// it does NOT mean "this caller is allowed to do anything." Allow-
// listing, role lookup, and policy decisions belong in an authorization
// layer that consumes Identity.
type Verifier interface {
	Verify(ctx context.Context, token string) (*Identity, error)
}

// VerifierFunc adapts a plain function to the Verifier interface.
type VerifierFunc func(ctx context.Context, token string) (*Identity, error)

// Verify implements Verifier.
func (f VerifierFunc) Verify(ctx context.Context, token string) (*Identity, error) {
	return f(ctx, token)
}

// Chain composes multiple verifiers. Verify tries each in order:
//
//	first (info, nil)         → returned
//	first (nil, non-nil err)  → returned (short-circuits)
//	all (nil, nil)            → returns (nil, ErrNoVerifierAccepted)
type Chain []Verifier

// Verify implements Verifier.
func (c Chain) Verify(ctx context.Context, token string) (*Identity, error) {
	for _, v := range c {
		id, err := v.Verify(ctx, token)
		if err != nil {
			return nil, err
		}
		if id != nil {
			return id, nil
		}
	}
	return nil, ErrNoVerifierAccepted
}

// ErrNoVerifierAccepted is returned by Chain.Verify when no verifier
// claimed the token (all returned (nil, nil)).
var ErrNoVerifierAccepted = errors.New("identity: no verifier accepted the token")
