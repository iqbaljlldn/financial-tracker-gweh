package executor_test

import (
	"testing"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/executor"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

func TestExtractionExecutor_FullMatch(t *testing.T) {
	rules := domain.ExtractionRules{
		SenderPattern: "*@bca.co.id",
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: "td.amount",
				PostProcessing: []string{
					domain.PostProcessingStripCurrencySymbol,
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDecimal,
				},
				Required: true,
			},
			"occurredAt": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: "td.date",
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDate + ":DD/MM/YYYY HH:mm",
				},
				Required: true,
			},
			"merchant": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `Merchant:\s*([^\n<]+)`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingTitleCase,
				},
				Required: true,
			},
			"paymentMethod": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `Card:\s*(\d{4})`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
				},
				Required: false,
			},
			"eventTypeHint": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  domain.EventTypeExpense,
				Required:     false,
			},
		},
	}

	tmpl, err := domain.NewSenderTemplate("tmpl-bca", "*@bca.co.id", 1, rules, true, time.Now())
	if err != nil {
		t.Fatalf("failed to create template: %v", err)
	}

	msg := &domain.RawEmailMessage{
		MessageID:      "msg-123",
		GmailAccountID: "acc-456",
		Sender:         "notifikasi@bca.co.id",
		Subject:        "Transaksi Debit BCA",
		BodyHTML: `
			<html>
				<body>
					<table>
						<tr><td class="amount">Rp 250.000,00</td></tr>
						<tr><td class="date">25/10/2026 14:30</td></tr>
					</table>
					<p>Merchant: kopi kenangan mantan</p>
					<p>Card: 4321</p>
				</body>
			</html>
		`,
		BodyText:   "Merchant: kopi kenangan mantan\nCard: 4321",
		ReceivedAt: time.Now(),
	}

	exec := executor.NewExtractionExecutor()
	res := exec.Execute(tmpl, msg, "user-789")

	if res.Outcome != executor.OutcomeFullMatch {
		t.Fatalf("expected OutcomeFullMatch, got: %s (err: %v)", res.Outcome, res.Err)
	}
	if res.ConfidenceAggregate != 1.0 {
		t.Errorf("expected aggregate confidence 1.0, got: %f", res.ConfidenceAggregate)
	}
	if res.Candidate == nil {
		t.Fatalf("expected candidate event to be built, got nil")
	}

	c := res.Candidate
	if c.Amount.Value != 250000.0 {
		t.Errorf("expected amount 250000, got: %f", c.Amount.Value)
	}
	if c.Amount.Currency != "IDR" {
		t.Errorf("expected currency IDR, got: %s", c.Amount.Currency)
	}
	if c.Merchant.NormalizedName != "Kopi Kenangan Mantan" {
		t.Errorf("expected merchant normalized 'Kopi Kenangan Mantan', got: %s", c.Merchant.NormalizedName)
	}
	if c.PaymentMethod.Detail != "4321" {
		t.Errorf("expected payment method detail 4321, got: %s", c.PaymentMethod.Detail)
	}
	if c.EventType != domain.EventTypeExpense {
		t.Errorf("expected event type Expense, got: %s", c.EventType)
	}
	expectedDate, _ := time.Parse(time.RFC3339, "2026-10-25T14:30:00Z")
	if !c.OccurredAt.Equal(expectedDate) {
		t.Errorf("expected occurredAt %v, got: %v", expectedDate, c.OccurredAt)
	}
}

func TestExtractionExecutor_PartialMatch(t *testing.T) {
	rules := domain.ExtractionRules{
		SenderPattern: "*@gojek.com",
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `Total:\s*Rp\s*([0-9.]+)`,
				ExtractionGroup:    1,
				PostProcessing:     []string{domain.PostProcessingParseDecimal},
				Required:           true,
			},
			"occurredAt": {
				SelectorType:       domain.SelectorHeader,
				SelectorExpression: "Date",
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDate,
				},
				Required: true,
			},
			"merchant": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  "GoJek",
				Required:     true,
			},
			"paymentMethod": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `Metode Pembayaran:\s*([^\n]+)`,
				ExtractionGroup:    1,
				Required:           false, // Optional field that will fail to match
			},
		},
	}

	tmpl, _ := domain.NewSenderTemplate("tmpl-gojek", "*@gojek.com", 1, rules, true, time.Now())

	msg := &domain.RawEmailMessage{
		MessageID:      "msg-gojek-1",
		GmailAccountID: "acc-1",
		Sender:         "no-reply@gojek.com",
		Headers: map[string]string{
			"Date": "2026-10-25T12:00:00Z",
		},
		BodyText: "Bukti Transaksi GoRide\nTotal: Rp 35.000\nTerima kasih!",
	}

	exec := executor.NewExtractionExecutor()
	res := exec.Execute(tmpl, msg, "user-123")

	if res.Outcome != executor.OutcomePartialMatch {
		t.Fatalf("expected OutcomePartialMatch, got: %s (err: %v)", res.Outcome, res.Err)
	}
	if res.ConfidenceAggregate != 0.95 {
		t.Errorf("expected aggregate confidence 0.95 for partial match, got: %f", res.ConfidenceAggregate)
	}
	if res.Candidate == nil {
		t.Fatalf("expected candidate event to be built, got nil")
	}
	if res.Candidate.Amount.Value != 35000.0 {
		t.Errorf("expected amount 35000, got: %f", res.Candidate.Amount.Value)
	}
}

func TestExtractionExecutor_RequiredFieldFailure(t *testing.T) {
	rules := domain.ExtractionRules{
		SenderPattern: "*@grab.com",
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".non-existent-amount-class", // Will fail to match
				Required:           true,
			},
			"merchant": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  "Grab",
				Required:     true,
			},
		},
	}

	tmpl, _ := domain.NewSenderTemplate("tmpl-grab", "*@grab.com", 1, rules, true, time.Now())

	msg := &domain.RawEmailMessage{
		MessageID: "msg-grab-fail",
		BodyHTML:  "<html><body><p>Your Grab receipt</p></body></html>",
	}

	exec := executor.NewExtractionExecutor()
	res := exec.Execute(tmpl, msg, "user-123")

	if res.Outcome != executor.OutcomeFailed {
		t.Fatalf("expected OutcomeFailed on required field missing, got: %s", res.Outcome)
	}
	if res.ConfidenceAggregate != 0.0 {
		t.Errorf("expected aggregate confidence 0.0 on failure, got: %f", res.ConfidenceAggregate)
	}
	if len(res.MissingFields) != 1 || res.MissingFields[0] != "amount" {
		t.Errorf("expected missing fields ['amount'], got: %v", res.MissingFields)
	}
	if res.Candidate != nil {
		t.Errorf("expected no candidate on required failure, got: %v", res.Candidate)
	}
}
