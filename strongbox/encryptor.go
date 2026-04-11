// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package strongbox

// encryptor is the internal encryption interface used by the Store.
type encryptor interface {
	Encrypt(derivedKey []byte, plaintext []byte) (ciphertext []byte, err error)
	Decrypt(derivedKey []byte, ciphertext []byte) ([]byte, error)
}