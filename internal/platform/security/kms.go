package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"sync"
)

var (
	ErrKMSKeyNotFound     = errors.New("kms: key id not found")
	ErrKMSInvalidPayload  = errors.New("kms: invalid ciphertext payload")
	ErrKMSDecryptionError = errors.New("kms: failed to decrypt data key")
	ErrInvalidDEKLength   = errors.New("kms: invalid data encryption key length")
)

// KMSClient defines the contract for Key Management Service operations.
// Per 16_security.md §4.3, master root keys never leave the KMS boundary.
type KMSClient interface {
	// GenerateDataKey generates a random 256-bit DEK and returns its plaintext
	// and wrapped (ciphertext) form under the specified keyID.
	GenerateDataKey(ctx context.Context, keyID string) (plaintextDEK []byte, ciphertextDEK []byte, err error)

	// Decrypt unwraps a wrapped DEK using the master root key identified by keyID.
	Decrypt(ctx context.Context, keyID string, ciphertextDEK []byte) (plaintextDEK []byte, err error)
}

// LocalKMSClient provides an in-process, AES-256-GCM based KMS implementation
// suitable for local development, testing, and single-host deployments.
type LocalKMSClient struct {
	mu   sync.RWMutex
	keys map[string][]byte
}

// NewLocalKMSClient creates a new LocalKMSClient initialized with a primary key.
// The provided primaryKey must be 32 bytes (256-bit AES key).
func NewLocalKMSClient(keyID string, primaryKey []byte) (*LocalKMSClient, error) {
	if len(primaryKey) != 32 {
		return nil, errors.New("kms: primary key must be exactly 32 bytes for AES-256")
	}
	client := &LocalKMSClient{
		keys: make(map[string][]byte),
	}
	client.keys[keyID] = append([]byte(nil), primaryKey...)
	return client, nil
}

// RegisterKey adds or rotates a master key under the given keyID.
func (c *LocalKMSClient) RegisterKey(keyID string, key []byte) error {
	if len(key) != 32 {
		return errors.New("kms: key must be exactly 32 bytes for AES-256")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys[keyID] = append([]byte(nil), key...)
	return nil
}

// GenerateDataKey generates a cryptographically secure 256-bit DEK,
// encrypts it using the master key identified by keyID, and returns both forms.
func (c *LocalKMSClient) GenerateDataKey(ctx context.Context, keyID string) ([]byte, []byte, error) {
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	c.mu.RLock()
	masterKey, exists := c.keys[keyID]
	c.mu.RUnlock()

	if !exists {
		return nil, nil, fmt.Errorf("%w: %s", ErrKMSKeyNotFound, keyID)
	}

	// 1. Generate 32-byte plaintext DEK
	plaintextDEK := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, plaintextDEK); err != nil {
		return nil, nil, fmt.Errorf("kms: failed to generate random DEK: %w", err)
	}

	// 2. Wrap DEK using AES-256-GCM with master key
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, nil, fmt.Errorf("kms: cipher initialization failed: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("kms: gcm initialization failed: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("kms: failed to generate nonce: %w", err)
	}

	// ciphertextDEK format: [nonce][sealed DEK + auth tag]
	ciphertextDEK := gcm.Seal(nonce, nonce, plaintextDEK, []byte(keyID))

	return plaintextDEK, ciphertextDEK, nil
}

// Decrypt unwraps the ciphertextDEK using the master key identified by keyID.
func (c *LocalKMSClient) Decrypt(ctx context.Context, keyID string, ciphertextDEK []byte) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	c.mu.RLock()
	masterKey, exists := c.keys[keyID]
	c.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrKMSKeyNotFound, keyID)
	}

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("kms: cipher initialization failed: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("kms: gcm initialization failed: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertextDEK) < nonceSize+gcm.Overhead() {
		return nil, ErrKMSInvalidPayload
	}

	nonce := ciphertextDEK[:nonceSize]
	ciphertext := ciphertextDEK[nonceSize:]

	plaintextDEK, err := gcm.Open(nil, nonce, ciphertext, []byte(keyID))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKMSDecryptionError, err)
	}

	if len(plaintextDEK) != 32 {
		// Zeroize corrupted DEK before returning error
		for i := range plaintextDEK {
			plaintextDEK[i] = 0
		}
		return nil, ErrInvalidDEKLength
	}

	return plaintextDEK, nil
}

// SecureWipe zeroizes sensitive byte slices from memory.
func SecureWipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// ConstantTimeEquals compares two byte slices in constant time.
func ConstantTimeEquals(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
