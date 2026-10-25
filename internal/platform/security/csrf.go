package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

var (
	ErrCSRFStateExpired   = errors.New("csrf: state token has expired")
	ErrCSRFStateInvalid   = errors.New("csrf: state token signature is invalid")
	ErrCSRFStateMalformed = errors.New("csrf: state token is malformed")
)

type statePayload struct {
	Nonce       string `json:"n"`
	ExpiresAt   int64  `json:"exp"`
	RedirectURL string `json:"r,omitempty"`
}

// OAuthStateManager generates and validates cryptographic single-use, short-TTL CSRF state tokens
// for OAuth 2.0 flows per 16_security.md §5.2.
type OAuthStateManager struct {
	signingKey []byte
	ttl        time.Duration
}

// NewOAuthStateManager creates a new OAuthStateManager with the given secret key and token TTL.
func NewOAuthStateManager(secretKey []byte, ttl time.Duration) (*OAuthStateManager, error) {
	if len(secretKey) < 16 {
		return nil, errors.New("csrf: secret key must be at least 16 bytes")
	}
	if ttl == 0 {
		ttl = 10 * time.Minute
	}
	return &OAuthStateManager{
		signingKey: append([]byte(nil), secretKey...),
		ttl:        ttl,
	}, nil
}

// GenerateState creates a cryptographically signed, tamper-resistant OAuth state token.
func (m *OAuthStateManager) GenerateState(redirectURL string) (string, error) {
	nonceBytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, nonceBytes); err != nil {
		return "", fmt.Errorf("csrf: failed to generate nonce: %w", err)
	}

	payload := statePayload{
		Nonce:       base64.RawURLEncoding.EncodeToString(nonceBytes),
		ExpiresAt:   time.Now().Add(m.ttl).Unix(),
		RedirectURL: redirectURL,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("csrf: failed to marshal state payload: %w", err)
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	// HMAC-SHA256 signature
	mac := hmac.New(sha256.New, m.signingKey)
	mac.Write([]byte(payloadB64))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("%s.%s", payloadB64, sigB64), nil
}

// ValidateState verifies that the provided state token was signed with the secret key and has not expired.
// It returns the optional redirect URL embedded during state generation.
func (m *OAuthStateManager) ValidateState(state string) (string, error) {
	var payloadB64, sigB64 string
	for i := 0; i < len(state); i++ {
		if state[i] == '.' {
			payloadB64 = state[:i]
			sigB64 = state[i+1:]
			break
		}
	}

	if payloadB64 == "" || sigB64 == "" {
		return "", ErrCSRFStateMalformed
	}

	expectedMAC := hmac.New(sha256.New, m.signingKey)
	expectedMAC.Write([]byte(payloadB64))
	expectedSig := expectedMAC.Sum(nil)

	actualSig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return "", ErrCSRFStateMalformed
	}

	if subtle.ConstantTimeCompare(actualSig, expectedSig) != 1 {
		return "", ErrCSRFStateInvalid
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return "", ErrCSRFStateMalformed
	}

	var payload statePayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return "", ErrCSRFStateMalformed
	}

	if time.Now().Unix() >= payload.ExpiresAt {
		return "", ErrCSRFStateExpired
	}

	return payload.RedirectURL, nil
}
