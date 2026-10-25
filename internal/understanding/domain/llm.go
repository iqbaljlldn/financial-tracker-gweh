package domain

import (
	"context"
	"errors"
)

var (
	ErrLLMRequestFailed      = errors.New("llm provider: request failed")
	ErrLLMRateLimitExceeded  = errors.New("llm provider: rate limit exceeded")
	ErrLLMBudgetExceeded     = errors.New("llm provider: daily budget exceeded")
	ErrLLMSchemaInvalid      = errors.New("llm provider: structured response schema invalid")
	ErrLLMModelNotConfigured = errors.New("llm provider: model identifier not configured")
)

// LLMMessage represents a role/content message in the prompt.
type LLMMessage struct {
	Role    string `json:"role"` // "system" | "user" | "assistant"
	Content string `json:"content"`
}

// ExtractionPromptRequest encapsulates prompt messages and options for structured LLM extraction.
type ExtractionPromptRequest struct {
	Messages        []LLMMessage
	ModelIdentifier string
	Temperature     float64
	ResponseFormat  string // "json_object"
	MaxTokens       int
}

// LLMTokenUsage captures token consumption for cost governance (11_ai_blueprint.md §10 & §11).
type LLMTokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StructuredLLMResponse encapsulates the raw response content and metadata returned by the provider.
type StructuredLLMResponse struct {
	RawJSON         string
	ModelIdentifier string
	LatencyMs       int64
	TokenUsage      LLMTokenUsage
}

// LLMExtractor defines the narrow, provider-agnostic domain interface for LLM extraction (11_ai_blueprint.md §8) [GATE].
// Application and domain layers must depend strictly on this abstraction, never on a third-party SDK.
type LLMExtractor interface {
	Extract(ctx context.Context, req ExtractionPromptRequest) (*StructuredLLMResponse, error)
}
