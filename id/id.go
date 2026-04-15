// Package id generates and validates prefixed base58-encoded IDs.
//
// IDs follow the format "{prefix}_{base58(16 random bytes)}", matching
// the Stripe/Clerk convention. Examples:
//
//	id.New("conn")  → "conn_7kVjR3mPqW2xNpTL8bR4Ym"
//	id.New("key")   → "key_9mTxK4vN2pR8wL5jQ3HnZd"
//
// IDs are opaque after generation — there is no Parse function.
// Use Validate to check format against an expected prefix.
package id

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

const (
	// randomBytes is the number of random bytes encoded in each ID.
	randomBytes = 16
)

// base58Alphabet is the set of valid base58 characters (Bitcoin alphabet).
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// New generates a prefixed ID: "{prefix}_{base58(16 random bytes)}".
// Panics if the system random source fails.
func New(prefix string) string {
	b := make([]byte, randomBytes)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("id: crypto/rand failed: %v", err))
	}
	return prefix + "_" + base58.Encode(b)
}

// Validate checks that raw matches the expected prefix, has the correct
// structure (prefix + underscore + base58 payload), and the payload uses
// only valid base58 characters with the expected length.
func Validate(raw, expectedPrefix string) error {
	if raw == "" {
		return fmt.Errorf("id: empty")
	}

	idx := strings.IndexByte(raw, '_')
	if idx < 0 {
		return fmt.Errorf("id: missing underscore separator")
	}

	prefix := raw[:idx]
	payload := raw[idx+1:]

	if prefix != expectedPrefix {
		return fmt.Errorf("id: prefix %q does not match expected %q", prefix, expectedPrefix)
	}

	if payload == "" {
		return fmt.Errorf("id: empty payload")
	}

	// Verify base58 alphabet
	for i, c := range payload {
		if !strings.ContainsRune(base58Alphabet, c) {
			return fmt.Errorf("id: invalid character %q at payload position %d", c, i)
		}
	}

	// Verify payload decodes to the expected length
	decoded, err := base58.Decode(payload)
	if err != nil {
		return fmt.Errorf("id: base58 decode failed: %w", err)
	}
	if len(decoded) != randomBytes {
		return fmt.Errorf("id: decoded length %d, expected %d", len(decoded), randomBytes)
	}

	return nil
}
