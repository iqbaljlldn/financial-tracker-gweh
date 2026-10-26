package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the registered Prometheus metric collectors for the platform
// per 17_observability.md §4.
type Metrics struct {
	Registry *prometheus.Registry

	// Ingestion Module Metrics (§4.1)
	GmailAPICallsTotal           *prometheus.CounterVec
	GmailQuotaUnitsConsumedTotal *prometheus.CounterVec
	WatchRenewalSuccessTotal     prometheus.Counter
	WatchRenewalFailureTotal     prometheus.Counter
	WatchExpiringSoon            prometheus.Gauge
	HistorySyncFallbackTotal     *prometheus.CounterVec
	PollFallbackLiveness         prometheus.Gauge // Unix timestamp of last poll-fallback heartbeat [GATE]
	MessagesIngestedTotal        *prometheus.CounterVec
	SyncLagSeconds               *prometheus.HistogramVec

	// Understanding Module Metrics (§4.2 & 13_email_parser.md §8)
	UnderstandingTemplateMatchesTotal      *prometheus.CounterVec
	UnderstandingTemplateFailuresTotal     *prometheus.CounterVec
	UnderstandingTemplateFailureRate       *prometheus.GaugeVec
	UnderstandingTemplateExecutionDuration *prometheus.HistogramVec

	// Understanding AI Module Metrics (17_observability.md §4.2 & 11_ai_blueprint.md §10)
	UnderstandingAIExtractionTotal          *prometheus.CounterVec
	UnderstandingAIConfidenceDistribution   *prometheus.HistogramVec
	UnderstandingAITokenUsageTotal          *prometheus.CounterVec
	UnderstandingAIQueueDepth               *prometheus.GaugeVec
	UnderstandingAIOldestTaskAgeSeconds     *prometheus.GaugeVec
	UnderstandingAIDailyBudgetConsumedRatio *prometheus.GaugeVec

	// Financial Core Module Metrics (§4.3)
	FinancialCoreEventsFinalizedTotal        *prometheus.CounterVec
	FinancialCoreEventsPendingReview         prometheus.Gauge
	FinancialCorePendingReviewSLABreachTotal prometheus.Counter
	FinancialCoreOverridesSubmittedTotal     *prometheus.CounterVec
	FinancialCoreDedupConflictTotal          prometheus.Counter

	// Budget Module Metrics (§4.4)
	BudgetThresholdCrossedTotal     *prometheus.CounterVec
	BudgetEvaluationDurationSeconds prometheus.Histogram
}

var defaultMetrics *Metrics

// InitMetrics initializes the Prometheus metrics registry and registers all platform metrics.
func InitMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		Registry: reg,

		GmailAPICallsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ingestion_gmail_api_calls_total",
				Help: "Total number of outbound Gmail REST API calls partitioned by method and status code.",
			},
			[]string{"method", "status_code"},
		),

		GmailQuotaUnitsConsumedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ingestion_gmail_quota_units_consumed_total",
				Help: "Estimated Gmail API quota units consumed per bucketed account.",
			},
			[]string{"account_id"},
		),

		WatchRenewalSuccessTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "ingestion_watch_renewal_success_total",
				Help: "Total number of successful Gmail users.watch renewals.",
			},
		),

		WatchRenewalFailureTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "ingestion_watch_renewal_failure_total",
				Help: "Total number of failed Gmail users.watch renewals.",
			},
		),

		WatchExpiringSoon: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "ingestion_watch_expiring_soon",
				Help: "Number of active Gmail accounts within 24 hours of watch expiration.",
			},
		),

		HistorySyncFallbackTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ingestion_history_sync_fallback_total",
				Help: "Total number of times History API 404/expiry triggered a bounded fallback re-sync.",
			},
			[]string{"reason"},
		),

		PollFallbackLiveness: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "ingestion_poll_fallback_liveness",
				Help: "Unix epoch timestamp of the latest completed poll-fallback heartbeat run [GATE].",
			},
		),

		MessagesIngestedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ingestion_messages_ingested_total",
				Help: "Total number of email messages ingested, labeled by routing decision.",
			},
			[]string{"routing_decision"},
		),

		SyncLagSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ingestion_sync_lag_seconds",
				Help:    "Latency in seconds between message internal_date and durable ingestion.",
				Buckets: []float64{10, 30, 60, 120, 300, 600, 1800, 3600},
			},
			[]string{"account_id"},
		),

		UnderstandingTemplateMatchesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "understanding_template_matches_total",
				Help: "Total number of successful template parses labeled by sender_pattern and outcome (full/partial).",
			},
			[]string{"sender_pattern", "outcome"},
		),

		UnderstandingTemplateFailuresTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "understanding_template_failures_total",
				Help: "Total number of failed template parse attempts labeled by sender_pattern and reason.",
			},
			[]string{"sender_pattern", "reason"},
		),

		UnderstandingTemplateFailureRate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "understanding_template_failure_rate",
				Help: "Rolling failure rate (0.0 - 1.0) for a template version over a trailing time window (1h/24h).",
			},
			[]string{"sender_pattern", "window"},
		),

		UnderstandingTemplateExecutionDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "understanding_template_execution_seconds",
				Help:    "Execution duration in seconds for deterministic template parsing.",
				Buckets: []float64{0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5},
			},
			[]string{"sender_pattern"},
		),

		UnderstandingAIExtractionTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "understanding_ai_extraction_total",
				Help: "Total number of AI extractions partitioned by outcome (success, schema_invalid, reprompt_success).",
			},
			[]string{"outcome"},
		),

		UnderstandingAIConfidenceDistribution: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "understanding_ai_confidence_distribution",
				Help:    "Aggregate confidence score distribution for AI extractions.",
				Buckets: []float64{0.1, 0.3, 0.5, 0.7, 0.8, 0.85, 0.9, 0.95, 1.0},
			},
			[]string{"extraction_method"},
		),

		UnderstandingAITokenUsageTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "understanding_ai_token_usage_total",
				Help: "Total tokens consumed by LLM extraction calls partitioned by direction (input, output).",
			},
			[]string{"direction"},
		),

		UnderstandingAIQueueDepth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "understanding_ai_queue_depth",
				Help: "Backlog depth of the understanding:ai fallback queue (Hotspot H3 backpressure).",
			},
			[]string{"queue_name"},
		),

		UnderstandingAIOldestTaskAgeSeconds: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "understanding_ai_oldest_task_age_seconds",
				Help: "Age in seconds of the oldest pending task in the understanding:ai fallback queue.",
			},
			[]string{"queue_name"},
		),

		UnderstandingAIDailyBudgetConsumedRatio: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "understanding_ai_daily_budget_consumed_ratio",
				Help: "Ratio of daily AI call budget consumed (0.0 - 1.0) partitioned by scope (platform, per_user).",
			},
			[]string{"scope"},
		),

		FinancialCoreEventsFinalizedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "financialcore_events_finalized_total",
				Help: "Total FinancialEvents finalized, partitioned by event_type and confidence_method.",
			},
			[]string{"event_type", "confidence_method"},
		),

		FinancialCoreEventsPendingReview: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "financialcore_events_pending_review_total",
				Help: "Current number of FinancialEvents awaiting user review.",
			},
		),

		FinancialCorePendingReviewSLABreachTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "financialcore_pending_review_sla_breach_total",
				Help: "Total pending-review events that crossed the review SLA (7-day default) without confirmation.",
			},
		),

		FinancialCoreOverridesSubmittedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "financialcore_overrides_submitted_total",
				Help: "Total manual EventOverrides submitted, partitioned by corrected field_name.",
			},
			[]string{"field_name"},
		),

		FinancialCoreDedupConflictTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "financialcore_dedup_conflict_total",
				Help: "ON CONFLICT DO NOTHING hits on the INV-3 uniqueness constraint (successful idempotent redeliveries).",
			},
		),

		BudgetThresholdCrossedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "budget_threshold_crossed_total",
				Help: "Total number of budget threshold crossings fired per threshold.",
			},
			[]string{"threshold"},
		),

		BudgetEvaluationDurationSeconds: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "budget_evaluation_duration_seconds",
				Help:    "Duration in seconds to evaluate budget impacts upon finalized expense events.",
				Buckets: prometheus.DefBuckets,
			},
		),
	}

	reg.MustRegister(
		m.GmailAPICallsTotal,
		m.GmailQuotaUnitsConsumedTotal,
		m.WatchRenewalSuccessTotal,
		m.WatchRenewalFailureTotal,
		m.WatchExpiringSoon,
		m.HistorySyncFallbackTotal,
		m.PollFallbackLiveness,
		m.MessagesIngestedTotal,
		m.SyncLagSeconds,
		m.UnderstandingTemplateMatchesTotal,
		m.UnderstandingTemplateFailuresTotal,
		m.UnderstandingTemplateFailureRate,
		m.UnderstandingTemplateExecutionDuration,
		m.UnderstandingAIExtractionTotal,
		m.UnderstandingAIConfidenceDistribution,
		m.UnderstandingAITokenUsageTotal,
		m.UnderstandingAIQueueDepth,
		m.UnderstandingAIOldestTaskAgeSeconds,
		m.UnderstandingAIDailyBudgetConsumedRatio,
		m.FinancialCoreEventsFinalizedTotal,
		m.FinancialCoreEventsPendingReview,
		m.FinancialCorePendingReviewSLABreachTotal,
		m.FinancialCoreOverridesSubmittedTotal,
		m.FinancialCoreDedupConflictTotal,
		m.BudgetThresholdCrossedTotal,
		m.BudgetEvaluationDurationSeconds,
	)

	defaultMetrics = m
	return m
}

// GetMetrics returns the singleton metrics instance, initializing it if necessary.
func GetMetrics() *Metrics {
	if defaultMetrics == nil {
		return InitMetrics()
	}
	return defaultMetrics
}

// Handler returns an HTTP handler for Prometheus scraping.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}
