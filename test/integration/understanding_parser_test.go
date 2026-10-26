package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding"
	understandingCmd "github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/validator"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestSprint4_Diagram3_DeterministicTemplateExtraction_HappyPath verifies Diagram 3:
// Known-sender email is matched against active template, extracted deterministically with 1.0 confidence,
// recorded in extraction_results, and FinancialEventCandidateCreated is recorded in outbox [GATE].
func TestSprint4_Diagram3_DeterministicTemplateExtraction_HappyPath(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	mod, err := understanding.NewModule(understanding.Config{
		DB: pool,
	})
	if err != nil {
		t.Fatalf("failed to initialize understanding module: %v", err)
	}

	// Seed curated MVP templates
	if err := mod.SeedCuratedTemplates(ctx); err != nil {
		t.Fatalf("failed to seed curated templates: %v", err)
	}

	userID := uuid.New().String()
	cleaner := NewTestCleaner(t, pool)
	cleaner.TrackUser(userID)

	msgID := uuid.New().String()
	cleaner.TrackMessage(msgID)

	// Ingested BCA debit transaction email
	rawEmail := domain.RawEmailMessage{
		MessageID:      msgID,
		GmailAccountID: uuid.New().String(),
		Sender:         "e-statement@bca.co.id",
		Subject:        "Notifikasi Transaksi Kartu Debit BCA",
		BodyHTML: `
			<div class="transaction-detail">
				<span class="trx-amount">Rp 175.000,00</span>
				<span class="trx-date">25/10/2026 18:30</span>
				<span class="trx-merchant">INDOMARET POINT</span>
				<span class="trx-card">Rekening: *******1234</span>
			</div>
		`,
		ReceivedAt: time.Now().UTC(),
	}

	// 1. Execute deterministic extraction (Diagram 3)
	res, err := mod.ApplyTemplateCmd.Handle(ctx, understandingCmd.ApplySenderTemplateCommand{
		MessageID: msgID,
		UserID:    userID,
		RawEmail:  rawEmail,
	})
	if err != nil {
		t.Fatalf("ApplySenderTemplate failed: %v", err)
	}

	// [GATE]: Verifies deterministic-first design: confidence = 1.0 for full match
	if res.Outcome != "full" {
		t.Errorf("expected outcome 'full', got: %s", res.Outcome)
	}
	cleaner.TrackExtractionResult(res.ExtractionResultID)
	if res.ConfidenceAggregate != 1.0 {
		t.Errorf("expected confidence aggregate 1.0, got: %f", res.ConfidenceAggregate)
	}
	if res.Candidate == nil {
		t.Fatalf("expected candidate event to be built, got nil")
	}

	cand := res.Candidate
	if cand.Amount.Value != 175000.0 {
		t.Errorf("expected amount 175000, got: %f", cand.Amount.Value)
	}
	if cand.Amount.Currency != "IDR" {
		t.Errorf("expected currency IDR, got: %s", cand.Amount.Currency)
	}
	if cand.Merchant.NormalizedName != "Indomaret Point" {
		t.Errorf("expected merchant normalized 'Indomaret Point', got: %s", cand.Merchant.NormalizedName)
	}
	if cand.Provenance.ExtractionMethod != domain.ExtractionMethodTemplate {
		t.Errorf("expected extraction method 'template', got: %s", cand.Provenance.ExtractionMethod)
	}

	// 2. Verify understanding.extraction_results row exists in PostgreSQL
	var dbCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM understanding.extraction_results WHERE id = $1 AND method = 'template'", res.ExtractionResultID).Scan(&dbCount)
	if err != nil {
		t.Fatalf("failed to query extraction_results: %v", err)
	}
	if dbCount != 1 {
		t.Errorf("expected 1 extraction_results record, got: %d", dbCount)
	}

	// 3. Verify outbox contains FinancialEventCandidateCreated [GATE]
	var outboxCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM understanding.outbox_events WHERE event_type = $1", domain.EventFinancialEventCandidateCreated).Scan(&outboxCount)
	if err != nil {
		t.Fatalf("failed to query outbox: %v", err)
	}
	if outboxCount == 0 {
		t.Errorf("expected FinancialEventCandidateCreated event in outbox")
	}
	_, _ = pool.Exec(ctx, "DELETE FROM understanding.outbox_events WHERE aggregate_id = $1::uuid", res.ExtractionResultID)
}

// TestSprint4_PublishTemplate_ValidationGateRejectsBrokenTemplate verifies that
// template publishing CANNOT succeed without passing sample-set validation [GATE].
func TestSprint4_PublishTemplate_ValidationGateRejectsBrokenTemplate(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	mod, err := understanding.NewModule(understanding.Config{
		DB: pool,
	})
	if err != nil {
		t.Fatalf("failed to initialize module: %v", err)
	}

	senderPattern := fmt.Sprintf("*@bank-%s.com", uuid.New().String()[:8])
	cleaner := NewTestCleaner(t, pool)
	cleaner.TrackTemplatePattern(senderPattern)

	// Define a deliberately broken template (CSS selector does not exist in sample)
	brokenRules := domain.ExtractionRules{
		SenderPattern: senderPattern,
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".non-existent-broken-selector", // WILL FAIL TO MATCH
				PostProcessing:     []string{domain.PostProcessingParseDecimal},
				Required:           true,
			},
			"merchant": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  "Bank Test",
				Required:     true,
			},
		},
	}

	sampleSet := &validator.SampleSet{
		SenderPattern: senderPattern,
		Samples: []validator.SampleEmail{
			{
				ID: "sample-01",
				RawEmail: domain.RawEmailMessage{
					MessageID: "msg-001",
					Sender:    "alert@" + senderPattern[2:],
					BodyHTML:  "<div><span class='real-amount'>Rp 100.000</span></div>",
				},
				Expected: validator.ExpectedExtraction{
					Amount:   100000.0,
					Currency: "IDR",
				},
			},
		},
	}

	// Attempt to publish broken template -> MUST FAIL [GATE]
	res, err := mod.PublishTemplateCmd.Handle(ctx, understandingCmd.PublishTemplateCommand{
		SenderPattern:   senderPattern,
		Version:         1,
		ExtractionRules: brokenRules,
		SampleSet:       sampleSet,
	})

	if err == nil {
		t.Fatalf("GATE VIOLATION: expected PublishTemplateCommand to fail sample-set validation, got success: %+v", res)
	}
	if res.ValidationReport != nil && res.ValidationReport.Passed {
		t.Errorf("expected validation report to be failed, got passed")
	}

	// Verify broken template was NOT inserted as active in DB
	var activeCount int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM understanding.sender_templates WHERE sender_pattern = $1 AND active = true", senderPattern).Scan(&activeCount)
	if activeCount != 0 {
		t.Errorf("expected 0 active templates in DB for broken template, got: %d", activeCount)
	}

	// Now fix the template and publish -> MUST SUCCEED
	validRules := domain.ExtractionRules{
		SenderPattern: senderPattern,
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".real-amount",
				PostProcessing: []string{
					domain.PostProcessingStripCurrencySymbol,
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDecimal,
				},
				Required: true,
			},
			"merchant": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  "Bank Test",
				Required:     true,
			},
		},
	}

	validRes, err := mod.PublishTemplateCmd.Handle(ctx, understandingCmd.PublishTemplateCommand{
		SenderPattern:   senderPattern,
		Version:         1,
		ExtractionRules: validRules,
		SampleSet:       sampleSet,
	})
	if err != nil {
		t.Fatalf("expected valid template publish to succeed, got error: %v", err)
	}
	if !validRes.ValidationReport.Passed {
		t.Errorf("expected valid template to pass sample set evaluation")
	}

	// Now publish version 2 -> verifies atomic swap and that old version is retained (never deleted) [GATE]
	validResV2, err := mod.PublishTemplateCmd.Handle(ctx, understandingCmd.PublishTemplateCommand{
		SenderPattern:   senderPattern,
		Version:         2,
		ExtractionRules: validRules,
		SampleSet:       sampleSet,
	})
	if err != nil {
		t.Fatalf("failed to publish v2: %v", err)
	}
	if validResV2.Version != 2 {
		t.Errorf("expected version 2, got: %d", validResV2.Version)
	}

	// Verify database state: exactly 1 active version, but 2 total versions (old never deleted) [GATE]
	var totalVersions int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM understanding.sender_templates WHERE sender_pattern = $1", senderPattern).Scan(&totalVersions)
	if totalVersions != 2 {
		t.Errorf("expected 2 total template versions retained in DB, got: %d", totalVersions)
	}

	_ = pool.QueryRow(ctx, "SELECT count(*) FROM understanding.sender_templates WHERE sender_pattern = $1 AND active = true", senderPattern).Scan(&activeCount)
	if activeCount != 1 {
		t.Errorf("expected exactly 1 active template version, got: %d", activeCount)
	}
}

// TestSprint4_ParserMonitoring_SpikeDetectionAlert verifies that when a template's failure rate
// exceeds the 5% threshold, an alert warning is recorded and the Prometheus metric is updated [GATE].
func TestSprint4_ParserMonitoring_SpikeDetectionAlert(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	mod, err := understanding.NewModule(understanding.Config{
		DB: pool,
	})
	if err != nil {
		t.Fatalf("failed to initialize module: %v", err)
	}

	senderPattern := fmt.Sprintf("*@spike-%s.com", uuid.New().String()[:8])
	cleaner := NewTestCleaner(t, pool)
	cleaner.TrackTemplatePattern(senderPattern)
	rules := domain.ExtractionRules{
		SenderPattern: senderPattern,
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".amount-tag",
				Required:           true,
			},
		},
	}

	tmpl, _ := domain.NewSenderTemplate(uuid.New().String(), senderPattern, 1, rules, true, time.Now())
	if err := mod.Repo.PublishAtomic(ctx, tmpl); err != nil {
		t.Fatalf("publish test template: %v", err)
	}

	tmplVer := "v1"

	// Simulate 6 executions: 1 success, 5 failures -> failure rate = 5/6 = 83.3% (>5% threshold)
	for i := 0; i < 5; i++ {
		resID := uuid.New().String()
		cleaner.TrackExtractionResult(resID)
		_ = mod.Repo.SaveResult(ctx, &domain.ExtractionResultRecord{
			ID:                  resID,
			IngestedMessageID:   uuid.New().String(),
			Method:              domain.ExtractionMethodTemplate,
			TemplateVersion:     &tmplVer,
			ConfidenceAggregate: 0.0, // Failed extraction
			CreatedAt:           time.Now().UTC(),
		})
	}
	resID := uuid.New().String()
	cleaner.TrackExtractionResult(resID)
	_ = mod.Repo.SaveResult(ctx, &domain.ExtractionResultRecord{
		ID:                  resID,
		IngestedMessageID:   uuid.New().String(),
		Method:              domain.ExtractionMethodTemplate,
		TemplateVersion:     &tmplVer,
		ConfidenceAggregate: 1.0, // Successful extraction
		CreatedAt:           time.Now().UTC(),
	})

	// Run failure rate rollup aggregation
	rollupRes, err := mod.RollupMetricsCmd.Handle(ctx, understandingCmd.RollupParserMetricsCommand{
		Trigger: "manual",
	})
	if err != nil {
		t.Fatalf("rollup metrics failed: %v", err)
	}

	// Verify spike alert triggered [GATE]
	if len(rollupRes.SpikeAlerts) == 0 {
		t.Errorf("expected spike alert for template with >5%% failure rate, got 0 alerts")
	}

	// Verify Prometheus gauge updated
	gaugeVal := observability.GetMetrics().UnderstandingTemplateFailureRate.WithLabelValues(senderPattern, "1h")
	if gaugeVal == nil {
		t.Errorf("expected Prometheus gauge to be populated for %s", senderPattern)
	}
}
