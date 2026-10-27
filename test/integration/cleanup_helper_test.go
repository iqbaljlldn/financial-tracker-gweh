package integration_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// getDatabaseURL returns the PostgreSQL connection string for integration tests.
func getDatabaseURL() string {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://postgres:postgres@localhost:5433/financial_tracker?sslmode=disable"
	}
	return url
}

// TestCleaner tracks database records created by integration tests and automatically deletes
// them when the test finishes successfully (!t.Failed()). If the test fails, records are
// preserved for developer inspection and debugging.
type TestCleaner struct {
	t                   *testing.T
	pool                *pgxpool.Pool
	mu                  sync.Mutex
	userIDs             map[string]struct{}
	accountIDs          map[string]struct{}
	emails              map[string]struct{}
	messageIDs          map[string]struct{}
	financialEventIDs   map[string]struct{}
	extractionResultIDs map[string]struct{}
	templatePatterns    map[string]struct{}
	cleanups            []func(ctx context.Context) error
}

// NewTestCleaner initializes a test database cleaner and registers a cleanup hook on t.
func NewTestCleaner(t *testing.T, pool *pgxpool.Pool) *TestCleaner {
	c := &TestCleaner{
		t:                   t,
		pool:                pool,
		userIDs:             make(map[string]struct{}),
		accountIDs:          make(map[string]struct{}),
		emails:              make(map[string]struct{}),
		messageIDs:          make(map[string]struct{}),
		financialEventIDs:   make(map[string]struct{}),
		extractionResultIDs: make(map[string]struct{}),
		templatePatterns:    make(map[string]struct{}),
	}

	t.Cleanup(func() {
		if t.Failed() {
			t.Log("[TestCleaner] Test failed: preserving test database records for debugging.")
			return
		}
		c.CleanupNow()
	})

	return c
}

// TrackUser registers a user ID to be cleaned up.
func (c *TestCleaner) TrackUser(userID string) {
	if userID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userIDs[userID] = struct{}{}
}

// TrackGmailAccount registers a Gmail account ID to be cleaned up.
func (c *TestCleaner) TrackGmailAccount(accountID string) {
	if accountID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accountIDs[accountID] = struct{}{}
}

// TrackEmail registers an email address to be cleaned up.
func (c *TestCleaner) TrackEmail(email string) {
	if email == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.emails[email] = struct{}{}
}

// TrackMessage registers an ingested message ID to be cleaned up.
func (c *TestCleaner) TrackMessage(msgID string) {
	if msgID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messageIDs[msgID] = struct{}{}
}

// TrackFinancialEvent registers a financial event ID to be cleaned up.
func (c *TestCleaner) TrackFinancialEvent(eventID string) {
	if eventID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.financialEventIDs[eventID] = struct{}{}
}

// TrackExtractionResult registers an understanding extraction result ID to be cleaned up.
func (c *TestCleaner) TrackExtractionResult(resultID string) {
	if resultID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.extractionResultIDs[resultID] = struct{}{}
}

// TrackTemplatePattern registers a sender pattern to be cleaned up.
func (c *TestCleaner) TrackTemplatePattern(pattern string) {
	if pattern == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.templatePatterns[pattern] = struct{}{}
}

// AddCleanup registers a custom cleanup function.
func (c *TestCleaner) AddCleanup(fn func(ctx context.Context) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanups = append(c.cleanups, fn)
}

// setupTestDB connects to the test database and returns a pool along with a TestCleaner.
// It automatically closes the pool AFTER all test cleanup has completed.
func setupTestDB(t *testing.T) (*pgxpool.Pool, *TestCleaner) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping integration test, postgres ping failed: %v", err)
	}

	cleaner := &TestCleaner{
		t:                   t,
		pool:                pool,
		userIDs:             make(map[string]struct{}),
		accountIDs:          make(map[string]struct{}),
		emails:              make(map[string]struct{}),
		messageIDs:          make(map[string]struct{}),
		financialEventIDs:   make(map[string]struct{}),
		extractionResultIDs: make(map[string]struct{}),
		templatePatterns:    make(map[string]struct{}),
	}

	t.Cleanup(func() {
		if !t.Failed() {
			cleaner.CleanupNow()
		} else {
			t.Log("[TestCleaner] Test failed: preserving test database records for debugging.")
		}
		pool.Close()
	})

	return pool, cleaner
}

// CleanupNow immediately executes deletion of all tracked test records.
func (c *TestCleaner) CleanupNow() {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Ensure we have an active database connection even if caller did 'defer pool.Close()'
	pool := c.pool
	if err := pool.Ping(ctx); err != nil {
		freshPool, freshErr := pgxpool.New(ctx, getDatabaseURL())
		if freshErr == nil {
			pool = freshPool
			defer freshPool.Close()
		}
	}

	// 1. Custom cleanups
	for _, fn := range c.cleanups {
		_ = fn(ctx)
	}

	// 2. Financial Events
	for eventID := range c.financialEventIDs {
		_, _ = pool.Exec(ctx, "DELETE FROM financialcore.category_assignments WHERE financial_event_id = $1::uuid", eventID)
		_, _ = pool.Exec(ctx, "DELETE FROM financialcore.event_overrides WHERE financial_event_id = $1::uuid", eventID)
		_, _ = pool.Exec(ctx, "DELETE FROM financialcore.financial_events WHERE id = $1::uuid", eventID)
		_, _ = pool.Exec(ctx, "DELETE FROM financialcore.outbox_events WHERE aggregate_id = $1::uuid", eventID)
	}

	// 3. Extraction Results
	for resultID := range c.extractionResultIDs {
		_, _ = pool.Exec(ctx, "DELETE FROM understanding.outbox_events WHERE aggregate_id = $1::uuid", resultID)
		_, _ = pool.Exec(ctx, "DELETE FROM understanding.extraction_results WHERE id = $1::uuid", resultID)
	}

	// 4. Messages
	for msgID := range c.messageIDs {
		_, _ = pool.Exec(ctx, "DELETE FROM understanding.outbox_events WHERE aggregate_id IN (SELECT id FROM understanding.extraction_results WHERE ingested_message_id = $1::uuid)", msgID)
		_, _ = pool.Exec(ctx, "DELETE FROM understanding.extraction_results WHERE ingested_message_id = $1::uuid", msgID)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.outbox_events WHERE aggregate_id = $1::uuid", msgID)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.ingested_messages WHERE id = $1::uuid", msgID)
	}

	// 5. Template Patterns
	for pattern := range c.templatePatterns {
		_, _ = pool.Exec(ctx, "DELETE FROM understanding.sender_templates WHERE sender_pattern = $1", pattern)
		_, _ = pool.Exec(ctx, "DELETE FROM administration.parser_metrics WHERE template_id = $1", pattern)
	}

	// 6. Gmail Accounts
	for accountID := range c.accountIDs {
		_, _ = pool.Exec(ctx, "DELETE FROM understanding.outbox_events WHERE aggregate_id IN (SELECT id FROM understanding.extraction_results WHERE ingested_message_id IN (SELECT id FROM ingestion.ingested_messages WHERE gmail_account_id = $1::uuid))", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM understanding.extraction_results WHERE ingested_message_id IN (SELECT id FROM ingestion.ingested_messages WHERE gmail_account_id = $1::uuid)", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.ingested_messages WHERE gmail_account_id = $1::uuid", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.sender_watch_log WHERE gmail_account_id = $1::uuid", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM identity.oauth_tokens WHERE gmail_account_id = $1::uuid", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.outbox_events WHERE aggregate_id = $1::uuid", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.gmail_accounts WHERE id = $1::uuid", accountID)
	}

	// 7. Emails
	for email := range c.emails {
		_, _ = pool.Exec(ctx, "DELETE FROM understanding.outbox_events WHERE aggregate_id IN (SELECT id FROM understanding.extraction_results WHERE ingested_message_id IN (SELECT id FROM ingestion.ingested_messages WHERE gmail_account_id IN (SELECT id FROM ingestion.gmail_accounts WHERE email_address = $1)))", email)
		_, _ = pool.Exec(ctx, "DELETE FROM understanding.extraction_results WHERE ingested_message_id IN (SELECT id FROM ingestion.ingested_messages WHERE gmail_account_id IN (SELECT id FROM ingestion.gmail_accounts WHERE email_address = $1))", email)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.ingested_messages WHERE gmail_account_id IN (SELECT id FROM ingestion.gmail_accounts WHERE email_address = $1)", email)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.sender_watch_log WHERE gmail_account_id IN (SELECT id FROM ingestion.gmail_accounts WHERE email_address = $1)", email)
		_, _ = pool.Exec(ctx, "DELETE FROM identity.oauth_tokens WHERE gmail_account_id IN (SELECT id FROM ingestion.gmail_accounts WHERE email_address = $1)", email)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.outbox_events WHERE aggregate_id IN (SELECT id FROM ingestion.gmail_accounts WHERE email_address = $1)", email)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.gmail_accounts WHERE email_address = $1", email)
		_, _ = pool.Exec(ctx, "DELETE FROM identity.refresh_tokens WHERE user_id IN (SELECT id FROM identity.users WHERE email = $1)", email)
		_, _ = pool.Exec(ctx, "DELETE FROM identity.outbox_events WHERE aggregate_id IN (SELECT id FROM identity.users WHERE email = $1)", email)
		_, _ = pool.Exec(ctx, "DELETE FROM identity.users WHERE email = $1", email)
	}

	// 8. Users
	for userID := range c.userIDs {
		_, _ = pool.Exec(ctx, "DELETE FROM engagement.notification_jobs WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM engagement.notification_preferences WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM engagement.outbox_events WHERE aggregate_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM analytics.cash_flow_view WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM analytics.category_breakdown_view WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM analytics.merchant_ranking_view WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM analytics.trend_view WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM analytics.projected_events WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM budget.budget_fired_alerts WHERE budget_id IN (SELECT id FROM budget.budgets WHERE user_id = $1::uuid)", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM budget.outbox_events WHERE aggregate_id IN (SELECT id FROM budget.budgets WHERE user_id = $1::uuid) OR aggregate_id IN (SELECT id FROM budget.savings_goals WHERE user_id = $1::uuid)", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM budget.budgets WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM budget.savings_goals WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM financialcore.category_assignments WHERE financial_event_id IN (SELECT id FROM financialcore.financial_events WHERE user_id = $1::uuid)", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM financialcore.event_overrides WHERE financial_event_id IN (SELECT id FROM financialcore.financial_events WHERE user_id = $1::uuid)", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM financialcore.outbox_events WHERE aggregate_id IN (SELECT id FROM financialcore.financial_events WHERE user_id = $1::uuid)", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM financialcore.financial_events WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM understanding.outbox_events WHERE aggregate_id IN (SELECT id FROM understanding.extraction_results WHERE ingested_message_id IN (SELECT id FROM ingestion.ingested_messages WHERE gmail_account_id IN (SELECT id FROM ingestion.gmail_accounts WHERE user_id = $1::uuid)))", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM understanding.extraction_results WHERE ingested_message_id IN (SELECT id FROM ingestion.ingested_messages WHERE gmail_account_id IN (SELECT id FROM ingestion.gmail_accounts WHERE user_id = $1::uuid))", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.ingested_messages WHERE gmail_account_id IN (SELECT id FROM ingestion.gmail_accounts WHERE user_id = $1::uuid)", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.sender_watch_log WHERE gmail_account_id IN (SELECT id FROM ingestion.gmail_accounts WHERE user_id = $1::uuid)", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM identity.oauth_tokens WHERE gmail_account_id IN (SELECT id FROM ingestion.gmail_accounts WHERE user_id = $1::uuid)", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.outbox_events WHERE aggregate_id IN (SELECT id FROM ingestion.gmail_accounts WHERE user_id = $1::uuid)", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM ingestion.gmail_accounts WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM identity.refresh_tokens WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM identity.outbox_events WHERE aggregate_id = $1::uuid", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM identity.users WHERE id = $1::uuid", userID)
	}

	// 9. Safety sweep: purge any test events or extractions that were orphaned
	_, _ = pool.Exec(ctx, `
		DELETE FROM analytics.cash_flow_view WHERE user_id NOT IN (SELECT id FROM identity.users);
		DELETE FROM analytics.category_breakdown_view WHERE user_id NOT IN (SELECT id FROM identity.users);
		DELETE FROM analytics.merchant_ranking_view WHERE user_id NOT IN (SELECT id FROM identity.users);
		DELETE FROM analytics.trend_view WHERE user_id NOT IN (SELECT id FROM identity.users);
		DELETE FROM analytics.projected_events WHERE user_id NOT IN (SELECT id FROM identity.users);
		DELETE FROM financialcore.category_assignments WHERE financial_event_id IN (
			SELECT id FROM financialcore.financial_events WHERE user_id NOT IN (SELECT id FROM identity.users)
		);
		DELETE FROM financialcore.event_overrides WHERE financial_event_id IN (
			SELECT id FROM financialcore.financial_events WHERE user_id NOT IN (SELECT id FROM identity.users)
		);
		DELETE FROM financialcore.financial_events WHERE user_id NOT IN (SELECT id FROM identity.users);
		DELETE FROM understanding.outbox_events WHERE aggregate_id IN (
			SELECT id FROM understanding.extraction_results WHERE ingested_message_id NOT IN (SELECT id FROM ingestion.ingested_messages)
		);
		DELETE FROM understanding.extraction_results WHERE ingested_message_id NOT IN (SELECT id FROM ingestion.ingested_messages);
	`)
}
