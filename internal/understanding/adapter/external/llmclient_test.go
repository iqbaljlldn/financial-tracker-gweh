package external_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/adapter/external"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

func TestOpenAICompatibleLLMClient_Extract_Success(t *testing.T) {
	expectedJSON := `{"eventType":"Expense","amount":{"value":"45.00","currency":"USD","confidence":0.95}}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody["model"] != "test-model-v1" {
			t.Errorf("expected model 'test-model-v1', got %v", reqBody["model"])
		}

		resp := map[string]any{
			"id":    "chatcmpl-123",
			"model": "test-model-v1",
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": expectedJSON,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     150,
				"completion_tokens": 50,
				"total_tokens":      200,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := external.NewOpenAICompatibleLLMClient(external.LLMClientConfig{
		BaseURL:      ts.URL,
		APIKey:       "test-api-key",
		DefaultModel: "test-model-v1",
		Timeout:      2 * time.Second,
	})

	res, err := client.Extract(context.Background(), domain.ExtractionPromptRequest{
		Messages: []domain.LLMMessage{
			{Role: "system", Content: "Return JSON"},
			{Role: "user", Content: "Receipt: $45 at Amazon"},
		},
		ResponseFormat: "json_object",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RawJSON != expectedJSON {
		t.Errorf("expected RawJSON %s, got %s", expectedJSON, res.RawJSON)
	}
	if res.TokenUsage.TotalTokens != 200 {
		t.Errorf("expected 200 total tokens, got %d", res.TokenUsage.TotalTokens)
	}
	if res.ModelIdentifier != "test-model-v1" {
		t.Errorf("expected model 'test-model-v1', got %s", res.ModelIdentifier)
	}
}

func TestOpenAICompatibleLLMClient_RateLimit_429(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"Rate limit reached"}}`))
	}))
	defer ts.Close()

	client := external.NewOpenAICompatibleLLMClient(external.LLMClientConfig{
		BaseURL:      ts.URL,
		APIKey:       "test-key",
		DefaultModel: "test-model",
	})

	_, err := client.Extract(context.Background(), domain.ExtractionPromptRequest{
		Messages: []domain.LLMMessage{{Role: "user", Content: "Hello"}},
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != domain.ErrLLMRateLimitExceeded {
		t.Errorf("expected ErrLLMRateLimitExceeded, got %v", err)
	}
}

func TestOpenAICompatibleLLMClient_ProviderSwapZeroCodeChanges(t *testing.T) {
	// Demonstrates that swapping baseURL, apiKey, and model requires zero code changes [GATE]
	groqServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"provider":"groq"}`}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer groqServer.Close()

	geminiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"provider":"gemini"}`}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer geminiServer.Close()

	// Swapping config only:
	clientGroq := external.NewOpenAICompatibleLLMClient(external.LLMClientConfig{
		BaseURL:      groqServer.URL,
		APIKey:       "groq-key",
		DefaultModel: "qwen/qwen3.8-27b",
	})
	resGroq, err := clientGroq.Extract(context.Background(), domain.ExtractionPromptRequest{
		Messages: []domain.LLMMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil || resGroq.RawJSON != `{"provider":"groq"}` {
		t.Fatalf("groq swap failed: %v, raw: %s", err, resGroq.RawJSON)
	}

	clientGemini := external.NewOpenAICompatibleLLMClient(external.LLMClientConfig{
		BaseURL:      geminiServer.URL,
		APIKey:       "gemini-key",
		DefaultModel: "gemini-flash-latest",
	})
	resGemini, err := clientGemini.Extract(context.Background(), domain.ExtractionPromptRequest{
		Messages: []domain.LLMMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil || resGemini.RawJSON != `{"provider":"gemini"}` {
		t.Fatalf("gemini swap failed: %v, raw: %s", err, resGemini.RawJSON)
	}
}
