package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const (
	// RefreshTokenEntropyBytes specifies the raw random entropy (256 bits).
	RefreshTokenEntropyBytes = 32
)

// GenerateRefreshToken creates a high-entropy cryptographically secure opaque token
// and its SHA-256 hash for database persistence per 16_security.md §5.1.
// The plain token is returned to the client once; only the hash is stored in DB.
func GenerateRefreshToken() (string, string, error) {
	bytes := make([]byte, RefreshTokenEntropyBytes)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", "", fmt.Errorf("security: failed to generate refresh token: %w", err)
	}

	plainToken := base64.RawURLEncoding.EncodeToString(bytes)
	tokenHash := HashRefreshToken(plainToken)

	return plainToken, tokenHash, nil
}

// HashRefreshToken calculates the SHA-256 hex digest of an opaque refresh token.
func HashRefreshToken(plainToken string) string {
	sum := sha256.Sum256([]byte(plainToken))
	return hex.EncodeToString(sum[:])
}

// ValidateRefreshToken performs constant-time verification of a plain refresh token against a stored hash.
func ValidateRefreshToken(plainToken string, storedHash string) (bool, error) {
	if plainToken == "" || storedHash == "" {
		return false, errors.New("security: token and stored hash must not be empty")
	}

	computedHash := HashRefreshToken(plainToken)
	match := subtle.ConstantTimeCompare([]byte(computedHash), []byte(storedHash)) == 1
	return match, nil
}
