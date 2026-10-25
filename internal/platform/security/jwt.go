package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrJWTExpired   = errors.New("jwt: token has expired")
	ErrJWTInvalid   = errors.New("jwt: token signature is invalid")
	ErrJWTMalformed = errors.New("jwt: token structure is malformed")
)

const (
	DefaultIssuer     = "financial-tracker-platform"
	DefaultAudience   = "financial-tracker-api"
	DefaultSessionTTL = 1 * time.Hour
)

// SessionClaims represents the payload claims inside a platform session JWT per 16_security.md §5.1.
type SessionClaims struct {
	UserID    string `json:"sub"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// SessionTokenManager issues and validates platform session JWTs.
type SessionTokenManager struct {
	signingKey []byte
	issuer     string
	audience   string
}

// NewSessionTokenManager initializes a SessionTokenManager with the signing key.
func NewSessionTokenManager(signingKey []byte) (*SessionTokenManager, error) {
	if len(signingKey) < 32 {
		return nil, errors.New("jwt: signing key must be at least 32 bytes for HS256")
	}
	return &SessionTokenManager{
		signingKey: append([]byte(nil), signingKey...),
		issuer:     DefaultIssuer,
		audience:   DefaultAudience,
	}, nil
}

// Issue creates a short-lived signed JWT session token for the authenticated user.
func (m *SessionTokenManager) Issue(userID, email, role string, ttl time.Duration) (string, time.Time, error) {
	if userID == "" {
		return "", time.Time{}, errors.New("jwt: user ID cannot be empty")
	}
	if role == "" {
		role = "user"
	}
	if ttl == 0 {
		ttl = DefaultSessionTTL
	}

	now := time.Now()
	expiresAt := now.Add(ttl)

	header := jwtHeader{
		Alg: "HS256",
		Typ: "JWT",
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("jwt: failed to marshal header: %w", err)
	}
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)

	claims := SessionClaims{
		UserID:    userID,
		Email:     email,
		Role:      role,
		Issuer:    m.issuer,
		Audience:  m.audience,
		IssuedAt:  now.Unix(),
		ExpiresAt: expiresAt.Unix(),
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("jwt: failed to marshal claims: %w", err)
	}
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	unsignedToken := fmt.Sprintf("%s.%s", headerB64, claimsB64)

	mac := hmac.New(sha256.New, m.signingKey)
	mac.Write([]byte(unsignedToken))
	signature := mac.Sum(nil)
	sigB64 := base64.RawURLEncoding.EncodeToString(signature)

	tokenString := fmt.Sprintf("%s.%s", unsignedToken, sigB64)
	return tokenString, expiresAt, nil
}

// Validate parses and verifies a session token, returning the validated claims.
func (m *SessionTokenManager) Validate(tokenString string) (*SessionClaims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, ErrJWTMalformed
	}

	headerB64, claimsB64, sigB64 := parts[0], parts[1], parts[2]

	// 1. Verify signature in constant time
	unsignedToken := fmt.Sprintf("%s.%s", headerB64, claimsB64)
	mac := hmac.New(sha256.New, m.signingKey)
	mac.Write([]byte(unsignedToken))
	expectedSig := mac.Sum(nil)

	actualSig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, ErrJWTMalformed
	}

	if subtle.ConstantTimeCompare(actualSig, expectedSig) != 1 {
		return nil, ErrJWTInvalid
	}

	// 2. Decode claims
	claimsJSON, err := base64.RawURLEncoding.DecodeString(claimsB64)
	if err != nil {
		return nil, ErrJWTMalformed
	}

	var claims SessionClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, ErrJWTMalformed
	}

	// 3. Check expiration
	if time.Now().Unix() >= claims.ExpiresAt {
		return nil, ErrJWTExpired
	}

	// 4. Check audience & issuer
	if claims.Issuer != m.issuer || claims.Audience != m.audience {
		return nil, ErrJWTInvalid
	}

	return &claims, nil
}
