// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package ingest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// requireTLS rejects non-TLS requests with 403 Forbidden.
// A request is considered TLS if r.TLS is non-nil or the
// X-Forwarded-Proto header is "https".
func requireTLS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
			http.Error(w, `{"error":"TLS required","code":"tls_required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// verifyWebhookSignature verifies an HMAC-SHA256 webhook signature.
//
// The header format is: t=<unix_timestamp>;s=sha256;v=<hex_signature>
//
// The function checks that:
//  1. The timestamp is within 300 seconds of now.
//  2. The HMAC-SHA256 of "<timestamp>.<body>" matches the provided signature.
func verifyWebhookSignature(body []byte, header string, signingKey string) error {
	if header == "" {
		return fmt.Errorf("missing signature header")
	}

	parts := make(map[string]string)
	for _, segment := range strings.Split(header, ";") {
		kv := strings.SplitN(segment, "=", 2)
		if len(kv) == 2 {
			parts[kv[0]] = kv[1]
		}
	}

	tsStr, ok := parts["t"]
	if !ok {
		return fmt.Errorf("missing timestamp in signature")
	}
	scheme, ok := parts["s"]
	if !ok || scheme != "sha256" {
		return fmt.Errorf("unsupported or missing signature scheme")
	}
	sigHex, ok := parts["v"]
	if !ok {
		return fmt.Errorf("missing signature value")
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	age := math.Abs(float64(time.Now().Unix() - ts))
	if age > 300 {
		return fmt.Errorf("signature timestamp too old or too far in the future")
	}

	// Compute expected signature: HMAC-SHA256(key, "<ts>.<body>")
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(tsStr))
	mac.Write([]byte("."))
	mac.Write(body)
	expected := mac.Sum(nil)

	provided, err := hex.DecodeString(sigHex)
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}

	if !hmac.Equal(expected, provided) {
		return fmt.Errorf("signature mismatch")
	}

	return nil
}