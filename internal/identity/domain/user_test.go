package domain_test

import (
	"testing"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
)

func TestUserAggregateCreation(t *testing.T) {
	u, err := domain.NewUser("usr-123", "User@Example.com", "Jane Doe", "Asia/Jakarta")
	if err != nil {
		t.Fatalf("unexpected error creating user: %v", err)
	}

	if u.ID() != "usr-123" {
		t.Fatalf("expected ID usr-123, got %s", u.ID())
	}
	if u.Email() != "user@example.com" { // Lowercased
		t.Fatalf("expected email user@example.com, got %s", u.Email())
	}
	if u.DisplayName() != "Jane Doe" {
		t.Fatalf("expected displayName Jane Doe, got %s", u.DisplayName())
	}
	if u.Timezone() != "Asia/Jakarta" {
		t.Fatalf("expected timezone Asia/Jakarta, got %s", u.Timezone())
	}
	if u.CreatedAt().IsZero() || u.UpdatedAt().IsZero() {
		t.Fatal("createdAt or updatedAt should not be zero")
	}

	// Default timezone when empty
	uDef, err := domain.NewUser("usr-124", "jane@example.com", "Jane", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uDef.Timezone() != "UTC" {
		t.Fatalf("expected default timezone UTC, got %s", uDef.Timezone())
	}

	// Invalid email
	_, err = domain.NewUser("usr-125", "invalid-email-no-at", "Jane", "UTC")
	if err != domain.ErrInvalidEmail {
		t.Fatalf("expected ErrInvalidEmail, got %v", err)
	}
}

func TestUserUpdateSettings(t *testing.T) {
	u, err := domain.NewUser("usr-123", "user@example.com", "Jane Doe", "UTC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dailyTime := "08:30:00"
	u.UpdateSettings("Jane Smith", "Asia/Jakarta", &dailyTime)

	if u.DisplayName() != "Jane Smith" {
		t.Fatalf("expected Jane Smith, got %s", u.DisplayName())
	}
	if u.Timezone() != "Asia/Jakarta" {
		t.Fatalf("expected Asia/Jakarta, got %s", u.Timezone())
	}
	if u.NotificationDailyTime() == nil || *u.NotificationDailyTime() != dailyTime {
		t.Fatalf("expected notificationDailyTime %s, got %v", dailyTime, u.NotificationDailyTime())
	}
}

func TestOAuthTokenEntity(t *testing.T) {
	expiresAt := time.Now().UTC().Add(3600 * time.Second)
	token, err := domain.NewOAuthToken(
		"tok-1",
		"gmail-acc-1",
		[]byte("encrypted-access"),
		[]byte("encrypted-refresh"),
		expiresAt,
		"https://www.googleapis.com/auth/gmail.readonly openid email profile",
	)
	if err != nil {
		t.Fatalf("unexpected error creating OAuthToken: %v", err)
	}

	if token.ID() != "tok-1" {
		t.Fatalf("expected ID tok-1, got %s", token.ID())
	}
	if token.GmailAccountID() != "gmail-acc-1" {
		t.Fatalf("expected gmailAccountID gmail-acc-1, got %s", token.GmailAccountID())
	}
	if token.IsExpired() {
		t.Fatal("token should not be expired")
	}

	// Test expired token (clock skew buffer is 60s)
	pastToken, err := domain.NewOAuthToken(
		"tok-2",
		"gmail-acc-2",
		[]byte("access"),
		[]byte("refresh"),
		time.Now().UTC().Add(30*time.Second), // within 60s skew
		"openid",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pastToken.IsExpired() {
		t.Fatal("token expiring in 30s should be considered expired due to 60s buffer")
	}

	// Test update tokens
	newExpires := time.Now().UTC().Add(7200 * time.Second)
	token.UpdateTokens([]byte("new-access"), []byte("new-refresh"), newExpires)
	if string(token.EncryptedAccessToken()) != "new-access" {
		t.Fatalf("expected new-access, got %s", string(token.EncryptedAccessToken()))
	}
	if string(token.EncryptedRefreshToken()) != "new-refresh" {
		t.Fatalf("expected new-refresh, got %s", string(token.EncryptedRefreshToken()))
	}
	if token.AccessTokenExpiresAt() != newExpires {
		t.Fatalf("expected updated expiration")
	}
}
