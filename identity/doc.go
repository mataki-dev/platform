// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

// Package identity provides authentication primitives for Mataki Go
// services. It deliberately does NOT do authorization — its job ends
// when it has cryptographically proved who the caller is.
//
// The package has three halves:
//
//   - A Verifier interface and Chain type for composing multiple
//     issuer-specific verifiers (Google ID tokens, Clerk JWTs, GitHub
//     OIDC, etc.). Verifiers fall through with (nil, nil) when a token
//     isn't theirs, letting chains stack cleanly.
//   - A net/http middleware (Middleware) and a huma wrapper that verify
//     the bearer token on each request and stash an Identity on the
//     request context for downstream handlers. Verification failures
//     render as 401 via the platform errors envelope.
//   - An outbound http.RoundTripper (NewTransport) that mints Google-
//     signed ID tokens against a target audience and attaches them
//     plus on-behalf-of headers on every request.
//
// Authorization — deciding what an authenticated caller is allowed to
// do — is a separate layer that consumes Identity from request context
// and applies whatever policy a given service needs (allow-list, RBAC,
// ABAC, etc.). The package does not ship an opinion on that.
package identity
