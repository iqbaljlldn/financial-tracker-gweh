package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity"
	identityhttp "github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/adapter/http"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = io.ReadFull(rand.Reader, b)
	return b
}

func setupMockGoogleOAuthServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			code := r.Form.Get("code")
			grantType := r.Form.Get("grant_type")
			refreshToken := r.Form.Get("refresh_token")

			if grantType == "refresh_token" {
				if refreshToken == "revoked_token" {
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"error": "invalid_grant",
					})
					return
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token": "mock_refreshed_access_token_999",
					"expires_in":   3600,
					"token_type":   "Bearer",
					"scope":        "https://www.googleapis.com/auth/gmail.readonly openid email profile",
				})
				return
			}

			if code == "valid_auth_code" {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token":  "mock_google_access_token_secret_xyz123",
					"refresh_token": "mock_google_refresh_token_secret_abc789",
					"expires_in":    3600,
					"token_type":    "Bearer",
					"scope":         "https://www.googleapis.com/auth/gmail.readonly openid email profile",
					"id_token":      "mock_id_token",
				})
				return
			}

			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":             "invalid_grant",
				"error_description": "Invalid authorization code",
			})

		case "/userinfo":
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":            "google-user-12345",
				"email":          "testuser@gmail.com",
				"email_verified": true,
				"name":           "Jane Test",
				"picture":        "https://example.com/avatar.jpg",
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestIdentityModule_EndToEndOAuthAndEnvelopeEncryption(t *testing.T) {
	dbURL := getDatabaseURL()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("skipping integration test: database not available at %s: %v", dbURL, err)
		return
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skipping integration test: failed to ping database: %v", err)
		return
	}

	// 1. Stand up mock Google OAuth server
	mockGoogle := setupMockGoogleOAuthServer()
	defer mockGoogle.Close()

	// 2. Initialize KMS and Module
	kmsKeyID := "integration-test-kms-key"
	kmsClient, err := security.NewLocalKMSClient(kmsKeyID, randomBytes(32))
	if err != nil {
		t.Fatalf("failed to create kms client: %v", err)
	}

	jwtSecret := randomBytes(32)
	csrfSecret := randomBytes(32)

	mod, err := identity.NewModule(identity.Config{
		DB:                 pool,
		KMSClient:          kmsClient,
		KMSKeyID:           kmsKeyID,
		JWTSecret:          jwtSecret,
		CSRFSecret:         csrfSecret,
		GoogleClientID:     "mock-client-id",
		GoogleClientSecret: "mock-client-secret",
		GoogleRedirectURI:  "http://localhost:8082/api/v1/auth/google/callback",
		GoogleAuthURL:      mockGoogle.URL + "/auth",
		GoogleTokenURL:     mockGoogle.URL + "/token",
		GoogleUserInfoURL:  mockGoogle.URL + "/userinfo",
	})
	if err != nil {
		t.Fatalf("failed to initialize identity module: %v", err)
	}

	router := chi.NewRouter()
	mod.RegisterHTTPRoutes(router)

	cleaner := NewTestCleaner(t, pool)
	cleaner.TrackEmail("testuser@gmail.com")
	cleaner.CleanupNow()

	// --- Step 1: POST /api/v1/auth/google/initiate ---
	initReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/google/initiate", nil)
	initRec := httptest.NewRecorder()
	router.ServeHTTP(initRec, initReq)

	if initRec.Code != http.StatusOK {
		t.Fatalf("initiate returned status %d, body: %s", initRec.Code, initRec.Body.String())
	}

	var initResp identityhttp.InitiateAuthResponse
	if err := json.Unmarshal(initRec.Body.Bytes(), &initResp); err != nil {
		t.Fatalf("failed to parse initiate response: %v", err)
	}

	if initResp.State == "" || initResp.AuthorizationURL == "" {
		t.Fatal("initiate response missing state or authorizationUrl")
	}

	// GATE VERIFICATION: Check Google OAuth parameter correctness per 12_gmail_connector.md §4.2 & 16_security.md §5.1
	parsedAuthURL, err := url.Parse(initResp.AuthorizationURL)
	if err != nil {
		t.Fatalf("failed to parse auth url: %v", err)
	}
	params := parsedAuthURL.Query()
	if params.Get("access_type") != "offline" {
		t.Fatalf("GATE VIOLATION: expected access_type=offline, got %s", params.Get("access_type"))
	}
	if params.Get("prompt") != "consent" {
		t.Fatalf("GATE VIOLATION: expected prompt=consent, got %s", params.Get("prompt"))
	}
	if params.Get("include_granted_scopes") != "true" {
		t.Fatalf("GATE VIOLATION: expected include_granted_scopes=true, got %s", params.Get("include_granted_scopes"))
	}
	if params.Get("code_challenge_method") != "S256" {
		t.Fatalf("GATE VIOLATION: expected code_challenge_method=S256, got %s", params.Get("code_challenge_method"))
	}

	scope := params.Get("scope")
	if !strings.Contains(scope, "https://www.googleapis.com/auth/gmail.readonly") {
		t.Fatalf("GATE VIOLATION: missing gmail.readonly scope in %s", scope)
	}
	if strings.Contains(scope, "modify") || strings.Contains(scope, "send") || strings.Contains(scope, "compose") {
		t.Fatalf("GATE VIOLATION: write scopes detected in OAuth request: %s", scope)
	}

	// --- Step 2: GET /api/v1/auth/google/callback ---
	callbackURL := "/api/v1/auth/google/callback?code=valid_auth_code&state=" + url.QueryEscape(initResp.State)
	cbReq := httptest.NewRequest(http.MethodGet, callbackURL, nil)
	cbRec := httptest.NewRecorder()
	router.ServeHTTP(cbRec, cbReq)

	if cbRec.Code != http.StatusOK {
		t.Fatalf("callback returned status %d, body: %s", cbRec.Code, cbRec.Body.String())
	}

	var cbResp identityhttp.CallbackAuthResponse
	if err := json.Unmarshal(cbRec.Body.Bytes(), &cbResp); err != nil {
		t.Fatalf("failed to parse callback response: %v", err)
	}

	if cbResp.SessionToken == "" || cbResp.RefreshToken == "" {
		t.Fatal("missing sessionToken or refreshToken in callback response")
	}
	if cbResp.User.Email != "testuser@gmail.com" {
		t.Fatalf("expected user email testuser@gmail.com, got %s", cbResp.User.Email)
	}

	// --- Step 3: VERIFY TOKENS ARE ENCRYPTED AT REST (DIRECT DB INSPECTION) [GATE] ---
	targetAccountID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("testuser@gmail.com")).String()
	var encAccess, encRefresh []byte
	err = pool.QueryRow(ctx, `
		SELECT encrypted_access_token, encrypted_refresh_token 
		FROM identity.oauth_tokens 
		WHERE gmail_account_id = $1
		ORDER BY updated_at DESC 
		LIMIT 1
	`, targetAccountID).Scan(&encAccess, &encRefresh)
	if err != nil {
		t.Fatalf("failed to query identity.oauth_tokens directly: %v", err)
	}

	// Envelope format check: byte 0 is EnvelopeVersion1 (0x01)
	if len(encAccess) < 4 || encAccess[0] != 0x01 {
		t.Fatalf("GATE VIOLATION: encrypted_access_token does not match envelope format")
	}
	if len(encRefresh) < 4 || encRefresh[0] != 0x01 {
		t.Fatalf("GATE VIOLATION: encrypted_refresh_token does not match envelope format")
	}

	// Ciphertext verification: plaintext MUST NOT appear anywhere in raw DB bytes
	rawPlainAccess := "mock_google_access_token_secret_xyz123"
	rawPlainRefresh := "mock_google_refresh_token_secret_abc789"
	if bytes.Contains(encAccess, []byte(rawPlainAccess)) {
		t.Fatalf("CRITICAL SECURITY FAILURE: raw plaintext access token found in database!")
	}
	if bytes.Contains(encRefresh, []byte(rawPlainRefresh)) {
		t.Fatalf("CRITICAL SECURITY FAILURE: raw plaintext refresh token found in database!")
	}

	// Decrypt with EnvelopeEncryptor and verify correctness
	decryptedAccess, err := mod.EnvelopeEncryptor.Decrypt(ctx, encAccess)
	if err != nil {
		t.Fatalf("failed to decrypt envelope ciphertext from DB: %v", err)
	}
	if string(decryptedAccess) != rawPlainAccess {
		t.Fatalf("decrypted access token mismatch: got %s, want %s", decryptedAccess, rawPlainAccess)
	}

	decryptedRefresh, err := mod.EnvelopeEncryptor.Decrypt(ctx, encRefresh)
	if err != nil {
		t.Fatalf("failed to decrypt envelope ciphertext from DB: %v", err)
	}
	if string(decryptedRefresh) != rawPlainRefresh {
		t.Fatalf("decrypted refresh token mismatch: got %s, want %s", decryptedRefresh, rawPlainRefresh)
	}

	// --- Step 4: GET /api/v1/me (Authenticated) ---
	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+cbResp.SessionToken)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)

	if meRec.Code != http.StatusOK {
		t.Fatalf("GET /me returned %d, body: %s", meRec.Code, meRec.Body.String())
	}
	var meUser identityhttp.UserDTO
	if err := json.Unmarshal(meRec.Body.Bytes(), &meUser); err != nil {
		t.Fatalf("failed to parse /me response: %v", err)
	}
	if meUser.Email != "testuser@gmail.com" {
		t.Fatalf("expected /me email testuser@gmail.com, got %s", meUser.Email)
	}

	// --- Step 5: PATCH /api/v1/me/settings ---
	newDisplayName := "Updated Jane"
	newTimezone := "Asia/Jakarta"
	patchBody, _ := json.Marshal(identityhttp.UpdateSettingsRequest{
		DisplayName: &newDisplayName,
		Timezone:    &newTimezone,
	})
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/me/settings", bytes.NewReader(patchBody))
	patchReq.Header.Set("Authorization", "Bearer "+cbResp.SessionToken)
	patchRec := httptest.NewRecorder()
	router.ServeHTTP(patchRec, patchReq)

	if patchRec.Code != http.StatusOK {
		t.Fatalf("PATCH /me/settings returned %d, body: %s", patchRec.Code, patchRec.Body.String())
	}
	var updatedUser identityhttp.UserDTO
	if err := json.Unmarshal(patchRec.Body.Bytes(), &updatedUser); err != nil {
		t.Fatalf("failed to parse updated user: %v", err)
	}
	if updatedUser.DisplayName != newDisplayName || updatedUser.Timezone != newTimezone {
		t.Fatalf("user settings not updated properly: %+v", updatedUser)
	}

	// --- Step 6: POST /api/v1/auth/refresh ---
	refreshBody, _ := json.Marshal(identityhttp.RefreshSessionRequest{
		RefreshToken: cbResp.RefreshToken,
	})
	refReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(refreshBody))
	refRec := httptest.NewRecorder()
	router.ServeHTTP(refRec, refReq)

	if refRec.Code != http.StatusOK {
		t.Fatalf("POST /auth/refresh returned %d, body: %s", refRec.Code, refRec.Body.String())
	}
	var refResp identityhttp.RefreshSessionResponse
	if err := json.Unmarshal(refRec.Body.Bytes(), &refResp); err != nil {
		t.Fatalf("failed to parse refresh response: %v", err)
	}
	if refResp.SessionToken == "" {
		t.Fatal("expected new sessionToken from refresh")
	}

	// --- Step 7: H1 CONFLICT TEST (Diagram 1) ---
	// Invalid or duplicate callback attempts with bad code must return 400 OAUTH_CALLBACK_INVALID
	badCbReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=bad_code&state=invalid_state", nil)
	badCbRec := httptest.NewRecorder()
	router.ServeHTTP(badCbRec, badCbReq)

	if badCbRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad state, got %d", badCbRec.Code)
	}
	var errResp identityhttp.ErrorEnvelope
	if err := json.Unmarshal(badCbRec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if errResp.Error.Code != "OAUTH_CALLBACK_INVALID" {
		t.Fatalf("expected error code OAUTH_CALLBACK_INVALID, got %s", errResp.Error.Code)
	}
}
