// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package strongbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

const (
	formatVersion  = 0x01
	dekSize        = 32
	nonceSize      = 12
	gcmTagSize     = 16
	minCipherLen   = 1 + nonceSize + dekSize + gcmTagSize + nonceSize + gcmTagSize // 89
)

// envelopeEncryptor implements AES-256-GCM envelope encryption.
type envelopeEncryptor struct{}

// NewEnvelopeEncryptor returns an encryptor that uses AES-256-GCM envelope encryption.
func NewEnvelopeEncryptor() encryptor {
	return &envelopeEncryptor{}
}

// Encrypt generates a random DEK, encrypts it with derivedKey, then encrypts
// plaintext with the DEK. Returns the envelope as a single byte slice.
func (e *envelopeEncryptor) Encrypt(derivedKey []byte, plaintext []byte) ([]byte, error) {
	// 1. Generate random 32-byte DEK.
	dek := make([]byte, dekSize)
	if _, err := rand.Read(dek); err != nil {
		return nil, fmt.Errorf("generate DEK: %w", err)
	}

	// 2. Generate random 12-byte dek_nonce.
	dekNonce := make([]byte, nonceSize)
	if _, err := rand.Read(dekNonce); err != nil {
		return nil, fmt.Errorf("generate DEK nonce: %w", err)
	}

	// 3. encrypted_dek = AES-256-GCM(key=derivedKey, nonce=dek_nonce, plaintext=DEK)
	kekBlock, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("create KEK cipher: %w", err)
	}
	kekGCM, err := cipher.NewGCM(kekBlock)
	if err != nil {
		return nil, fmt.Errorf("create KEK GCM: %w", err)
	}
	encryptedDEK := kekGCM.Seal(nil, dekNonce, dek, nil)

	// 4. Generate random 12-byte value_nonce.
	valueNonce := make([]byte, nonceSize)
	if _, err := rand.Read(valueNonce); err != nil {
		return nil, fmt.Errorf("generate value nonce: %w", err)
	}

	// 5. encrypted_value = AES-256-GCM(key=DEK, nonce=value_nonce, plaintext=plaintext)
	dekBlock, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("create DEK cipher: %w", err)
	}
	dekGCM, err := cipher.NewGCM(dekBlock)
	if err != nil {
		return nil, fmt.Errorf("create DEK GCM: %w", err)
	}
	encryptedValue := dekGCM.Seal(nil, valueNonce, plaintext, nil)

	// 6. Zero DEK from memory.
	zero(dek)

	// 7. Assemble envelope: [version | dekNonce | encryptedDEK | valueNonce | encryptedValue]
	out := make([]byte, 0, 1+len(dekNonce)+len(encryptedDEK)+len(valueNonce)+len(encryptedValue))
	out = append(out, formatVersion)
	out = append(out, dekNonce...)
	out = append(out, encryptedDEK...)
	out = append(out, valueNonce...)
	out = append(out, encryptedValue...)

	return out, nil
}

// Decrypt parses the envelope, decrypts the DEK with derivedKey, then
// decrypts the value with the DEK.
func (e *envelopeEncryptor) Decrypt(derivedKey []byte, ciphertext []byte) ([]byte, error) {
	// 1. Check minimum length.
	if len(ciphertext) < minCipherLen {
		return nil, fmt.Errorf("%w: ciphertext too short", ErrDecryptFailed)
	}

	// 2. Parse format byte.
	if ciphertext[0] != formatVersion {
		return nil, fmt.Errorf("%w: unsupported format version 0x%02x", ErrDecryptFailed, ciphertext[0])
	}

	// 3. Extract components.
	dekNonce := ciphertext[1:13]
	encryptedDEK := ciphertext[13:61]
	valueNonce := ciphertext[61:73]
	encryptedValue := ciphertext[73:]

	// 4. Decrypt DEK.
	kekBlock, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("%w: create KEK cipher: %v", ErrDecryptFailed, err)
	}
	kekGCM, err := cipher.NewGCM(kekBlock)
	if err != nil {
		return nil, fmt.Errorf("%w: create KEK GCM: %v", ErrDecryptFailed, err)
	}
	dek, err := kekGCM.Open(nil, dekNonce, encryptedDEK, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: decrypt DEK: %v", ErrDecryptFailed, err)
	}

	// 5. Decrypt value.
	dekBlock, err := aes.NewCipher(dek)
	if err != nil {
		zero(dek)
		return nil, fmt.Errorf("%w: create DEK cipher: %v", ErrDecryptFailed, err)
	}
	dekGCM, err := cipher.NewGCM(dekBlock)
	if err != nil {
		zero(dek)
		return nil, fmt.Errorf("%w: create DEK GCM: %v", ErrDecryptFailed, err)
	}
	plaintext, err := dekGCM.Open(nil, valueNonce, encryptedValue, nil)
	if err != nil {
		zero(dek)
		return nil, fmt.Errorf("%w: decrypt value: %v", ErrDecryptFailed, err)
	}

	// 6. Zero DEK from memory.
	zero(dek)

	return plaintext, nil
}