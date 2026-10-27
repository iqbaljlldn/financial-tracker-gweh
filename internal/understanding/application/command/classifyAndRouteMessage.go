package command

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/classifier"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/executor"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// Task names for Asynq workers per 07_sequence_diagrams.md Diagram 3 & 4
const (
	TaskClassifyAndRouteMessage = "understanding:classify_and_route"
	TaskApplySenderTemplate     = "understanding:apply_template"
	TaskInvokeAIExtractor       = "understanding:invoke_ai" // Fallback queue for unknown/failed extractions
)

// Queue names per 06_architecture.md §6.5 & Diagram 3
const (
	QueueTemplate = "understanding:template"
	QueueAI       = "understanding:ai"
)

// ClassifyAndRouteMessageCommand payload consumed from outbox or Asynq queue.
type ClassifyAndRouteMessageCommand struct {
	MessageID      string                 `json:"messageId"`
	GmailAccountID string                 `json:"gmailAccountId"`
	UserID         string                 `json:"userId"`
	RawEmail       domain.RawEmailMessage `json:"rawEmail"`
}

// ClassifyAndRouteMessageResult represents the routing decision outcome.
type ClassifyAndRouteMessageResult struct {
	Route      string `json:"route"` // "template_parser" | "ai_extractor" | "discarded"
	TemplateID string `json:"templateId,omitempty"`
}

// ClassifyAndRouteMessageHandler authoritatively decides between deterministic template parsing and AI fallback.
type ClassifyAndRouteMessageHandler struct {
	matcher     *executor.TemplateMatcher
	applyCmd    *ApplySenderTemplateHandler
	aiCmd       *InvokeAIExtractorHandler
	asynqClient *asynq.Client
	logger      *slog.Logger
}

// SetAIExtractor configures the AI extractor command handler for direct or queued execution.
func (h *ClassifyAndRouteMessageHandler) SetAIExtractor(aiCmd *InvokeAIExtractorHandler) {
	h.aiCmd = aiCmd
}

// NewClassifyAndRouteMessageHandler creates a new ClassifyAndRouteMessageHandler.
func NewClassifyAndRouteMessageHandler(
	matcher *executor.TemplateMatcher,
	applyCmd *ApplySenderTemplateHandler,
	asynqClient *asynq.Client,
	logger *slog.Logger,
) *ClassifyAndRouteMessageHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ClassifyAndRouteMessageHandler{
		matcher:     matcher,
		applyCmd:    applyCmd,
		asynqClient: asynqClient,
		logger:      logger,
	}
}

// Handle inspects the sender pattern, chooses the appropriate extraction path, and enqueues/executes it.
func (h *ClassifyAndRouteMessageHandler) Handle(
	ctx context.Context,
	cmd ClassifyAndRouteMessageCommand,
) (*ClassifyAndRouteMessageResult, error) {
	// 1. Authoritative known-sender resolution using Redis-cached matcher (06_architecture.md §13)
	tmpl, err := h.matcher.MatchSender(ctx, cmd.RawEmail.Sender)
	if err == nil && tmpl != nil {
		// Sender is known and has an active template -> route to template_parser (Diagram 3)
		h.logger.Info("routing message to deterministic template parser",
			slog.String("sender", cmd.RawEmail.Sender),
			slog.String("templatePattern", tmpl.SenderPattern()),
			slog.Int("version", tmpl.Version()),
			slog.String("messageId", cmd.MessageID),
		)

		applyCmdPayload := ApplySenderTemplateCommand{
			MessageID: cmd.MessageID,
			UserID:    cmd.UserID,
			RawEmail:  cmd.RawEmail,
		}

		// Enqueue to Asynq queue understanding:template if client is configured
		if h.asynqClient != nil {
			payloadBytes, _ := json.Marshal(applyCmdPayload)
			task := asynq.NewTask(TaskApplySenderTemplate, payloadBytes)
			_, enqueueErr := h.asynqClient.Enqueue(task, asynq.Queue(QueueTemplate))
			if enqueueErr != nil {
				h.logger.Warn("failed to enqueue to understanding:template, running in-process",
					slog.String("error", enqueueErr.Error()))
				_, _ = h.applyCmd.Handle(ctx, applyCmdPayload)
			}
		} else {
			// In-process fallback (e.g. tests or synchronous mode)
			_, _ = h.applyCmd.Handle(ctx, applyCmdPayload)
		}

		return &ClassifyAndRouteMessageResult{
			Route:      "template_parser",
			TemplateID: tmpl.ID(),
		}, nil
	}

	// 2. Unknown sender: evaluate transactional-content heuristic per 12_gmail_connector.md §7.1 [GATE]
	if isNonTransactional, reason := classifier.EvaluateTransactionalContent(cmd.RawEmail); isNonTransactional {
		h.logger.Info("non-transactional email discarded by heuristic",
			slog.String("sender", cmd.RawEmail.Sender),
			slog.String("subject", cmd.RawEmail.Subject),
			slog.String("messageId", cmd.MessageID),
			slog.String("reason", reason),
		)
		return &ClassifyAndRouteMessageResult{
			Route: "discarded",
		}, nil
	}

	// 3. Positive or ambiguous transactional heuristic -> routes to AI extractor fallback queue (11_ai_blueprint.md / Diagram 4)
	h.logger.Info("unknown sender: routing to AI extractor fallback queue",
		slog.String("sender", cmd.RawEmail.Sender),
		slog.String("messageId", cmd.MessageID),
	)

	aiCmdPayload := InvokeAIExtractorCommand{
		MessageID:      cmd.MessageID,
		GmailAccountID: cmd.GmailAccountID,
		UserID:         cmd.UserID,
		RawEmail:       cmd.RawEmail,
		TriggerReason:  "unknown_sender",
	}

	if h.asynqClient != nil {
		payloadBytes, _ := json.Marshal(aiCmdPayload)
		task := asynq.NewTask(TaskInvokeAIExtractor, payloadBytes)
		_, _ = h.asynqClient.Enqueue(task, asynq.Queue(QueueAI))
	} else if h.aiCmd != nil {
		_, _ = h.aiCmd.Handle(ctx, aiCmdPayload)
	}

	return &ClassifyAndRouteMessageResult{
		Route: "ai_extractor",
	}, nil
}
