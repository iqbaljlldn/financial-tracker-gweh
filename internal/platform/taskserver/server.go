package taskserver

import (
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

// Queue definitions according to 06_architecture.md §6.3.
const (
	QueueIngestionCritical     = "ingestion:critical"
	QueueIngestionBulk         = "ingestion:bulk"
	QueueUnderstandingTemplate = "understanding:template"
	QueueUnderstandingAI       = "understanding:ai"
	QueueFinancialCoreFinalize = "financialcore:finalize"
	QueueAnalyticsProject      = "analytics:project"
	QueueBudgetEvaluate        = "budget:evaluate"
	QueueEngagementCompose     = "engagement:compose"
	QueueEngagementDeliver     = "engagement:deliver"
	QueueDeadLetter            = "dead-letter"
)

// Task names for Ingestion module.
const (
	TaskStartInitialFullSync  = "ingestion:start_initial_full_sync"
	TaskRegisterGmailWatch    = "ingestion:register_gmail_watch"
	TaskRenewExpiringWatches  = "ingestion:renew_expiring_watches"
	TaskRunIncrementalSync    = "ingestion:run_incremental_sync"
	TaskScheduledPollFallback = "ingestion:scheduled_poll_fallback"
	TaskMessageIngested       = "ingestion:message_ingested"
)

// Task names for Understanding module.
const (
	TaskClassifyAndRouteMessage = "understanding:classify_and_route"
	TaskApplySenderTemplate     = "understanding:apply_template"
	TaskInvokeAIExtractor       = "understanding:invoke_ai"
	TaskRollupParserMetrics     = "understanding:rollup_parser_metrics"
)

// Task names for Financial Core module.
const (
	TaskEvaluateConfidenceGate = "financialcore:evaluate_confidence_gate"
	TaskArchiveFinancialEvents = "financialcore:archive_financial_events"
)

// Task names for Budget module.
const (
	TaskEvaluateBudgetImpact = "budget:evaluate_impact"
)

// DefaultQueueWeights provides standard priority weighting per 06_architecture.md §6.3.
var DefaultQueueWeights = map[string]int{
	QueueIngestionCritical:     6,
	QueueIngestionBulk:         3,
	QueueUnderstandingTemplate: 6,
	QueueUnderstandingAI:       3,
	QueueFinancialCoreFinalize: 6,
	QueueAnalyticsProject:      3,
	QueueBudgetEvaluate:        3,
	QueueEngagementCompose:     3,
	QueueEngagementDeliver:     6,
	"default":                  1,
}

// NewClient constructs an Asynq client connected to the given Redis URI.
func NewClient(redisURI string) (*asynq.Client, error) {
	if redisURI == "" {
		redisURI = "redis://localhost:6380/0"
	}

	redisOpt, err := asynq.ParseRedisURI(redisURI)
	if err != nil {
		return nil, fmt.Errorf("taskserver: parse redis uri: %w", err)
	}

	return asynq.NewClient(redisOpt), nil
}

// ServerConfig configures an Asynq background worker task server.
type ServerConfig struct {
	RedisURI     string
	Concurrency  int
	QueueWeights map[string]int
	Logger       *slog.Logger
}

// NewServer constructs an Asynq worker server configured with weighted queues.
func NewServer(cfg ServerConfig) (*asynq.Server, error) {
	redisURI := cfg.RedisURI
	if redisURI == "" {
		redisURI = "redis://localhost:6380/0"
	}

	redisOpt, err := asynq.ParseRedisURI(redisURI)
	if err != nil {
		return nil, fmt.Errorf("taskserver: parse redis uri: %w", err)
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	weights := cfg.QueueWeights
	if len(weights) == 0 {
		weights = DefaultQueueWeights
	}

	asynqCfg := asynq.Config{
		Concurrency: concurrency,
		Queues:      weights,
	}

	return asynq.NewServer(redisOpt, asynqCfg), nil
}
