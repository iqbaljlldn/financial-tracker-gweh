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
