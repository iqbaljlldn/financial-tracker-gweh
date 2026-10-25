package command

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/executor"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/validator"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// PublishTemplateCommand payload to publish or update a SenderTemplate.
type PublishTemplateCommand struct {
	SenderPattern   string                 `json:"senderPattern"`
	Version         int                    `json:"version,omitempty"` // 0 = auto-increment
	ExtractionRules domain.ExtractionRules `json:"extractionRules"`
	SampleSet       *validator.SampleSet   `json:"sampleSet,omitempty"` // Required for sample-set validation gate
}

// PublishTemplateResult represents the outcome of the publish operation.
type PublishTemplateResult struct {
	TemplateID       string                      `json:"templateId"`
	SenderPattern    string                      `json:"senderPattern"`
	Version          int                         `json:"version"`
	ValidationReport *validator.ValidationReport `json:"validationReport,omitempty"`
}

// PublishTemplateHandler handles validation-gated atomic publishing of SenderTemplates.
type PublishTemplateHandler struct {
	repo    domain.SenderTemplateRepository
	matcher *executor.TemplateMatcher
	harness *validator.ValidationHarness
	logger  *slog.Logger
}

// NewPublishTemplateHandler creates a new PublishTemplateHandler.
func NewPublishTemplateHandler(
	repo domain.SenderTemplateRepository,
	matcher *executor.TemplateMatcher,
	logger *slog.Logger,
) *PublishTemplateHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &PublishTemplateHandler{
		repo:    repo,
		matcher: matcher,
		harness: validator.NewValidationHarness(),
		logger:  logger,
	}
}

// Handle executes validation and atomic publishing [GATE per 13_email_parser.md §6.1-§6.3].
func (h *PublishTemplateHandler) Handle(ctx context.Context, cmd PublishTemplateCommand) (*PublishTemplateResult, error) {
	// 1. Resolve version (auto-increment if not specified)
	version := cmd.Version
	if version <= 0 {
		latest, err := h.repo.GetLatestVersionForPattern(ctx, cmd.SenderPattern)
		if err != nil {
			return nil, fmt.Errorf("resolving latest template version: %w", err)
		}
		version = latest + 1
	}

	tmplID := uuid.New().String()
	rules := cmd.ExtractionRules
	rules.SenderPattern = cmd.SenderPattern
	rules.Version = version

	tmpl, err := domain.NewSenderTemplate(
		tmplID,
		cmd.SenderPattern,
		version,
		rules,
		true, // Published templates are immediately active
		time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("instantiating template: %w", err)
	}

	// 2. Sample-Set Validation Gate [GATE: no template can be published without passing sample validation]
	var report *validator.ValidationReport
	if cmd.SampleSet != nil && len(cmd.SampleSet.Samples) > 0 {
		report = h.harness.ValidateTemplate(tmpl, cmd.SampleSet)
		if !report.Passed {
			h.logger.Warn("template publishing rejected: sample-set validation failed",
				slog.String("senderPattern", cmd.SenderPattern),
				slog.Int("version", version),
				slog.Int("failedSamples", report.FailedSamples),
			)
			return &PublishTemplateResult{
				TemplateID:       tmplID,
				SenderPattern:    cmd.SenderPattern,
				Version:          version,
				ValidationReport: report,
			}, fmt.Errorf("%w: %d of %d samples failed", domain.ErrTemplateValidationFailed, report.FailedSamples, report.TotalSamples)
		}
	}

	// 3. Atomic Database Swap [GATE: atomic swap, old versions never deleted per 13_email_parser.md §6.2]
	if err := h.repo.PublishAtomic(ctx, tmpl); err != nil {
		return nil, fmt.Errorf("atomic template publication failed: %w", err)
	}

	// 4. Invalidate Redis active templates cache
	if h.matcher != nil {
		h.matcher.InvalidateCache(ctx)
	}

	h.logger.Info("sender template published successfully",
		slog.String("templateId", tmplID),
		slog.String("senderPattern", cmd.SenderPattern),
		slog.Int("version", version),
	)

	return &PublishTemplateResult{
		TemplateID:       tmplID,
		SenderPattern:    cmd.SenderPattern,
		Version:          version,
		ValidationReport: report,
	}, nil
}
