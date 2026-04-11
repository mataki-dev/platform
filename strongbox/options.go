// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package strongbox

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
)

// Option configures a Store during construction.
type Option func(*storeConfig)

type storeConfig struct {
	rootKey      []byte
	prevKey      []byte
	keyCount     int
	prevCount    int
	audit        AuditLogger
	maxValSize   int
	maxBatchSize int
}

// ---------------------------------------------------------------------------
// Required — exactly one key option
// ---------------------------------------------------------------------------

// WithKeyFromEnv reads a hex-encoded 32-byte root key from the named
// environment variable.
func WithKeyFromEnv(envVar string) Option {
	return func(c *storeConfig) {
		raw := os.Getenv(envVar)
		if raw == "" {
			// Mark that an attempt was made so validate catches it.
			c.keyCount++
			return
		}
		b, err := hex.DecodeString(raw)
		if err != nil {
			c.keyCount++
			return
		}
		c.rootKey = b
		c.keyCount++
	}
}

// WithKeyFromBytes sets the root key directly (intended for testing).
func WithKeyFromBytes(key []byte) Option {
	return func(c *storeConfig) {
		c.rootKey = make([]byte, len(key))
		copy(c.rootKey, key)
		c.keyCount++
	}
}

// ---------------------------------------------------------------------------
// Key rotation
// ---------------------------------------------------------------------------

// WithPreviousKeyFromEnv reads a hex-encoded 32-byte previous root key from
// the named environment variable (used during key rotation).
func WithPreviousKeyFromEnv(envVar string) Option {
	return func(c *storeConfig) {
		raw := os.Getenv(envVar)
		if raw == "" {
			c.prevCount++
			return
		}
		b, err := hex.DecodeString(raw)
		if err != nil {
			c.prevCount++
			return
		}
		c.prevKey = b
		c.prevCount++
	}
}

// WithPreviousKeyFromBytes sets the previous root key directly.
func WithPreviousKeyFromBytes(key []byte) Option {
	return func(c *storeConfig) {
		c.prevKey = make([]byte, len(key))
		copy(c.prevKey, key)
		c.prevCount++
	}
}

// ---------------------------------------------------------------------------
// Optional
// ---------------------------------------------------------------------------

// WithAuditLogger sets the audit logger for the Store.
func WithAuditLogger(logger AuditLogger) Option {
	return func(c *storeConfig) {
		c.audit = logger
	}
}

// WithMaxValueSize sets the maximum plaintext value size in bytes.
// Default: 65536.
func WithMaxValueSize(bytes int) Option {
	return func(c *storeConfig) {
		c.maxValSize = bytes
	}
}

// WithMaxBatchSize sets the maximum number of entries in a batch operation.
// Default: 500.
func WithMaxBatchSize(n int) Option {
	return func(c *storeConfig) {
		c.maxBatchSize = n
	}
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func (c *storeConfig) validate() error {
	if c.keyCount == 0 {
		return errors.New("strongbox: a root key option is required")
	}
	if c.keyCount > 1 {
		return errors.New("strongbox: exactly one root key option is allowed")
	}
	if len(c.rootKey) != 32 {
		return fmt.Errorf("strongbox: root key must be 32 bytes, got %d", len(c.rootKey))
	}
	if c.prevCount > 1 {
		return errors.New("strongbox: at most one previous key option is allowed")
	}
	if c.prevCount == 1 && len(c.prevKey) != 32 {
		return fmt.Errorf("strongbox: previous key must be 32 bytes, got %d", len(c.prevKey))
	}
	return nil
}