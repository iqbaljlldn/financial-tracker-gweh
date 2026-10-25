package command

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/categorization"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/governance"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/prompt"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/sanitizer"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// Outcomes for AI extraction
const (
	OutcomeSuccess         = "success"
	OutcomeRepromptSuccess = "reprompt_success"
	OutcomeSchemaInvalid   = "schema_invalid"
	OutcomeBudgetExceeded  = "budget_exceeded"
	OutcomeFailed          = "failed"
)

// Schema validation outcomes for prompt_metrics (11_ai_blueprint.md §10)
const (
	ValidationValidFirstTry      = "valid_first_try"
	ValidationValidAfterReprompt = "valid_after_reprompt"
	ValidationInvalid            = "invalid"
)

// InvokeAIExtractorCommand encapsulates the request to perform AI-based extraction (11_ai_blueprint.md / Diagram 4).
type InvokeAIExtractorCommand struct {
	MessageID      string                 `json:"messageId"`
	GmailAccountID string                 `json:"gmailAccountId"`
	UserID         string                 `json:"userId"`
	RawEmail       domain.RawEmailMessage `json:"rawEmail"`
	TriggerReason  string                 `json:"triggerReason"` // "unknown_sender" | "template_failed"
}

// InvokeAIExtractorResult models the outcome of an AI extraction call.
type InvokeAIExtractorResult struct {
	Outcome             string                          `json:"outcome"`
	ExtractionResultID  string                          `json:"extractionResultId"`
	ConfidenceAggregate float64                         `json:"confidenceAggregate"`
	Candidate           *domain.CandidateFinancialEvent `json:"candidate,omitempty"`
	ErrorMessage        string                          `json:"errorMessage,omitempty"`
}

// InvokeAIExtractorHandler coordinates prompt construction, LLM extraction, structured validation,
// deterministic confidence computation, and categorization (Diagram 4) [GATE].
type InvokeAIExtractorHandler struct {
	llmExtractor   domain.LLMExtractor
	promptRegistry *prompt.Registry
	categorizer    *categorization.Engine
	governor       *governance.DailyBudgetGovernor
	extractionRepo domain.ExtractionResultRepository
	promptMetrics  domain.PromptMetricsRepository
	outboxRepo     domain.OutboxRepository
	weights        domain.ConfidenceWeights
	logger         *slog.Logger
}

// NewInvokeAIExtractorHandler creates a new InvokeAIExtractorHandler.
func NewInvokeAIExtractorHandler(
	llmExtractor domain.LLMExtractor,
	promptRegistry *prompt.Registry,
	categorizer *categorization.Engine,
	governor *governance.DailyBudgetGovernor,
	extractionRepo domain.ExtractionResultRepository,
	promptMetrics domain.PromptMetricsRepository,
	outboxRepo domain.OutboxRepository,
	weights domain.ConfidenceWeights,
	logger *slog.Logger,
) *InvokeAIExtractorHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &InvokeAIExtractorHandler{
		llmExtractor:   llmExtractor,
		promptRegistry: promptRegistry,
		categorizer:    categorizer,
		governor:       governor,
		extractionRepo: extractionRepo,
		promptMetrics:  promptMetrics,
		outboxRepo:     outboxRepo,
		weights:        weights,
		logger:         logger,
	}
}

type validationResult struct {
	rawOutput         *prompt.RawAIExtractionOutput
	validationOutcome string
	overallOutcome    string
	llmResp           *domain.StructuredLLMResponse
	failed            bool
	errResult         *InvokeAIExtractorResult
}

// Handle executes the full AI extraction pipeline per 11_ai_blueprint.md & Diagram 4.
func (h *InvokeAIExtractorHandler) Handle(
	ctx context.Context,
	cmd InvokeAIExtractorCommand,
) (*InvokeAIExtractorResult, error) {
	resultID := uuid.New().String()

	// 1. Cost & Rate-Limit Governance: Check daily budget allowance (11_ai_blueprint.md §11)
	if h.governor != nil {
		if err := h.governor.AcquireCall(ctx, cmd.UserID); err != nil {
			if errors.Is(err, domain.ErrLLMBudgetExceeded) {
				h.logger.Warn("daily AI budget exceeded, routing directly to manual review queue",
					slog.String("userId", cmd.UserID),
					slog.String("messageId", cmd.MessageID),
					slog.String("error", err.Error()),
				)
				return h.handleBudgetExceeded(ctx, cmd, resultID, err.Error())
			}
		}
	}

	// 2. PII Minimization Layer & Prompt Construction (11_ai_blueprint.md §4, §9)
	activePrompt, messages := h.preparePrompt(cmd)

	// 3. Invoke LLM Provider via provider-agnostic interface (11_ai_blueprint.md §8) [GATE]
	req := domain.ExtractionPromptRequest{
		Messages:       messages,
		ResponseFormat: "json_object",
		Temperature:    0.1, // low temperature for deterministic structured extraction
	}
	llmResp, err := h.llmExtractor.Extract(ctx, req)
	if err != nil {
		h.logger.Error("LLM extraction failed",
			slog.String("messageId", cmd.MessageID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("llm extraction failed: %w", err)
	}

	// 4. Structured Output Validation & Re-Prompt Policy (11_ai_blueprint.md §5.1) [GATE]
	valRes, err := h.validateAndReprompt(ctx, messages, cmd, resultID, activePrompt, llmResp)
	if err != nil || (valRes != nil && valRes.failed) {
		if valRes != nil && valRes.errResult != nil {
			return valRes.errResult, nil
		}
		return nil, err
	}

	// 5. Confidence, Categorization & Candidate Event Assembly (11_ai_blueprint.md §6.2, §7, 04_domain_model.md §7.2)
	candidate, aggregateConf, perFieldConf, categoryAssign, err := h.buildCandidate(
		ctx, cmd, valRes.rawOutput, activePrompt.VersionID, valRes.llmResp.ModelIdentifier,
	)
	if err != nil {
		return nil, err
	}

	// 6. Persist Results, Record Metrics, Emit Outbox Event (11_ai_blueprint.md §10, Diagram 4)
	h.recordSuccess(ctx, cmd, resultID, activePrompt.VersionID, valRes, aggregateConf, perFieldConf, candidate)

	h.logger.Info("AI extraction completed successfully",
		slog.String("messageId", cmd.MessageID),
		slog.String("outcome", valRes.overallOutcome),
		slog.Float64("confidence", aggregateConf),
		slog.String("category", categoryAssign.Category),
		slog.String("categorySource", categoryAssign.Source),
	)

	return &InvokeAIExtractorResult{
		Outcome:             valRes.overallOutcome,
		ExtractionResultID:  resultID,
		ConfidenceAggregate: aggregateConf,
		Candidate:           candidate,
	}, nil
}

func (h *InvokeAIExtractorHandler) preparePrompt(cmd InvokeAIExtractorCommand) (*prompt.PromptVersion, []domain.LLMMessage) {
	content := cmd.RawEmail.BodyText
	if strings.TrimSpace(content) == "" {
		content = cmd.RawEmail.BodyHTML
	}
	sanitizedEmail := sanitizer.SanitizeEmail(fmt.Sprintf("Subject: %s\n\n%s", cmd.RawEmail.Subject, content))

	activePrompt := h.promptRegistry.GetActive()
	if activePrompt == nil {
		activePrompt = prompt.GetV1PromptVersion()
	}
	messages := activePrompt.BuildExtractionPrompt(sanitizedEmail)
	return activePrompt, messages
}

func (h *InvokeAIExtractorHandler) validateAndReprompt(
	ctx context.Context,
	messages []domain.LLMMessage,
	cmd InvokeAIExtractorCommand,
	resultID string,
	activePrompt *prompt.PromptVersion,
	initialResp *domain.StructuredLLMResponse,
) (*validationResult, error) {
	rawOutput, valErr := prompt.ValidateRawOutput(initialResp.RawJSON)
	if valErr == nil {
		return &validationResult{
			rawOutput:         rawOutput,
			validationOutcome: ValidationValidFirstTry,
			overallOutcome:    OutcomeSuccess,
			llmResp:           initialResp,
		}, nil
	}

	h.logger.Warn("raw LLM response failed schema validation, attempting 1 automatic re-prompt",
		slog.String("messageId", cmd.MessageID),
		slog.String("error", valErr.Error()),
	)

	// [GATE]: Exactly ONE automatic re-prompt on validation failure
	repromptMsg := prompt.BuildRePromptMessage(valErr.Error())
	retryMessages := append(messages,
		domain.LLMMessage{Role: "assistant", Content: initialResp.RawJSON},
		repromptMsg,
	)

	retryResp, retryErr := h.llmExtractor.Extract(ctx, domain.ExtractionPromptRequest{
		Messages:       retryMessages,
		ResponseFormat: "json_object",
		Temperature:    0.0,
	})

	if retryErr == nil {
		rawOutput, valErr = prompt.ValidateRawOutput(retryResp.RawJSON)
		initialResp.LatencyMs += retryResp.LatencyMs
		initialResp.TokenUsage.PromptTokens += retryResp.TokenUsage.PromptTokens
		initialResp.TokenUsage.CompletionTokens += retryResp.TokenUsage.CompletionTokens
		initialResp.TokenUsage.TotalTokens += retryResp.TokenUsage.TotalTokens
	}

	if valErr != nil {
		// Second failure -> route to manual review queue, NOT retry/dead-letter (11_ai_blueprint.md §5.1) [GATE]
		h.logger.Error("LLM response failed schema validation after re-prompt, routing to manual review",
			slog.String("messageId", cmd.MessageID),
			slog.String("error", valErr.Error()),
		)
		res, err := h.handleSchemaInvalid(ctx, cmd, resultID, activePrompt.VersionID, initialResp, valErr.Error())
		return &validationResult{failed: true, errResult: res}, err
	}

	return &validationResult{
		rawOutput:         rawOutput,
		validationOutcome: ValidationValidAfterReprompt,
		overallOutcome:    OutcomeRepromptSuccess,
		llmResp:           initialResp,
	}, nil
}

func (h *InvokeAIExtractorHandler) buildCandidate(
	ctx context.Context,
	cmd InvokeAIExtractorCommand,
	rawOutput *prompt.RawAIExtractionOutput,
	promptVersion, modelID string,
) (*domain.CandidateFinancialEvent, float64, map[string]float64, categorization.Assignment, error) {
	eventConf := 0.90
	fc := domain.FieldConfidences{
		Amount:        &rawOutput.Amount.Confidence,
		Merchant:      &rawOutput.Merchant.Confidence,
		EventType:     &eventConf,
		OccurredAt:    &rawOutput.OccurredAt.Confidence,
		PaymentMethod: &rawOutput.PaymentMethod.Confidence,
		CategoryHint:  &rawOutput.CategoryHint.Confidence,
	}
	aggregateConf, perFieldConf := domain.ComputeAggregateConfidence(fc, h.weights)

	categoryAssign := h.categorizer.AssignCategory(ctx, categorization.Request{
		UserID:             cmd.UserID,
		NormalizedMerchant: rawOutput.Merchant.NormalizedName,
		CategoryHint:       rawOutput.CategoryHint.Value,
		CategoryConfidence: rawOutput.CategoryHint.Confidence,
	})

	provenance := domain.Provenance{
		ConnectorType:      "gmail",
		ConnectorAccountID: cmd.GmailAccountID,
		SourceMessageRef:   cmd.MessageID,
		ExtractionMethod:   domain.ExtractionMethodAI,
		PromptVersion:      promptVersion,
		ModelIdentifier:    modelID,
		ExtractedAt:        time.Now().UTC(),
	}

	candidate, err := prompt.ConvertToDomainCandidate(rawOutput, cmd.UserID, provenance, aggregateConf, perFieldConf)
	if err != nil {
		return nil, 0, nil, categoryAssign, fmt.Errorf("converting to domain candidate: %w", err)
	}

	if candidate.OccurredAt.IsZero() {
		candidate.OccurredAt = cmd.RawEmail.ReceivedAt
		if candidate.OccurredAt.IsZero() {
			candidate.OccurredAt = time.Now().UTC()
		}
	}

	if candidate.TypeSpecificPayload == nil {
		candidate.TypeSpecificPayload = make(map[string]any)
	}
	candidate.TypeSpecificPayload["category"] = categoryAssign.Category
	candidate.TypeSpecificPayload["categorySource"] = categoryAssign.Source
	candidate.TypeSpecificPayload["categoryConfidence"] = categoryAssign.Confidence
	if rawOutput.ExtractionNotes != "" {
		candidate.TypeSpecificPayload["extractionNotes"] = rawOutput.ExtractionNotes
	}

	return candidate, aggregateConf, perFieldConf, categoryAssign, nil
}

func (h *InvokeAIExtractorHandler) recordSuccess(
	ctx context.Context,
	cmd InvokeAIExtractorCommand,
	resultID, promptVersion string,
	valRes *validationResult,
	aggregateConf float64,
	perFieldConf map[string]float64,
	candidate *domain.CandidateFinancialEvent,
) {
	promptVerStr := promptVersion
	modelIDStr := valRes.llmResp.ModelIdentifier
	_ = h.extractionRepo.SaveResult(ctx, &domain.ExtractionResultRecord{
		ID:                  resultID,
		IngestedMessageID:   cmd.MessageID,
		Method:              domain.ExtractionMethodAI,
		PromptVersion:       &promptVerStr,
		ModelIdentifier:     &modelIDStr,
		CandidatePayload:    *candidate,
		ConfidenceAggregate: aggregateConf,
		ConfidencePerField:  perFieldConf,
		CreatedAt:           time.Now().UTC(),
	})

	if h.promptMetrics != nil {
		_ = h.promptMetrics.RecordPromptMetric(ctx, &domain.PromptMetricRecord{
			ID:                      uuid.New().String(),
			PromptVersion:           promptVersion,
			ModelIdentifier:         valRes.llmResp.ModelIdentifier,
			AggregateConfidence:     aggregateConf,
			SchemaValidationOutcome: valRes.validationOutcome,
			LaterOverridden:         false,
			LatencyMs:               int(valRes.llmResp.LatencyMs),
			TokenUsage:              valRes.llmResp.TokenUsage.TotalTokens,
			RecordedAt:              time.Now().UTC(),
		})
	}

	_ = h.outboxRepo.RecordOutboxEvent(
		ctx,
		"candidate_event",
		resultID,
		domain.EventFinancialEventCandidateCreated,
		domain.FinancialEventCandidateCreatedPayload{
			ExtractionResultID: resultID,
			IngestedMessageID:  cmd.MessageID,
			Candidate:          *candidate,
			OccurredAt:         time.Now().UTC(),
		},
	)

	metrics := observability.GetMetrics()
	if metrics != nil {
		metrics.UnderstandingAIExtractionTotal.WithLabelValues(valRes.overallOutcome).Inc()
		metrics.UnderstandingAIConfidenceDistribution.WithLabelValues("ai").Observe(aggregateConf)
		metrics.UnderstandingAITokenUsageTotal.WithLabelValues("input").Add(float64(valRes.llmResp.TokenUsage.PromptTokens))
		metrics.UnderstandingAITokenUsageTotal.WithLabelValues("output").Add(float64(valRes.llmResp.TokenUsage.CompletionTokens))
	}
}

func (h *InvokeAIExtractorHandler) handleSchemaInvalid(
	ctx context.Context,
	cmd InvokeAIExtractorCommand,
	resultID, promptVersion string,
	llmResp *domain.StructuredLLMResponse,
	errDetail string,
) (*InvokeAIExtractorResult, error) {
	// Record schema_invalid in prompt_metrics
	if h.promptMetrics != nil {
		_ = h.promptMetrics.RecordPromptMetric(ctx, &domain.PromptMetricRecord{
			ID:                      uuid.New().String(),
			PromptVersion:           promptVersion,
			ModelIdentifier:         llmResp.ModelIdentifier,
			AggregateConfidence:     0.0,
			SchemaValidationOutcome: ValidationInvalid,
			LaterOverridden:         false,
			LatencyMs:               int(llmResp.LatencyMs),
			TokenUsage:              llmResp.TokenUsage.TotalTokens,
			RecordedAt:              time.Now().UTC(),
		})
	}

	// Create low-confidence candidate (confidence 0.0) routing to manual review queue (Diagram 4)
	candidate := &domain.CandidateFinancialEvent{
		UserID:    cmd.UserID,
		EventType: domain.EventTypeOthers,
		Amount: domain.Money{
			Value:    0.01,
			Currency: "USD",
		},
		OccurredAt: time.Now().UTC(),
		Confidence: domain.Confidence{
			Aggregate: 0.0,
		},
		Provenance: domain.Provenance{
			ConnectorType:      "gmail",
			ConnectorAccountID: cmd.GmailAccountID,
			SourceMessageRef:   cmd.MessageID,
			ExtractionMethod:   domain.ExtractionMethodAI,
			PromptVersion:      promptVersion,
			ModelIdentifier:    llmResp.ModelIdentifier,
			ExtractedAt:        time.Now().UTC(),
		},
		TypeSpecificPayload: map[string]any{
			"reviewReason": "schema_invalid",
			"errorDetail":  errDetail,
		},
	}

	_ = h.extractionRepo.SaveResult(ctx, &domain.ExtractionResultRecord{
		ID:                  resultID,
		IngestedMessageID:   cmd.MessageID,
		Method:              domain.ExtractionMethodAI,
		PromptVersion:       &promptVersion,
		ModelIdentifier:     &llmResp.ModelIdentifier,
		CandidatePayload:    *candidate,
		ConfidenceAggregate: 0.0,
		CreatedAt:           time.Now().UTC(),
	})

	_ = h.outboxRepo.RecordOutboxEvent(
		ctx,
		"candidate_event",
		resultID,
		domain.EventFinancialEventCandidateCreated,
		domain.FinancialEventCandidateCreatedPayload{
			ExtractionResultID: resultID,
			IngestedMessageID:  cmd.MessageID,
			Candidate:          *candidate,
			OccurredAt:         time.Now().UTC(),
		},
	)

	metrics := observability.GetMetrics()
	if metrics != nil {
		metrics.UnderstandingAIExtractionTotal.WithLabelValues(OutcomeSchemaInvalid).Inc()
	}

	return &InvokeAIExtractorResult{
		Outcome:             OutcomeSchemaInvalid,
		ExtractionResultID:  resultID,
		ConfidenceAggregate: 0.0,
		Candidate:           candidate,
		ErrorMessage:        errDetail,
	}, nil
}

func (h *InvokeAIExtractorHandler) handleBudgetExceeded(
	ctx context.Context,
	cmd InvokeAIExtractorCommand,
	resultID, errDetail string,
) (*InvokeAIExtractorResult, error) {
	// Create low-confidence candidate for manual review with reason budget_exceeded
	candidate := &domain.CandidateFinancialEvent{
		UserID:    cmd.UserID,
		EventType: domain.EventTypeOthers,
		Amount: domain.Money{
			Value:    0.01,
			Currency: "USD",
		},
		OccurredAt: time.Now().UTC(),
		Confidence: domain.Confidence{
			Aggregate: 0.0,
		},
		Provenance: domain.Provenance{
			ConnectorType:      "gmail",
			ConnectorAccountID: cmd.GmailAccountID,
			SourceMessageRef:   cmd.MessageID,
			ExtractionMethod:   domain.ExtractionMethodAI,
			ExtractedAt:        time.Now().UTC(),
		},
		TypeSpecificPayload: map[string]any{
			"reviewReason": "budget_exceeded",
			"errorDetail":  errDetail,
		},
	}

	_ = h.extractionRepo.SaveResult(ctx, &domain.ExtractionResultRecord{
		ID:                  resultID,
		IngestedMessageID:   cmd.MessageID,
		Method:              domain.ExtractionMethodAI,
		CandidatePayload:    *candidate,
		ConfidenceAggregate: 0.0,
		CreatedAt:           time.Now().UTC(),
	})

	_ = h.outboxRepo.RecordOutboxEvent(
		ctx,
		"candidate_event",
		resultID,
		domain.EventFinancialEventCandidateCreated,
		domain.FinancialEventCandidateCreatedPayload{
			ExtractionResultID: resultID,
			IngestedMessageID:  cmd.MessageID,
			Candidate:          *candidate,
			OccurredAt:         time.Now().UTC(),
		},
	)

	return &InvokeAIExtractorResult{
		Outcome:             OutcomeBudgetExceeded,
		ExtractionResultID:  resultID,
		ConfidenceAggregate: 0.0,
		Candidate:           candidate,
		ErrorMessage:        errDetail,
	}, nil
}
