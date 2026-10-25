package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/config"
)

func TestLoad_YAMLAndEnvironmentResolution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	yamlContent := `
platform:
  env: "development"
  http:
    port: 8080
understanding:
  provider: "groq"
  model: "qwen/qwen3.8-27b"
  confidence_threshold: 0.85
`
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config yaml: %v", err)
	}

	envContent := `
DATABASE_URL=postgres://test:test@localhost:5432/testdb
GROQ_PROVIDER_API_KEY=gsk_test12345
GROQ_PROVIDER_BASE_URL=https://api.groq.com/openai/v1
`
	envPath := filepath.Join(tempDir, "secrets.env")
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to write env: %v", err)
	}

	cfg, err := config.Load(configPath, envPath)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.Platform.Env != "development" {
		t.Errorf("expected env development, got %s", cfg.Platform.Env)
	}
	if cfg.Platform.DatabaseURL != "postgres://test:test@localhost:5432/testdb" {
		t.Errorf("expected database URL from env, got %s", cfg.Platform.DatabaseURL)
	}
	if cfg.Understanding.Provider != "groq" {
		t.Errorf("expected provider groq, got %s", cfg.Understanding.Provider)
	}
	if cfg.Understanding.ResolvedAPIKey != "gsk_test12345" {
		t.Errorf("expected resolved groq api key, got %s", cfg.Understanding.ResolvedAPIKey)
	}
	if cfg.Understanding.ResolvedBaseURL != "https://api.groq.com/openai/v1" {
		t.Errorf("expected resolved groq base url, got %s", cfg.Understanding.ResolvedBaseURL)
	}
}

func TestResolveAICredentials_GeminiNormalization(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gemini-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	yamlContent := `
understanding:
  provider: "gemini"
`
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config yaml: %v", err)
	}

	// Deliberately provide raw generateContent URL
	envContent := `
GEMINI_PROVIDER_API_KEY=AIzaSy_test
GEMINI_PROVIDER_BASE_URL=https://generativelanguage.googleapis.com/v1beta/models/gemini-flash-latest:generateContent
`
	envPath := filepath.Join(tempDir, "secrets.env")
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to write env: %v", err)
	}

	cfg, err := config.Load(configPath, envPath)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	// Should be automatically normalized to Google's OpenAI-compatible endpoint
	expectedURL := "https://generativelanguage.googleapis.com/v1beta/openai/"
	if cfg.Understanding.ResolvedBaseURL != expectedURL {
		t.Errorf("expected normalized OpenAI URL %q, got %q", expectedURL, cfg.Understanding.ResolvedBaseURL)
	}
	if cfg.Understanding.ResolvedAPIKey != "AIzaSy_test" {
		t.Errorf("expected gemini api key, got %q", cfg.Understanding.ResolvedAPIKey)
	}
}
