package command

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/executor"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// ApplySenderTemplateCommand payload to extract financial event from an email.
type ApplySenderTemplateCommand struct {
	MessageID string                 `json:"messageId"`
	UserID    string                 `json:"userId"`
	RawEmail  domain.RawEmailMessage `json:"rawEmail"`
}

// ApplySenderTemplateResult represents extraction output.
type ApplySenderTemplateResult struct {
	Outcome             string                          `json:"outcome"`
	ConfidenceAggregate float64                         `json:"confidenceAggregate"`
	Candidate           *domain.CandidateFinancialEvent `json:"candidate,omitempty"`
	ExtractionResultID  string                          `json:"extractionResultId"`
	MissingFields       []string                        `json:"missingFields,omitempty"`
}

// ApplySenderTemplateHandler handles deterministic template extraction against raw email messages.
type ApplySenderTemplateHandler struct {
	matcher        *executor.TemplateMatcher
	executor       *executor.ExtractionExecutor
	extractionRepo domain.ExtractionResultRepository
	outboxRepo     domain.OutboxRepository
	aiExtractorCmd *InvokeAIExtractorHandler
	asynqClient    *asynq.Client
	logger         *slog.Logger
}

// SetAIExtractor wires the transparent fallback to AI extractor on template failure (13_email_parser.md §7) [GATE].
func (h *ApplySenderTemplateHandler) SetAIExtractor(aiExtractor *InvokeAIExtractorHandler, asynqClient *asynq.Client) {
	h.aiExtractorCmd = aiExtractor
	h.asynqClient = asynqClient
}

// NewApplySenderTemplateHandler creates a new ApplySenderTemplateHandler.
func NewApplySenderTemplateHandler(
	matcher *executor.TemplateMatcher,
	extractionRepo domain.ExtractionResultRepository,
	outboxRepo domain.OutboxRepository,
	logger *slog.Logger,
) *ApplySenderTemplateHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ApplySenderTemplateHandler{
		matcher:        matcher,
		executor:       executor.NewExtractionExecutor(),
		extractionRepo: extractionRepo,
		outboxRepo:     outboxRepo,
		logger:         logger,
	}
}

// Handle executes template matching, rule evaluation, transactional recording, and outbox emission (Diagram 3).
func (h *ApplySenderTemplateHandler) Handle(
	ctx context.Context,
	cmd ApplySenderTemplateCommand,
) (*ApplySenderTemplateResult, error) {
	// 1. Resolve matching active template
	tmpl, err := h.matcher.MatchSender(ctx, cmd.RawEmail.Sender)
	if err != nil {
		h.logger.Warn("no matching template found for sender",
			slog.String("sender", cmd.RawEmail.Sender),
			slog.String("messageId", cmd.MessageID),
		)
		h.recordExtractionFailure(ctx, cmd, nil, "no active template found for sender", nil)
		return nil, domain.ErrTemplateNotFound
	}

	// 2. Deterministically apply template rules (Zero LLM calls per GATE)
	res := h.executor.Execute(tmpl, &cmd.RawEmail, cmd.UserID)

	// 3. Handle extraction failure per 13_email_parser.md §5.2 & §7
	if res.Outcome == executor.OutcomeFailed {
		tmplVer := fmt.Sprintf("v%d", tmpl.Version())
		h.recordExtractionFailure(ctx, cmd, &tmplVer, "required field extraction failed", res.MissingFields)

		// Increment Prometheus failure metric
		observability.GetMetrics().UnderstandingTemplateFailuresTotal.WithLabelValues(
			tmpl.SenderPattern(),
			"required_field_missing",
		).Inc()

		h.logger.Warn("template extraction failed, executing transparent AI fallback",
			slog.String("templatePattern", tmpl.SenderPattern()),
			slog.Int("version", tmpl.Version()),
			slog.Any("missingFields", res.MissingFields),
			slog.String("messageId", cmd.MessageID),
		)

		// [GATE - Fallback Wiring]: Transparent hand-off to AI Extractor on template failure (13_email_parser.md §7)
		if h.asynqClient != nil {
			aiPayload := InvokeAIExtractorCommand{
				MessageID:      cmd.MessageID,
				GmailAccountID: cmd.RawEmail.GmailAccountID,
				UserID:         cmd.UserID,
				RawEmail:       cmd.RawEmail,
				TriggerReason:  "template_failed",
			}
			payloadBytes, _ := json.Marshal(aiPayload)
			task := asynq.NewTask(TaskInvokeAIExtractor, payloadBytes)
			_, _ = h.asynqClient.Enqueue(task, asynq.Queue(QueueAI))
		} else if h.aiExtractorCmd != nil {
			aiRes, err := h.aiExtractorCmd.Handle(ctx, InvokeAIExtractorCommand{
				MessageID:      cmd.MessageID,
				GmailAccountID: cmd.RawEmail.GmailAccountID,
				UserID:         cmd.UserID,
				RawEmail:       cmd.RawEmail,
				TriggerReason:  "template_failed",
			})
			if err == nil && aiRes != nil && aiRes.Candidate != nil {
				return &ApplySenderTemplateResult{
					Outcome:             aiRes.Outcome,
					ConfidenceAggregate: aiRes.ConfidenceAggregate,
					Candidate:           aiRes.Candidate,
					ExtractionResultID:  aiRes.ExtractionResultID,
				}, nil
			}
		}

		return &ApplySenderTemplateResult{
			Outcome:             string(res.Outcome),
			ConfidenceAggregate: 0.0,
			MissingFields:       res.MissingFields,
		}, domain.ErrExtractionExecutionFailed
	}

	// 4. Handle success (Full match or Partial match)
	tmplVer := fmt.Sprintf("v%d", tmpl.Version())
	resultID := uuid.New().String()

	// 4a. Persist extraction result record
	rec := &domain.ExtractionResultRecord{
		ID:                  resultID,
		IngestedMessageID:   cmd.MessageID,
		Method:              domain.ExtractionMethodTemplate,
		TemplateVersion:     &tmplVer,
		CandidatePayload:    *res.Candidate,
		ConfidenceAggregate: res.ConfidenceAggregate,
		ConfidencePerField:  res.ConfidencePerField,
		CreatedAt:           time.Now().UTC(),
	}
	if err := h.extractionRepo.SaveResult(ctx, rec); err != nil {
		return nil, fmt.Errorf("persisting extraction result: %w", err)
	}

	// 4b. Record outbox events (Diagram 3)
	_ = h.outboxRepo.RecordOutboxEvent(
		ctx,
		"extraction_result",
		resultID,
		domain.EventTemplateExtractionSucceeded,
		domain.TemplateExtractionSucceededPayload{
			IngestedMessageID: cmd.MessageID,
			TemplateID:        tmpl.ID(),
			SenderPattern:     tmpl.SenderPattern(),
			TemplateVersion:   tmpl.Version(),
			Confidence:        res.ConfidenceAggregate,
			OccurredAt:        time.Now().UTC(),
		},
	)

	// [GATE]: FinancialEventCandidateCreated outbox event for Financial Core confidence gating
	_ = h.outboxRepo.RecordOutboxEvent(
		ctx,
		"candidate_event",
		resultID,
		domain.EventFinancialEventCandidateCreated,
		domain.FinancialEventCandidateCreatedPayload{
			ExtractionResultID: resultID,
			IngestedMessageID:  cmd.MessageID,
			Candidate:          *res.Candidate,
			OccurredAt:         time.Now().UTC(),
		},
	)

	// 4c. Increment metrics
	observability.GetMetrics().UnderstandingTemplateMatchesTotal.WithLabelValues(
		tmpl.SenderPattern(),
		string(res.Outcome),
	).Inc()

	h.logger.Info("template extraction completed successfully",
		slog.String("senderPattern", tmpl.SenderPattern()),
		slog.Int("version", tmpl.Version()),
		slog.String("outcome", string(res.Outcome)),
		slog.Float64("confidence", res.ConfidenceAggregate),
		slog.String("messageId", cmd.MessageID),
	)

	return &ApplySenderTemplateResult{
		Outcome:             string(res.Outcome),
		ConfidenceAggregate: res.ConfidenceAggregate,
		Candidate:           res.Candidate,
		ExtractionResultID:  resultID,
	}, nil
}

func (h *ApplySenderTemplateHandler) recordExtractionFailure(
	ctx context.Context,
	cmd ApplySenderTemplateCommand,
	tmplVer *string,
	reason string,
	missingFields []string,
) {
	resultID := uuid.New().String()

	// Persist failure result in extraction_results
	_ = h.extractionRepo.SaveResult(ctx, &domain.ExtractionResultRecord{
		ID:                  resultID,
		IngestedMessageID:   cmd.MessageID,
		Method:              domain.ExtractionMethodTemplate,
		TemplateVersion:     tmplVer,
		ConfidenceAggregate: 0.0,
		CreatedAt:           time.Now().UTC(),
	})

	// Emit TemplateExtractionFailed outbox event (triggers AI fallback policy per Diagram 3 / 13_email_parser.md §7)
	_ = h.outboxRepo.RecordOutboxEvent(
		ctx,
		"extraction_result",
		resultID,
		domain.EventTemplateExtractionFailed,
		domain.TemplateExtractionFailedPayload{
			IngestedMessageID: cmd.MessageID,
			TemplateVersion:   0,
			Reason:            reason,
			MissingFields:     missingFields,
			OccurredAt:        time.Now().UTC(),
		},
	)
}
