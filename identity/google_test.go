// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// makeUnsignedToken builds a JWT with the given claims, signed with HS256
// and a junk key. The signature won't verify but the iss claim parses out
// cleanly via jwt.ParseUnverified — which is all the issuer-sniff needs.
func makeUnsignedToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte("test"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func TestGoogleVerifier_SkipsNonGoogleIssuer(t *testing.T) {
	v, err := NewGoogleVerifier(context.Background(), "https://api.example.com")
	if err != nil {
		t.Fatalf("NewGoogleVerifier: %v", err)
	}
	tok := makeUnsignedToken(t, jwt.MapClaims{"iss": "https://token.actions.githubusercontent.com"})
	id, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("expected (nil, nil), got err %v", err)
	}
	if id != nil {
		t.Errorf("expected nil identity for non-Google issuer, got %+v", id)
	}
}

func TestGoogleVerifier_SkipsMalformedToken(t *testing.T) {
	v, err := NewGoogleVerifier(context.Background(), "https://api.example.com")
	if err != nil {
		t.Fatalf("NewGoogleVerifier: %v", err)
	}
	id, err := v.Verify(context.Background(), "not-a-jwt")
	if err != nil {
		t.Fatalf("expected (nil, nil) for unparsable, got err %v", err)
	}
	if id != nil {
		t.Errorf("expected nil identity, got %+v", id)
	}
}

func TestGoogleVerifier_AcceptsBothIssuerForms(t *testing.T) {
	for _, iss := range []string{"accounts.google.com", "https://accounts.google.com"} {
		t.Run(iss, func(t *testing.T) {
			v, err := NewGoogleVerifier(context.Background(), "https://api.example.com")
			if err != nil {
				t.Fatalf("NewGoogleVerifier: %v", err)
			}
			tok := makeUnsignedToken(t, jwt.MapClaims{"iss": iss})
			id, err := v.Verify(context.Background(), tok)
			if err == nil {
				t.Error("expected signature verification to fail")
			}
			if id != nil {
				t.Error("expected nil identity on signature failure")
			}
		})
	}
}

// testJWKS spins up an httptest server that serves a JWKS containing
// one RSA public key under "test-kid". The matching private key is
// returned for signing tokens.
func testJWKS(t *testing.T) (*rsa.PrivateKey, *httptest.Server) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen rsa: %v", err)
	}
	jwks := map[string]any{
		"keys": []any{
			map[string]any{
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"kid": "test-kid",
				"n":   base64.RawURLEncoding.EncodeToString(priv.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes()),
			},
		},
	}
	body, _ := json.Marshal(jwks)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return priv, srv
}

// signRS256 signs claims with the given key and "test-kid".
func signRS256(t *testing.T, priv *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "test-kid"
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("sign rs256: %v", err)
	}
	return s
}

// jwksRedirectClient returns an *http.Client whose RoundTripper rewrites
// any request to googleapis.com to the test JWKS server.
func jwksRedirectClient(srv *httptest.Server) *http.Client {
	base := srv.URL
	return &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "www.googleapis.com" || r.URL.Host == "oauth2.googleapis.com" {
				newURL := base + r.URL.Path
				newReq, _ := http.NewRequestWithContext(r.Context(), r.Method, newURL, r.Body)
				for k, v := range r.Header {
					newReq.Header[k] = v
				}
				return http.DefaultTransport.RoundTrip(newReq)
			}
			return http.DefaultTransport.RoundTrip(r)
		}),
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestGoogleVerifier_FullVerifySuccess(t *testing.T) {
	priv, srv := testJWKS(t)
	now := time.Now().Unix()
	tok := signRS256(t, priv, jwt.MapClaims{
		"iss":   "https://accounts.google.com",
		"aud":   "https://api.example.com",
		"sub":   "12345",
		"email": "svc@example.iam.gserviceaccount.com",
		"iat":   now,
		"exp":   now + 3600,
	})

	v, err := NewGoogleVerifier(
		context.Background(),
		"https://api.example.com",
		WithGoogleHTTPClient(jwksRedirectClient(srv)),
	)
	if err != nil {
		t.Fatalf("NewGoogleVerifier: %v", err)
	}

	id, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id == nil {
		t.Fatal("expected identity, got nil")
	}
	if id.Subject != "12345" {
		t.Errorf("subject: got %q", id.Subject)
	}
	if id.Email != "svc@example.iam.gserviceaccount.com" {
		t.Errorf("email: got %q", id.Email)
	}
	if id.Issuer != "https://accounts.google.com" {
		t.Errorf("issuer: got %q", id.Issuer)
	}
	if id.Claims["sub"] != "12345" {
		t.Errorf("claims missing sub: %+v", id.Claims)
	}
}

func TestGoogleVerifier_AudienceMismatch(t *testing.T) {
	priv, srv := testJWKS(t)
	now := time.Now().Unix()
	tok := signRS256(t, priv, jwt.MapClaims{
		"iss":   "https://accounts.google.com",
		"aud":   "https://other.example.com",
		"sub":   "12345",
		"email": "svc@example.iam.gserviceaccount.com",
		"iat":   now,
		"exp":   now + 3600,
	})

	v, _ := NewGoogleVerifier(
		context.Background(),
		"https://api.example.com",
		WithGoogleHTTPClient(jwksRedirectClient(srv)),
	)
	id, err := v.Verify(context.Background(), tok)
	if id != nil {
		t.Errorf("expected nil identity, got %+v", id)
	}
	if err == nil {
		t.Error("expected error on audience mismatch")
	}
}
