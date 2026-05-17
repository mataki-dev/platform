// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
)

// GoogleOption configures NewGoogleVerifier.
type GoogleOption func(*googleOptions)

type googleOptions struct {
	httpClient *http.Client
}

// WithGoogleHTTPClient overrides the http.Client used by idtoken for
// JWKS fetches. Used by tests; production callers should leave it unset.
func WithGoogleHTTPClient(c *http.Client) GoogleOption {
	return func(o *googleOptions) { o.httpClient = c }
}

const (
	googleIssuerBare  = "accounts.google.com"
	googleIssuerHTTPS = "https://accounts.google.com"
)

// NewGoogleVerifier returns a Verifier for Google-signed ID tokens.
//
// On Verify:
//  1. Issuer-sniff: parse the token without verifying. If iss is not a
//     Google issuer (or token is unparsable), return (nil, nil) so the
//     chain can fall through.
//  2. Full verify: idtoken.Validate(ctx, token, audience) checks
//     signature (JWKS cached internally by the library), audience, and
//     exp/nbf/iat.
//  3. Return *Identity populated with verified claims.
//
// The verifier does NOT decide whether the caller is allowed to do
// anything. Callers consume the Identity and apply their own
// authorization (allow-list, RBAC, etc.) downstream.
func NewGoogleVerifier(
	ctx context.Context,
	audience string,
	opts ...GoogleOption,
) (Verifier, error) {
	o := &googleOptions{}
	for _, opt := range opts {
		opt(o)
	}

	clientOpts := []idtoken.ClientOption{}
	if o.httpClient != nil {
		clientOpts = append(clientOpts, option.WithHTTPClient(o.httpClient))
	}
	validator, err := idtoken.NewValidator(ctx, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("identity: new validator: %w", err)
	}

	parser := jwt.NewParser(jwt.WithoutClaimsValidation())

	return VerifierFunc(func(ctx context.Context, token string) (*Identity, error) {
		// Issuer sniff: bail out without error if this token isn't Google's.
		unverified, _, perr := parser.ParseUnverified(token, jwt.MapClaims{})
		if perr != nil {
			return nil, nil
		}
		claims, ok := unverified.Claims.(jwt.MapClaims)
		if !ok {
			return nil, nil
		}
		iss, _ := claims["iss"].(string)
		if iss != googleIssuerBare && iss != googleIssuerHTTPS {
			return nil, nil
		}

		// Full verify.
		payload, err := validator.Validate(ctx, token, audience)
		if err != nil {
			return nil, fmt.Errorf("identity: validate google id token: %w", err)
		}

		email, _ := payload.Claims["email"].(string)

		return &Identity{
			Issuer:  payload.Issuer,
			Subject: payload.Subject,
			Email:   email,
			Claims:  payload.Claims,
		}, nil
	}), nil
}
