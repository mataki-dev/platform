// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package strongbox

import (
	"crypto/sha256"
	"encoding/hex"
	"io"

	"golang.org/x/crypto/hkdf"
)

// DeriveKey derives a per-client-tenant key from root key using HKDF-SHA256.
// Salt = clientID, info = tenantID. Returns 32 bytes.
func DeriveKey(rootKey []byte, clientID ClientID, tenantID TenantID) ([]byte, error) {
	r := hkdf.New(sha256.New, rootKey, []byte(clientID), []byte(tenantID))
	key := make([]byte, 32)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}
	return key, nil
}

// RootKeyID returns SHA-256 of root key truncated to 16 hex chars.
func RootKeyID(rootKey []byte) string {
	h := sha256.Sum256(rootKey)
	return hex.EncodeToString(h[:])[:16]
}

// zero overwrites a byte slice with zeroes.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}