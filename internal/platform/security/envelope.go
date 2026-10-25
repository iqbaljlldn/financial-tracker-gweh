package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// EnvelopeVersion1 is the current envelope encryption payload version.
	EnvelopeVersion1 byte = 0x01
	// NonceSize standard for AES-GCM is 12 bytes.
	GCMNonceSize = 12
)

var (
	ErrEnvelopeMalformed  = errors.New("envelope: payload is too short or malformed")
	ErrUnsupportedVersion = errors.New("envelope: unsupported envelope format version")
	ErrDecryptionFailed   = errors.New("envelope: failed to decrypt payload")
)

// EnvelopeEncryptor provides application-layer envelope encryption for sensitive tokens
// (such as OAuth access/refresh tokens in identity.oauth_tokens) per 16_security.md §4.3.
type EnvelopeEncryptor struct {
	kmsClient KMSClient
	keyID     string
}

// NewEnvelopeEncryptor creates an EnvelopeEncryptor backed by the provided KMSClient and key identifier.
func NewEnvelopeEncryptor(kms KMSClient, keyID string) (*EnvelopeEncryptor, error) {
	if kms == nil {
		return nil, errors.New("envelope: kms client is required")
	}
	if keyID == "" {
		return nil, errors.New("envelope: keyID is required")
	}
	return &EnvelopeEncryptor{
		kmsClient: kms,
		keyID:     keyID,
	}, nil
}

// Encrypt performs envelope encryption on plaintext bytes.
// It generates a unique per-record 256-bit DEK wrapped by KMS, encrypts the plaintext with AES-256-GCM,
// and returns a serialized payload packing:
// [version: 1 byte][wrappedDEK length: 2 bytes uint16][wrappedDEK: N bytes][nonce: 12 bytes][ciphertext + auth tag]
func (e *EnvelopeEncryptor) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errors.New("envelope: cannot encrypt empty plaintext")
	}

	// 1. Generate DEK via KMS
	plaintextDEK, wrappedDEK, err := e.kmsClient.GenerateDataKey(ctx, e.keyID)
	if err != nil {
		return nil, fmt.Errorf("envelope: kms generate data key failed: %w", err)
	}
	defer SecureWipe(plaintextDEK)

	// 2. Initialize AES-256-GCM cipher with DEK
	block, err := aes.NewCipher(plaintextDEK)
	if err != nil {
		return nil, fmt.Errorf("envelope: failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("envelope: failed to create gcm: %w", err)
	}

	// 3. Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("envelope: failed to generate nonce: %w", err)
	}

	// 4. Encrypt plaintext
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// 5. Pack envelope payload
	wrappedLen := len(wrappedDEK)
	if wrappedLen > 65535 {
		return nil, errors.New("envelope: wrapped DEK exceeds maximum length")
	}

	totalLen := 1 + 2 + wrappedLen + len(nonce) + len(ciphertext)
	payload := make([]byte, totalLen)

	payload[0] = EnvelopeVersion1
	binary.BigEndian.PutUint16(payload[1:3], uint16(wrappedLen))
	copy(payload[3:3+wrappedLen], wrappedDEK)

	offset := 3 + wrappedLen
	copy(payload[offset:offset+len(nonce)], nonce)
	offset += len(nonce)
	copy(payload[offset:], ciphertext)

	return payload, nil
}

// Decrypt unpacks an envelope-encrypted payload, unwraps the per-record DEK via KMS,
// and decrypts the ciphertext using AES-256-GCM.
func (e *EnvelopeEncryptor) Decrypt(ctx context.Context, envelopeBytes []byte) ([]byte, error) {
	if len(envelopeBytes) < 1+2+1+GCMNonceSize+16 {
		return nil, ErrEnvelopeMalformed
	}

	version := envelopeBytes[0]
	if version != EnvelopeVersion1 {
		return nil, fmt.Errorf("%w: got 0x%x", ErrUnsupportedVersion, version)
	}

	wrappedLen := int(binary.BigEndian.Uint16(envelopeBytes[1:3]))
	if len(envelopeBytes) < 3+wrappedLen+GCMNonceSize+16 {
		return nil, ErrEnvelopeMalformed
	}

	wrappedDEK := envelopeBytes[3 : 3+wrappedLen]
	offset := 3 + wrappedLen
	nonce := envelopeBytes[offset : offset+GCMNonceSize]
	offset += GCMNonceSize
	ciphertext := envelopeBytes[offset:]

	// 1. Unwrap DEK via KMS
	plaintextDEK, err := e.kmsClient.Decrypt(ctx, e.keyID, wrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("envelope: kms unwrapping failed: %w", err)
	}
	defer SecureWipe(plaintextDEK)

	// 2. Initialize AES-GCM with unwrapped DEK
	block, err := aes.NewCipher(plaintextDEK)
	if err != nil {
		return nil, fmt.Errorf("envelope: failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("envelope: failed to create gcm: %w", err)
	}

	// 3. Decrypt ciphertext
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return plaintext, nil
}
