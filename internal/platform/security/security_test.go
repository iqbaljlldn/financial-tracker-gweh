package security_test

import (
	"context"
	"crypto/rand"
	"io"
	"testing"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

func randomKey32(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		t.Fatalf("failed to generate random 32-byte key: %v", err)
	}
	return k
}

func TestLocalKMSAndEnvelopeEncryption(t *testing.T) {
	ctx := context.Background()
	masterKey := randomKey32(t)
	keyID := "test-master-key-1"

	kmsClient, err := security.NewLocalKMSClient(keyID, masterKey)
	if err != nil {
		t.Fatalf("failed to create local kms client: %v", err)
	}

	encryptor, err := security.NewEnvelopeEncryptor(kmsClient, keyID)
	if err != nil {
		t.Fatalf("failed to create envelope encryptor: %v", err)
	}

	plaintext := []byte("google_oauth_access_token_super_secret_value_12345")

	// 1. Encrypt
	ciphertext, err := encryptor.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if len(ciphertext) == 0 {
		t.Fatal("ciphertext is empty")
	}

	// 2. Decrypt
	decrypted, err := encryptor.Decrypt(ctx, ciphertext)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("expected decrypted plaintext %q, got %q", plaintext, decrypted)
	}

	// 3. Test corruption
	corrupted := append([]byte(nil), ciphertext...)
	corrupted[len(corrupted)-1] ^= 0xFF
	_, err = encryptor.Decrypt(ctx, corrupted)
	if err == nil {
		t.Fatal("expected error when decrypting corrupted ciphertext, got nil")
	}

	// 4. Test truncated payload
	_, err = encryptor.Decrypt(ctx, ciphertext[:10])
	if err == nil {
		t.Fatal("expected error when decrypting truncated payload, got nil")
	}
}

func TestEnvelopeEncryptionKeyNotFound(t *testing.T) {
	ctx := context.Background()
	masterKey := randomKey32(t)
	kmsClient, err := security.NewLocalKMSClient("key-1", masterKey)
	if err != nil {
		t.Fatalf("failed to create kms client: %v", err)
	}

	encryptor, err := security.NewEnvelopeEncryptor(kmsClient, "non-existent-key")
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	_, err = encryptor.Encrypt(ctx, []byte("some-secret"))
	if err == nil {
		t.Fatal("expected error with non-existent key, got nil")
	}
}

func TestRefreshTokens(t *testing.T) {
	plainToken, tokenHash, err := security.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken failed: %v", err)
	}
	if len(plainToken) == 0 || len(tokenHash) == 0 {
		t.Fatal("token or hash is empty")
	}

	// Validate valid token
	valid, err := security.ValidateRefreshToken(plainToken, tokenHash)
	if err != nil {
		t.Fatalf("ValidateRefreshToken failed: %v", err)
	}
	if !valid {
		t.Fatal("expected token to be valid")
	}

	// Validate tampered token
	tamperedToken := plainToken + "tampered"
	valid, err = security.ValidateRefreshToken(tamperedToken, tokenHash)
	if err != nil {
		t.Fatalf("ValidateRefreshToken unexpected error: %v", err)
	}
	if valid {
		t.Fatal("expected tampered token to fail validation")
	}
}

func TestOAuthCSRFStateManager(t *testing.T) {
	secretKey := randomKey32(t)
	mgr, err := security.NewOAuthStateManager(secretKey, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create csrf manager: %v", err)
	}

	redirectURL := "http://localhost:3000/callback"
	state, err := mgr.GenerateState(redirectURL)
	if err != nil {
		t.Fatalf("GenerateState failed: %v", err)
	}

	// Validate state
	gotURL, err := mgr.ValidateState(state)
	if err != nil {
		t.Fatalf("ValidateState failed: %v", err)
	}
	if gotURL != redirectURL {
		t.Fatalf("expected redirectURL %q, got %q", redirectURL, gotURL)
	}

	// Validate tampered state
	tamperedState := state + "tampered"
	_, err = mgr.ValidateState(tamperedState)
	if err == nil {
		t.Fatal("expected error on tampered state, got nil")
	}

	// Validate expired state using manager with negative TTL
	expiredMgr, err := security.NewOAuthStateManager(secretKey, -1*time.Second)
	if err != nil {
		t.Fatalf("failed to create expired csrf manager: %v", err)
	}
	expiredState, err := expiredMgr.GenerateState(redirectURL)
	if err != nil {
		t.Fatalf("GenerateState failed: %v", err)
	}
	_, err = mgr.ValidateState(expiredState)
	if err != security.ErrCSRFStateExpired {
		t.Fatalf("expected ErrCSRFStateExpired, got %v", err)
	}
}

func TestJWTSessionTokenManager(t *testing.T) {
	signingKey := randomKey32(t)
	mgr, err := security.NewSessionTokenManager(signingKey)
	if err != nil {
		t.Fatalf("failed to create session token manager: %v", err)
	}

	userID := "019488a0-2f1a-7b3b-81d3-34e89ac41101"
	email := "test@example.com"
	role := "user"

	tokenString, expiresAt, err := mgr.Issue(userID, email, role, 1*time.Hour)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}
	if tokenString == "" {
		t.Fatal("tokenString is empty")
	}
	if expiresAt.Before(time.Now()) {
		t.Fatal("expiresAt should be in the future")
	}

	// Validate valid token
	claims, err := mgr.Validate(tokenString)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if claims.UserID != userID {
		t.Fatalf("expected userID %s, got %s", userID, claims.UserID)
	}
	if claims.Email != email {
		t.Fatalf("expected email %s, got %s", email, claims.Email)
	}
	if claims.Role != role {
		t.Fatalf("expected role %s, got %s", role, claims.Role)
	}

	// Validate ops role
	opsToken, _, err := mgr.Issue(userID, email, "ops", 1*time.Hour)
	if err != nil {
		t.Fatalf("Issue ops token failed: %v", err)
	}
	opsClaims, err := mgr.Validate(opsToken)
	if err != nil {
		t.Fatalf("Validate ops token failed: %v", err)
	}
	if opsClaims.Role != "ops" {
		t.Fatalf("expected role ops, got %s", opsClaims.Role)
	}

	// Validate tampered token
	tamperedToken := tokenString + "tampered"
	_, err = mgr.Validate(tamperedToken)
	if err == nil {
		t.Fatal("expected error on tampered token, got nil")
	}

	// Validate expired token
	expiredToken, _, err := mgr.Issue(userID, email, role, -1*time.Second)
	if err != nil {
		t.Fatalf("Issue expired token failed: %v", err)
	}
	_, err = mgr.Validate(expiredToken)
	if err != security.ErrJWTExpired {
		t.Fatalf("expected ErrJWTExpired, got %v", err)
	}
}
