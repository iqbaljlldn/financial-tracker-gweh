package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultTimeout = 15 * time.Second
	tracerName     = "financial-tracker/understanding/llmclient"
)

// LLMClientConfig configures the OpenAI-compatible HTTP client.
type LLMClientConfig struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
	Timeout      time.Duration
	HTTPClient   *http.Client
	Logger       *slog.Logger
}

// OpenAICompatibleLLMClient implements domain.LLMExtractor targeting any OpenAI-compatible API (11_ai_blueprint.md §8).
type OpenAICompatibleLLMClient struct {
	baseURL      string
	apiKey       string
	defaultModel string
	client       *http.Client
	logger       *slog.Logger
	tracer       trace.Tracer
}

// NewOpenAICompatibleLLMClient creates a new provider-agnostic LLM client.
func NewOpenAICompatibleLLMClient(cfg LLMClientConfig) *OpenAICompatibleLLMClient {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &OpenAICompatibleLLMClient{
		baseURL:      baseURL,
		apiKey:       cfg.APIKey,
		defaultModel: cfg.DefaultModel,
		client:       httpClient,
		logger:       logger,
		tracer:       otel.Tracer(tracerName),
	}
}

type openAIRequest struct {
	Model          string              `json:"model"`
	Messages       []domain.LLMMessage `json:"messages"`
	Temperature    float64             `json:"temperature,omitempty"`
	ResponseFormat *openAIFormat       `json:"response_format,omitempty"`
	MaxTokens      int                 `json:"max_tokens,omitempty"`
}

type openAIFormat struct {
	Type string `json:"type"`
}

type openAIResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

// Extract sends a prompt to the OpenAI-compatible endpoint and returns structured response (11_ai_blueprint.md §8).
func (c *OpenAICompatibleLLMClient) Extract(
	ctx context.Context,
	req domain.ExtractionPromptRequest,
) (*domain.StructuredLLMResponse, error) {
	model := req.ModelIdentifier
	if model == "" {
		model = c.defaultModel
	}
	if model == "" {
		return nil, domain.ErrLLMModelNotConfigured
	}

	// Trace span per 17_observability.md §5.3 [GATE: external_system=llm_provider, NO PII IN ATTRIBUTES]
	ctx, span := c.tracer.Start(ctx, "LLMProvider.Extract",
		trace.WithAttributes(
			attribute.String("external_system", "llm_provider"),
			attribute.String("llm.model", model),
			attribute.String("llm.base_url", c.baseURL),
		),
	)
	defer span.End()

	payload := openAIRequest{
		Model:       model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	if req.ResponseFormat == "json_object" {
		payload.ResponseFormat = &openAIFormat{Type: "json_object"}
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("marshaling LLM request: %w", err)
	}

	endpoint := c.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	startTime := time.Now()
	resp, err := c.client.Do(httpReq)
	latency := time.Since(startTime)
	latencyMs := latency.Milliseconds()

	span.SetAttributes(attribute.Int64("llm.latency_ms", latencyMs))

	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("%w: %v", domain.ErrLLMRequestFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("reading LLM response: %w", err)
	}

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, domain.ErrLLMRateLimitExceeded
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d: %s", domain.ErrLLMRequestFailed, resp.StatusCode, string(respBytes))
	}

	var res openAIResponse
	if err := json.Unmarshal(respBytes, &res); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("parsing LLM response: %w", err)
	}

	if res.Error != nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrLLMRequestFailed, res.Error.Message)
	}

	if len(res.Choices) == 0 {
		return nil, fmt.Errorf("%w: no completion choices returned", domain.ErrLLMRequestFailed)
	}

	content := res.Choices[0].Message.Content

	tokenUsage := domain.LLMTokenUsage{
		PromptTokens:     res.Usage.PromptTokens,
		CompletionTokens: res.Usage.CompletionTokens,
		TotalTokens:      res.Usage.TotalTokens,
	}

	span.SetAttributes(
		attribute.Int("llm.prompt_tokens", tokenUsage.PromptTokens),
		attribute.Int("llm.completion_tokens", tokenUsage.CompletionTokens),
		attribute.Int("llm.total_tokens", tokenUsage.TotalTokens),
	)

	return &domain.StructuredLLMResponse{
		RawJSON:         content,
		ModelIdentifier: model,
		LatencyMs:       latencyMs,
		TokenUsage:      tokenUsage,
	}, nil
}
