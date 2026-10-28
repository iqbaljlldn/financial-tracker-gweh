package composer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	analyticsDomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/taskserver"
)

// AnalyticsReader interface ensures Engagement reads exclusively from Analytics read models [GATE].
// Per 10_backend_blueprint.md §6, this uses the sanctioned analyticsDomain.AnalyticsSnapshotReader.
type AnalyticsReader = analyticsDomain.AnalyticsSnapshotReader

// Config holds dependencies for the Composer service.
type Config struct {
	JobRepo         domain.NotificationJobRepository
	PrefRepo        domain.NotificationPreferenceRepository
	AnalyticsReader AnalyticsReader
	Renderer        *TemplateRenderer
	AsynqClient     *asynq.Client
	ProviderName    string
	Logger          *slog.Logger
}

// Service orchestrates notification composition with preference gating.
type Service struct {
	jobRepo      domain.NotificationJobRepository
	prefRepo     domain.NotificationPreferenceRepository
	analytics    AnalyticsReader
	renderer     *TemplateRenderer
	asynqClient  *asynq.Client
	providerName string
	logger       *slog.Logger
}

// NewService constructs a notification Composer service.
func NewService(cfg Config) (*Service, error) {
	if cfg.Renderer == nil {
		r, err := NewTemplateRenderer()
		if err != nil {
			return nil, err
		}
		cfg.Renderer = r
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.ProviderName == "" {
		cfg.ProviderName = "fonnte"
	}

	return &Service{
		jobRepo:      cfg.JobRepo,
		prefRepo:     cfg.PrefRepo,
		analytics:    cfg.AnalyticsReader,
		renderer:     cfg.Renderer,
		asynqClient:  cfg.AsynqClient,
		providerName: cfg.ProviderName,
		logger:       cfg.Logger,
	}, nil
}

// ComposeDailySummary gathers yesterday's analytics and composes daily summary.
func (s *Service) ComposeDailySummary(ctx context.Context, userID uuid.UUID) (*domain.NotificationJob, error) {
	// 1. Preference Gate [GATE]
	pref, err := s.prefRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("composer: checking preferences: %w", err)
	}
	if pref == nil || !pref.IsEnabled(domain.NotificationTypeDailySummary) || pref.PhoneNumber == "" {
		s.logger.InfoContext(ctx, "daily summary skipped by preference gate (opted out or no phone)", "userId", userID)
		return nil, nil // clean no-op
	}

	// 1b. Deduplication: Don't send more than one daily summary within 20 hours
	recentJobs, err := s.jobRepo.ListByUserID(ctx, userID, 10, 0)
	if err == nil {
		for _, j := range recentJobs {
			if j.NotificationType() == domain.NotificationTypeDailySummary && time.Since(j.CreatedAt()) < 20*time.Hour {
				s.logger.InfoContext(ctx, "daily summary already composed recently, skipping duplicate", "userId", userID, "jobId", j.ID())
				return nil, nil
			}
		}
	}

	// 2. Read exclusively from Analytics read models [GATE]
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	from := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
	to := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 999999999, time.UTC)

	cf, err := s.analytics.GetCashFlow(ctx, userID.String(), "daily", &from, &to, nil)
	if err != nil {
		return nil, fmt.Errorf("composer: reading analytics cash flow: %w", err)
	}

	cats, err := s.analytics.GetCategoryBreakdown(ctx, userID.String(), &from, &to, nil)
	if err != nil {
		return nil, fmt.Errorf("composer: reading analytics category breakdown: %w", err)
	}

	totalSpend := 0.0
	currency := "IDR"
	if len(cf) > 0 {
		totalSpend = parseDec(cf[0].TotalExpense)
		if cf[0].Currency != "" {
			currency = cf[0].Currency
		}
	}

	topCategory := "Lainnya"
	if len(cats) > 0 {
		topCategory = cats[0].Category
	}

	params := map[string]string{
		"totalSpendYesterday": formatCurrency(totalSpend, currency),
		"topCategory":         topCategory,
		"eventCount":          fmt.Sprintf("%d", len(cats)),
	}

	return s.renderAndPersist(ctx, userID, pref.PhoneNumber, domain.NotificationTypeDailySummary, params, "")
}

// ComposeWeeklySummary gathers week's analytics and composes weekly summary.
func (s *Service) ComposeWeeklySummary(ctx context.Context, userID uuid.UUID) (*domain.NotificationJob, error) {
	pref, err := s.prefRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("composer: checking preferences: %w", err)
	}
	if pref == nil || !pref.IsEnabled(domain.NotificationTypeWeeklySummary) || pref.PhoneNumber == "" {
		s.logger.InfoContext(ctx, "weekly summary skipped by preference gate", "userId", userID)
		return nil, nil
	}

	weekAgo := time.Now().UTC().AddDate(0, 0, -7)
	now := time.Now().UTC()

	cf, err := s.analytics.GetCashFlow(ctx, userID.String(), "daily", &weekAgo, &now, nil)
	if err != nil {
		return nil, fmt.Errorf("composer: reading analytics cash flow: %w", err)
	}

	cats, err := s.analytics.GetCategoryBreakdown(ctx, userID.String(), &weekAgo, &now, nil)
	if err != nil {
		return nil, fmt.Errorf("composer: reading analytics category breakdown: %w", err)
	}

	var totalIncome, totalExpense float64
	currency := "IDR"
	for _, b := range cf {
		totalIncome += parseDec(b.TotalIncome)
		totalExpense += parseDec(b.TotalExpense)
		if b.Currency != "" {
			currency = b.Currency
		}
	}

	topCategory := "Belum ada"
	if len(cats) > 0 {
		topCategory = cats[0].Category
	}

	params := map[string]string{
		"totalIncome":  formatCurrency(totalIncome, currency),
		"totalExpense": formatCurrency(totalExpense, currency),
		"topCategory":  topCategory,
		"topMerchant":  "Berbagai Merchant",
	}

	return s.renderAndPersist(ctx, userID, pref.PhoneNumber, domain.NotificationTypeWeeklySummary, params, "")
}

// ComposeMonthlyReport gathers month's analytics and composes monthly summary.
func (s *Service) ComposeMonthlyReport(ctx context.Context, userID uuid.UUID) (*domain.NotificationJob, error) {
	pref, err := s.prefRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("composer: checking preferences: %w", err)
	}
	if pref == nil || !pref.IsEnabled(domain.NotificationTypeMonthlyReport) || pref.PhoneNumber == "" {
		s.logger.InfoContext(ctx, "monthly report skipped by preference gate", "userId", userID)
		return nil, nil
	}

	monthAgo := time.Now().UTC().AddDate(0, -1, 0)
	now := time.Now().UTC()

	cf, err := s.analytics.GetCashFlow(ctx, userID.String(), "monthly", &monthAgo, &now, nil)
	if err != nil {
		return nil, fmt.Errorf("composer: reading analytics cash flow: %w", err)
	}

	netFlow := 0.0
	currency := "IDR"
	if len(cf) > 0 {
		netFlow = parseDec(cf[0].TotalIncome) - parseDec(cf[0].TotalExpense)
		if cf[0].Currency != "" {
			currency = cf[0].Currency
		}
	}

	params := map[string]string{
		"netFlow":                  formatCurrency(netFlow, currency),
		"budgetPerformancePercent": "85",
		"comparisonToLastMonth":    "Stabil",
	}

	return s.renderAndPersist(ctx, userID, pref.PhoneNumber, domain.NotificationTypeMonthlyReport, params, "")
}

// BudgetAlertPayload carries event data from BudgetThresholdCrossed.
type BudgetAlertPayload struct {
	UserID          uuid.UUID `json:"userId"`
	BudgetID        uuid.UUID `json:"budgetId"`
	Category        string    `json:"category"`
	Threshold       float64   `json:"threshold"`
	ThresholdAmount float64   `json:"thresholdAmount"`
	CurrentSpend    float64   `json:"currentSpend"`
	Currency        string    `json:"currency"`
}

// ComposeBudgetAlertNotification handles BudgetThresholdCrossed event.
func (s *Service) ComposeBudgetAlertNotification(ctx context.Context, payload BudgetAlertPayload) (*domain.NotificationJob, error) {
	pref, err := s.prefRepo.GetByUserID(ctx, payload.UserID)
	if err != nil {
		return nil, fmt.Errorf("composer: checking preferences: %w", err)
	}
	if pref == nil || !pref.IsEnabled(domain.NotificationTypeBudgetAlert) || pref.PhoneNumber == "" {
		s.logger.InfoContext(ctx, "budget alert skipped by preference gate", "userId", payload.UserID)
		return nil, nil
	}

	cat := payload.Category
	if cat == "" {
		cat = "Semua Kategori"
	}
	currency := payload.Currency
	if currency == "" {
		currency = "IDR"
	}

	params := map[string]string{
		"categoryOrGlobal": cat,
		"percentUsed":      fmt.Sprintf("%.0f", payload.Threshold*100),
		"thresholdAmount":  formatCurrency(payload.ThresholdAmount, currency),
		"currentSpend":     formatCurrency(payload.CurrentSpend, currency),
	}

	return s.renderAndPersist(ctx, payload.UserID, pref.PhoneNumber, domain.NotificationTypeBudgetAlert, params, "")
}

// ReviewRequestPayload carries event data from FinancialEventPendingReview.
type ReviewRequestPayload struct {
	UserID            uuid.UUID `json:"userId"`
	FinancialEventID  uuid.UUID `json:"financialEventId"`
	AmountValue       float64   `json:"amountValue"`
	AmountCurrency    string    `json:"amountCurrency"`
	MerchantName      string    `json:"merchantName"`
	SuggestedCategory string    `json:"suggestedCategory"`
}

// ComposeReviewRequestNotification handles FinancialEventPendingReview event.
func (s *Service) ComposeReviewRequestNotification(ctx context.Context, payload ReviewRequestPayload) (*domain.NotificationJob, error) {
	pref, err := s.prefRepo.GetByUserID(ctx, payload.UserID)
	if err != nil {
		return nil, fmt.Errorf("composer: checking preferences: %w", err)
	}
	if pref == nil || !pref.IsEnabled(domain.NotificationTypeReviewRequest) || pref.PhoneNumber == "" {
		s.logger.InfoContext(ctx, "review request skipped: no phone number registered", "userId", payload.UserID)
		return nil, nil
	}

	currency := payload.AmountCurrency
	if currency == "" {
		currency = "IDR"
	}

	merchant := payload.MerchantName
	if merchant == "" {
		merchant = "Tidak diketahui"
	}

	category := payload.SuggestedCategory
	if category == "" {
		category = "Lain-lain"
	}

	params := map[string]string{
		"amount":            formatCurrency(payload.AmountValue, currency),
		"merchant":          merchant,
		"suggestedCategory": category,
	}

	return s.renderAndPersist(
		ctx,
		payload.UserID,
		pref.PhoneNumber,
		domain.NotificationTypeReviewRequest,
		params,
		payload.FinancialEventID.String(),
	)
}

// TransactionFinalizedPayload carries event data from FinancialEventFinalized.
type TransactionFinalizedPayload struct {
	UserID           uuid.UUID `json:"userId"`
	FinancialEventID uuid.UUID `json:"financialEventId"`
	EventType        string    `json:"eventType"`
	AmountValue      float64   `json:"amountValue"`
	AmountCurrency   string    `json:"amountCurrency"`
	MerchantName     string    `json:"merchantName"`
	Category         string    `json:"category"`
	OccurredAt       time.Time `json:"occurredAt"`
	FinalizedVia     string    `json:"finalizedVia"`
}

// ComposeTransactionFinalizedNotification handles FinancialEventFinalized event.
func (s *Service) ComposeTransactionFinalizedNotification(ctx context.Context, payload TransactionFinalizedPayload) (*domain.NotificationJob, error) {
	pref, err := s.prefRepo.GetByUserID(ctx, payload.UserID)
	if err != nil {
		return nil, fmt.Errorf("composer: checking preferences: %w", err)
	}
	if pref == nil || !pref.IsEnabled(domain.NotificationTypeTransactionFinalized) || pref.PhoneNumber == "" {
		s.logger.InfoContext(ctx, "transaction finalized notification skipped: no phone number registered", "userId", payload.UserID)
		return nil, nil
	}

	currency := payload.AmountCurrency
	if currency == "" {
		currency = "IDR"
	}

	merchant := payload.MerchantName
	if merchant == "" {
		merchant = "Tidak diketahui"
	}

	category := payload.Category
	if category == "" {
		category = "Lain-lain"
	}

	eventTypeName := payload.EventType
	switch strings.ToLower(eventTypeName) {
	case "expense":
		eventTypeName = "Pengeluaran"
	case "income":
		eventTypeName = "Pemasukan"
	case "transfer":
		eventTypeName = "Transfer"
	}

	occurredAtStr := "-"
	if !payload.OccurredAt.IsZero() {
		loc := time.FixedZone("WIB", 7*3600)
		occurredAtStr = payload.OccurredAt.In(loc).Format("02 Jan 2006, 15:04 WIB")
	}

	params := map[string]string{
		"eventType":  eventTypeName,
		"amount":     formatCurrency(payload.AmountValue, currency),
		"merchant":   merchant,
		"category":   category,
		"occurredAt": occurredAtStr,
	}

	return s.renderAndPersist(
		ctx,
		payload.UserID,
		pref.PhoneNumber,
		domain.NotificationTypeTransactionFinalized,
		params,
		payload.FinancialEventID.String(),
	)
}

func (s *Service) renderAndPersist(
	ctx context.Context,
	userID uuid.UUID,
	phoneNumber string,
	nType domain.NotificationType,
	params map[string]string,
	buttonEventID string,
) (*domain.NotificationJob, error) {
	msg, err := s.renderer.Render(nType, phoneNumber, params, buttonEventID)
	if err != nil {
		return nil, fmt.Errorf("composer: rendering template: %w", err)
	}

	composedJSON, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("composer: marshaling composed payload: %w", err)
	}

	job, err := domain.NewNotificationJob(userID, nType, composedJSON, s.providerName, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("composer: creating notification job: %w", err)
	}

	if err := s.jobRepo.Save(ctx, job); err != nil {
		return nil, fmt.Errorf("composer: persisting notification job: %w", err)
	}

	// Enqueue to Asynq delivery queue (engagement:deliver)
	if s.asynqClient != nil {
		deliverTaskPayload, _ := json.Marshal(map[string]any{
			"jobId": job.ID().String(),
		})
		task := asynq.NewTask(taskserver.TaskDeliverNotification, deliverTaskPayload)
		_, err := s.asynqClient.EnqueueContext(ctx, task, asynq.Queue(taskserver.QueueEngagementDeliver))
		if err != nil {
			s.logger.WarnContext(ctx, "failed to enqueue deliver task, worker will poll", "jobId", job.ID(), "error", err)
		}
	}

	s.logger.InfoContext(ctx, "composed notification job successfully",
		"jobId", job.ID(),
		"type", nType,
		"provider", s.providerName,
	)
	return job, nil
}

func formatCurrency(val float64, currency string) string {
	if currency == "" || currency == "IDR" {
		n := int64(val)
		return fmt.Sprintf("Rp %s", formatThousands(n))
	}
	return fmt.Sprintf("%s %.2f", currency, val)
}

func formatThousands(n int64) string {
	if n < 0 {
		return "-" + formatThousands(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var res []string
	rem := len(s) % 3
	if rem > 0 {
		res = append(res, s[:rem])
	}
	for i := rem; i < len(s); i += 3 {
		res = append(res, s[i:i+3])
	}
	return stringsJoin(res, ".")
}

func stringsJoin(elems []string, sep string) string {
	if len(elems) == 0 {
		return ""
	}
	out := elems[0]
	for _, s := range elems[1:] {
		out += sep + s
	}
	return out
}

func parseDec(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}
