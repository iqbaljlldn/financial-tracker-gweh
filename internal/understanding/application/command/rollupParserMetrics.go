package command

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// TaskRollupParserMetrics task name for scheduled Asynq/cron execution.
const TaskRollupParserMetrics = "understanding:rollup_parser_metrics"

// RollupParserMetricsCommand payload for rolling metrics computation.
type RollupParserMetricsCommand struct {
	Trigger string `json:"trigger"` // "scheduled" | "manual"
}

// TemplateMetricRollup contains computed failure rates for a single template.
type TemplateMetricRollup struct {
	SenderPattern string  `json:"senderPattern"`
	Version       int     `json:"version"`
	Total1h       int     `json:"total1h"`
	Failed1h      int     `json:"failed1h"`
	Rate1h        float64 `json:"rate1h"`
	Total24h      int     `json:"total24h"`
	Failed24h     int     `json:"failed24h"`
	Rate24h       float64 `json:"rate24h"`
}

// RollupParserMetricsResult contains rollups for all active templates.
type RollupParserMetricsResult struct {
	ExaminedTemplates int                    `json:"examinedTemplates"`
	Rollups           []TemplateMetricRollup `json:"rollups"`
	SpikeAlerts       []string               `json:"spikeAlerts,omitempty"`
}

// RollupParserMetricsHandler computes rolling failure rates and updates Prometheus gauges.
type RollupParserMetricsHandler struct {
	templateRepo   domain.SenderTemplateRepository
	extractionRepo domain.ExtractionResultRepository
	logger         *slog.Logger
}

// NewRollupParserMetricsHandler creates a new RollupParserMetricsHandler.
func NewRollupParserMetricsHandler(
	templateRepo domain.SenderTemplateRepository,
	extractionRepo domain.ExtractionResultRepository,
	logger *slog.Logger,
) *RollupParserMetricsHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RollupParserMetricsHandler{
		templateRepo:   templateRepo,
		extractionRepo: extractionRepo,
		logger:         logger,
	}
}

// Handle calculates rolling failure-rates for trailing 1h and 24h per 13_email_parser.md §8.1.
func (h *RollupParserMetricsHandler) Handle(
	ctx context.Context,
	_ RollupParserMetricsCommand,
) (*RollupParserMetricsResult, error) {
	templates, err := h.templateRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing active templates for metrics rollup: %w", err)
	}

	now := time.Now().UTC()
	since1h := now.Add(-1 * time.Hour)
	since24h := now.Add(-24 * time.Hour)

	var rollups []TemplateMetricRollup
	var spikeAlerts []string

	for _, tmpl := range templates {
		tmplVer := fmt.Sprintf("v%d", tmpl.Version())

		// Trailing 1h stats
		tot1h, fail1h, err := h.extractionRepo.GetTemplateStatsInWindow(ctx, tmplVer, since1h)
		if err != nil {
			h.logger.Warn("failed to fetch 1h stats", slog.String("template", tmpl.SenderPattern()), slog.String("error", err.Error()))
			continue
		}
		var rate1h float64
		if tot1h > 0 {
			rate1h = float64(fail1h) / float64(tot1h)
		}

		// Trailing 24h stats
		tot24h, fail24h, err := h.extractionRepo.GetTemplateStatsInWindow(ctx, tmplVer, since24h)
		if err != nil {
			h.logger.Warn("failed to fetch 24h stats", slog.String("template", tmpl.SenderPattern()), slog.String("error", err.Error()))
			continue
		}
		var rate24h float64
		if tot24h > 0 {
			rate24h = float64(fail24h) / float64(tot24h)
		}

		// Update Prometheus Gauges per 17_observability.md §4.2
		observability.GetMetrics().UnderstandingTemplateFailureRate.WithLabelValues(tmpl.SenderPattern(), "1h").Set(rate1h)
		observability.GetMetrics().UnderstandingTemplateFailureRate.WithLabelValues(tmpl.SenderPattern(), "24h").Set(rate24h)

		roll := TemplateMetricRollup{
			SenderPattern: tmpl.SenderPattern(),
			Version:       tmpl.Version(),
			Total1h:       tot1h,
			Failed1h:      fail1h,
			Rate1h:        rate1h,
			Total24h:      tot24h,
			Failed24h:     fail24h,
			Rate24h:       rate24h,
		}
		rollups = append(rollups, roll)

		// Spike detection check: threshold > 5% (0.05) trailing 1h [GATE per 13_email_parser.md §8.2]
		if tot1h >= 5 && rate1h > 0.05 {
			alertMsg := fmt.Sprintf("SPIKE ALERT: template %s (v%d) failure rate %.1f%% (>5%% threshold) over trailing 1h (%d/%d failed)",
				tmpl.SenderPattern(), tmpl.Version(), rate1h*100, fail1h, tot1h)
			spikeAlerts = append(spikeAlerts, alertMsg)
			h.logger.Error(alertMsg,
				slog.String("senderPattern", tmpl.SenderPattern()),
				slog.Int("version", tmpl.Version()),
				slog.Float64("failureRate1h", rate1h),
				slog.Int("total1h", tot1h),
				slog.Int("failed1h", fail1h),
			)
		}
	}

	h.logger.Info("parser metrics rollup completed",
		slog.Int("examinedTemplates", len(templates)),
		slog.Int("spikeAlerts", len(spikeAlerts)),
	)

	return &RollupParserMetricsResult{
		ExaminedTemplates: len(templates),
		Rollups:           rollups,
		SpikeAlerts:       spikeAlerts,
	}, nil
}
