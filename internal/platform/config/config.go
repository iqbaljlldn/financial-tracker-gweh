// Package config provides layered configuration loading and validation utilities
// for platform services and bounded contexts per 06_architecture.md §9 and 21_project_structure.md §5.1.
//
// Precedence: Defaults -> Config file (YAML) -> Environment variables (highest precedence).
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	// ErrMissingConfigPath indicates an empty configuration file path.
	ErrMissingConfigPath = errors.New("config file path is required")
	// ErrInvalidConfig indicates a configuration validation failure.
	ErrInvalidConfig = errors.New("invalid configuration")
)

// Config represents the top-level layered configuration mirroring the 8-module architecture map.
type Config struct {
	Platform       PlatformConfig       `yaml:"platform"`
	Identity       IdentityConfig       `yaml:"identity"`
	Ingestion      IngestionConfig      `yaml:"ingestion"`
	Understanding  UnderstandingConfig  `yaml:"understanding"`
	FinancialCore  FinancialCoreConfig  `yaml:"financialcore"`
	Budget         BudgetConfig         `yaml:"budget"`
	Analytics      AnalyticsConfig      `yaml:"analytics"`
	Engagement     EngagementConfig     `yaml:"engagement"`
	Administration AdministrationConfig `yaml:"administration"`
}

// PlatformConfig holds cross-cutting infrastructure settings.
type PlatformConfig struct {
	Env           string        `yaml:"env"`
	HTTP          HTTPConfig    `yaml:"http"`
	Observability Observability `yaml:"observability"`
	TaskServer    TaskServer    `yaml:"taskserver"`
	DatabaseURL   string        `yaml:"-"`
	RedisURL      string        `yaml:"-"`
	JWTSecret     string        `yaml:"-"`
	KMSKeyID      string        `yaml:"-"`
}

type HTTPConfig struct {
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

type Observability struct {
	LogLevel      string `yaml:"log_level"`
	EnableTracing bool   `yaml:"enable_tracing"`
	MetricsPort   int    `yaml:"metrics_port"`
}

type TaskServer struct {
	Concurrency int `yaml:"concurrency"`
}

type IdentityConfig struct {
	TokenRefreshWindow time.Duration `yaml:"token_refresh_window"`
	SessionTTL         time.Duration `yaml:"session_ttl"`
	GoogleClientID     string        `yaml:"-"`
	GoogleClientSecret string        `yaml:"-"`
	GoogleRedirectURL  string        `yaml:"-"`
}

type IngestionConfig struct {
	PollFallbackInterval time.Duration `yaml:"poll_fallback_interval"`
	BatchSize            int           `yaml:"batch_size"`
	MaxRetries           int           `yaml:"max_retries"`
}

// UnderstandingConfig holds AI inference and deterministic parsing configuration.
type UnderstandingConfig struct {
	Provider              string        `yaml:"provider"` // "groq", "gemini", "openai", "openrouter"
	Model                 string        `yaml:"model"`
	ConfidenceThreshold   float64       `yaml:"confidence_threshold"`
	AIRateLimitPerSecond  int           `yaml:"ai_rate_limit_per_second"`
	AIRequestTimeout      time.Duration `yaml:"ai_request_timeout"`
	MaxAIFallbacksPerHour int           `yaml:"max_ai_fallbacks_per_hour"`

	// Resolved runtime credentials (populated from environment variables)
	ResolvedAPIKey  string `yaml:"-"`
	ResolvedBaseURL string `yaml:"-"`
}

type FinancialCoreConfig struct {
	BaseCurrency            string  `yaml:"base_currency"`
	ConfidenceGateThreshold float64 `yaml:"confidence_gate_threshold"`
	AutoFinalize            bool    `yaml:"auto_finalize"`
}

type BudgetConfig struct {
	AlertEvaluationInterval time.Duration `yaml:"alert_evaluation_interval"`
	DefaultWarningThreshold float64       `yaml:"default_warning_threshold"`
}

type AnalyticsConfig struct {
	ProjectionBatchSize         int           `yaml:"projection_batch_size"`
	ProjectionLagAlertThreshold time.Duration `yaml:"projection_lag_alert_threshold"`
}

type EngagementConfig struct {
	NotificationBatchSize int           `yaml:"notification_batch_size"`
	DeliveryTimeout       time.Duration `yaml:"delivery_timeout"`
	MaxDeliveryRetries    int           `yaml:"max_delivery_retries"`
	WhatsAppToken         string        `yaml:"-"`
	WhatsAppPhoneNumberID string        `yaml:"-"`
}

type AdministrationConfig struct {
	AuditLogRetentionDays int           `yaml:"audit_log_retention_days"`
	MetricsScrapeInterval time.Duration `yaml:"metrics_scrape_interval"`
}

// Load reads and parses a YAML config file and merges environment variables into it.
// In development, it also loads environment variables from a .env file if specified or present.
func Load(configPath string, envPaths ...string) (*Config, error) {
	if configPath == "" {
		return nil, ErrMissingConfigPath
	}

	// Try loading .env files if present (without overriding existing environment variables)
	for _, envPath := range envPaths {
		_ = loadDotEnv(envPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", configPath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config yaml: %w", err)
	}

	applyEnvironmentOverrides(&cfg)
	resolveAICredentials(&cfg)

	return &cfg, nil
}

// applyEnvironmentOverrides injects secrets and runtime environment overrides into the typed config.
func applyEnvironmentOverrides(cfg *Config) {
	if val := os.Getenv("DATABASE_URL"); val != "" {
		cfg.Platform.DatabaseURL = val
	}
	if val := os.Getenv("REDIS_URL"); val != "" {
		cfg.Platform.RedisURL = val
	}
	if val := os.Getenv("JWT_SECRET"); val != "" {
		cfg.Platform.JWTSecret = val
	}
	if val := os.Getenv("KMS_KEY_ID"); val != "" {
		cfg.Platform.KMSKeyID = val
	}

	if val := os.Getenv("GOOGLE_OAUTH_CLIENT_ID"); val != "" {
		cfg.Identity.GoogleClientID = val
	}
	if val := os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"); val != "" {
		cfg.Identity.GoogleClientSecret = val
	}
	if val := os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"); val != "" {
		cfg.Identity.GoogleRedirectURL = val
	}

	if val := os.Getenv("WHATSAPP_API_TOKEN"); val != "" {
		cfg.Engagement.WhatsAppToken = val
	}
	if val := os.Getenv("WHATSAPP_PHONE_NUMBER_ID"); val != "" {
		cfg.Engagement.WhatsAppPhoneNumberID = val
	}
}

// resolveAICredentials maps provider-specific environment variables to the active LLM client configuration.
func resolveAICredentials(cfg *Config) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Understanding.Provider))

	groqKey := os.Getenv("GROQ_PROVIDER_API_KEY")
	groqURL := os.Getenv("GROQ_PROVIDER_BASE_URL")
	geminiKey := os.Getenv("GEMINI_PROVIDER_API_KEY")
	geminiURL := os.Getenv("GEMINI_PROVIDER_BASE_URL")
	genericKey := os.Getenv("AI_PROVIDER_API_KEY")
	genericURL := os.Getenv("AI_PROVIDER_BASE_URL")

	// If no provider explicitly set in YAML, auto-detect based on available keys
	if provider == "" {
		if groqKey != "" {
			provider = "groq"
		} else if geminiKey != "" {
			provider = "gemini"
		} else {
			provider = "openai"
		}
		cfg.Understanding.Provider = provider
	}

	switch provider {
	case "groq":
		if groqKey != "" {
			cfg.Understanding.ResolvedAPIKey = groqKey
		} else {
			cfg.Understanding.ResolvedAPIKey = genericKey
		}
		if groqURL != "" {
			cfg.Understanding.ResolvedBaseURL = groqURL
		} else {
			cfg.Understanding.ResolvedBaseURL = "https://api.groq.com/openai/v1"
		}
		if cfg.Understanding.Model == "" {
			cfg.Understanding.Model = "qwen/qwen3.8-27b"
		}

	case "gemini":
		if geminiKey != "" {
			cfg.Understanding.ResolvedAPIKey = geminiKey
		} else {
			cfg.Understanding.ResolvedAPIKey = genericKey
		}
		if geminiURL != "" {
			cfg.Understanding.ResolvedBaseURL = geminiURL
		} else {
			cfg.Understanding.ResolvedBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai/"
		}
		if cfg.Understanding.Model == "" {
			cfg.Understanding.Model = "gemini-flash-latest"
		}

	default: // "openai" or custom
		cfg.Understanding.ResolvedAPIKey = genericKey
		if genericURL != "" {
			cfg.Understanding.ResolvedBaseURL = genericURL
		} else {
			cfg.Understanding.ResolvedBaseURL = "https://api.openai.com/v1"
		}
	}

	// Normalize Google Gemini base URL: ensure it points to the OpenAI-compatible endpoint
	if strings.Contains(cfg.Understanding.ResolvedBaseURL, "generativelanguage.googleapis.com") {
		if strings.Contains(cfg.Understanding.ResolvedBaseURL, ":generateContent") {
			cfg.Understanding.ResolvedBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai/"
		}
	}
}

// loadDotEnv parses key-value pairs from a .env file and sets them if not already defined in os.Environ.
func loadDotEnv(filename string) error {
	cleanPath := filepath.Clean(filename)
	file, err := os.Open(cleanPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)

		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
	return scanner.Err()
}
