package fixtures

import (
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/validator"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// GetCuratedTemplates returns the initial MVP library of known sender templates per PRD §6.1.
func GetCuratedTemplates() []*domain.SenderTemplate {
	// 1. Bank Central Asia (BCA)
	bcaRules := domain.ExtractionRules{
		SenderPattern: "*@bca.co.id",
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".trx-amount",
				PostProcessing: []string{
					domain.PostProcessingStripCurrencySymbol,
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDecimal,
				},
				Required: true,
			},
			"occurredAt": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".trx-date",
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDate + ":DD/MM/YYYY HH:mm",
				},
				Required: true,
			},
			"merchant": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".trx-merchant",
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingTitleCase,
				},
				Required: true,
			},
			"paymentMethod": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".trx-card",
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
				},
				Required: false,
			},
			"currency": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  "IDR",
				Required:     false,
			},
			"eventTypeHint": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  domain.EventTypeExpense,
				Required:     false,
			},
		},
	}
	tmplBCA, _ := domain.NewSenderTemplate("tmpl-bca-001", "*@bca.co.id", 1, bcaRules, true, time.Now())

	// 2. Bank Mandiri
	mandiriRules := domain.ExtractionRules{
		SenderPattern: "*@bankmandiri.co.id",
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `Nominal:\s*(?:IDR\s*)?([0-9.,]+)`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDecimal,
				},
				Required: true,
			},
			"occurredAt": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `Tanggal:\s*([0-9-]+\s+[0-9:]+)`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDate + ":DD-MM-YYYY HH:mm:ss",
				},
				Required: true,
			},
			"merchant": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `Transaksi berhasil di\s*([^\n\r]+)`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingTitleCase,
				},
				Required: true,
			},
			"paymentMethod": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `Kartu:\s*([^\n\r]+)`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
				},
				Required: false,
			},
			"currency": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  "IDR",
				Required:     false,
			},
			"eventTypeHint": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  domain.EventTypeExpense,
				Required:     false,
			},
		},
	}
	tmplMandiri, _ := domain.NewSenderTemplate("tmpl-mandiri-001", "*@bankmandiri.co.id", 1, mandiriRules, true, time.Now())

	// 3. GoPay / Gojek
	gojekRules := domain.ExtractionRules{
		SenderPattern: "*@gojek.com",
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".payment-amount",
				PostProcessing: []string{
					domain.PostProcessingStripCurrencySymbol,
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDecimal,
				},
				Required: true,
			},
			"occurredAt": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".trx-time",
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDate + ":DD MMM YYYY HH:mm",
				},
				Required: true,
			},
			"merchant": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  "GoJek",
				Required:     true,
			},
			"paymentMethod": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".payment-method",
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
				},
				Required: false,
			},
			"categoryHint": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  "Transportation",
				Required:     false,
			},
			"currency": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  "IDR",
				Required:     false,
			},
			"eventTypeHint": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  domain.EventTypeExpense,
				Required:     false,
			},
		},
	}
	tmplGojek, _ := domain.NewSenderTemplate("tmpl-gojek-001", "*@gojek.com", 1, gojekRules, true, time.Now())

	// 4. OVO
	ovoRules := domain.ExtractionRules{
		SenderPattern: "*@ovo.id",
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".total-amount",
				PostProcessing: []string{
					domain.PostProcessingStripCurrencySymbol,
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDecimal,
				},
				Required: true,
			},
			"occurredAt": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".date-time",
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDate + ":YYYY-MM-DD HH:mm:ss",
				},
				Required: true,
			},
			"merchant": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".merchant-name",
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingTitleCase,
				},
				Required: true,
			},
			"paymentMethod": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".payment-source",
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
				},
				Required: false,
			},
			"currency": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  "IDR",
				Required:     false,
			},
			"eventTypeHint": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  domain.EventTypeExpense,
				Required:     false,
			},
		},
	}
	tmplOVO, _ := domain.NewSenderTemplate("tmpl-ovo-001", "*@ovo.id", 1, ovoRules, true, time.Now())

	// 5. Grab
	grabRules := domain.ExtractionRules{
		SenderPattern: "*@grab.com",
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".fare-total",
				PostProcessing: []string{
					domain.PostProcessingStripCurrencySymbol,
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDecimal,
				},
				Required: true,
			},
			"occurredAt": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".timestamp",
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDate + ":DD/MM/YYYY HH:mm",
				},
				Required: true,
			},
			"merchant": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".merchant-title",
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingTitleCase,
				},
				Required: true,
			},
			"paymentMethod": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".paid-by",
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
				},
				Required: false,
			},
			"categoryHint": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  "Food & Beverage",
				Required:     false,
			},
			"currency": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  "IDR",
				Required:     false,
			},
			"eventTypeHint": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  domain.EventTypeExpense,
				Required:     false,
			},
		},
	}
	tmplGrab, _ := domain.NewSenderTemplate("tmpl-grab-001", "*@grab.com", 1, grabRules, true, time.Now())

	return []*domain.SenderTemplate{
		tmplBCA,
		tmplMandiri,
		tmplGojek,
		tmplOVO,
		tmplGrab,
	}
}

// GetSampleSets returns real anonymized sample email sets with expected exact-match extractions.
func GetSampleSets() map[string]*validator.SampleSet {
	sets := make(map[string]*validator.SampleSet)

	// BCA Samples
	sets["*@bca.co.id"] = &validator.SampleSet{
		SenderPattern: "*@bca.co.id",
		Samples: []validator.SampleEmail{
			{
				ID: "bca-sample-01",
				RawEmail: domain.RawEmailMessage{
					MessageID:      "msg-bca-001",
					GmailAccountID: "acc-test",
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
					ReceivedAt: time.Now(),
				},
				Expected: validator.ExpectedExtraction{
					EventType:  domain.EventTypeExpense,
					Amount:     175000.0,
					Currency:   "IDR",
					Merchant:   "Indomaret Point",
					OccurredAt: "2026-10-25T18:30:00Z",
				},
			},
		},
	}

	// Mandiri Samples
	sets["*@bankmandiri.co.id"] = &validator.SampleSet{
		SenderPattern: "*@bankmandiri.co.id",
		Samples: []validator.SampleEmail{
			{
				ID: "mandiri-sample-01",
				RawEmail: domain.RawEmailMessage{
					MessageID:      "msg-mandiri-001",
					GmailAccountID: "acc-test",
					Sender:         "mandiricare@bankmandiri.co.id",
					Subject:        "Notifikasi Transaksi Mandiri Debit",
					BodyText: `
Mandiri Debit Alert:
Transaksi berhasil di STARBUCKS COFFEE
Nominal: IDR 68.000
Tanggal: 25-10-2026 09:15:00
Kartu: Mandiri Visa ...5678
					`,
					ReceivedAt: time.Now(),
				},
				Expected: validator.ExpectedExtraction{
					EventType:  domain.EventTypeExpense,
					Amount:     68000.0,
					Currency:   "IDR",
					Merchant:   "Starbucks Coffee",
					OccurredAt: "2026-10-25T09:15:00Z",
				},
			},
		},
	}

	// Gojek Samples
	sets["*@gojek.com"] = &validator.SampleSet{
		SenderPattern: "*@gojek.com",
		Samples: []validator.SampleEmail{
			{
				ID: "gojek-sample-01",
				RawEmail: domain.RawEmailMessage{
					MessageID:      "msg-gojek-001",
					GmailAccountID: "acc-test",
					Sender:         "no-reply@gojek.com",
					Subject:        "Bukti Transaksi GoRide",
					BodyHTML: `
						<div class="gojek-receipt">
							<h1 class="service-name">GoRide</h1>
							<div class="payment-amount">Rp 24.000</div>
							<div class="trx-time">25 Okt 2026 11:20</div>
							<div class="payment-method">GoPay</div>
						</div>
					`,
					ReceivedAt: time.Now(),
				},
				Expected: validator.ExpectedExtraction{
					EventType:  domain.EventTypeExpense,
					Amount:     24000.0,
					Currency:   "IDR",
					Merchant:   "Gojek",
					OccurredAt: "2026-10-25T11:20:00Z",
				},
			},
		},
	}

	// OVO Samples
	sets["*@ovo.id"] = &validator.SampleSet{
		SenderPattern: "*@ovo.id",
		Samples: []validator.SampleEmail{
			{
				ID: "ovo-sample-01",
				RawEmail: domain.RawEmailMessage{
					MessageID:      "msg-ovo-001",
					GmailAccountID: "acc-test",
					Sender:         "no-reply@ovo.id",
					Subject:        "Struk Transaksi OVO",
					BodyHTML: `
						<div class="ovo-receipt">
							<span class="merchant-name">KOPI KENANGAN MANTAN</span>
							<span class="total-amount">Rp 42.000</span>
							<span class="date-time">2026-10-25 15:40:00</span>
							<span class="payment-source">OVO Cash</span>
						</div>
					`,
					ReceivedAt: time.Now(),
				},
				Expected: validator.ExpectedExtraction{
					EventType:  domain.EventTypeExpense,
					Amount:     42000.0,
					Currency:   "IDR",
					Merchant:   "Kopi Kenangan Mantan",
					OccurredAt: "2026-10-25T15:40:00Z",
				},
			},
		},
	}

	// Grab Samples
	sets["*@grab.com"] = &validator.SampleSet{
		SenderPattern: "*@grab.com",
		Samples: []validator.SampleEmail{
			{
				ID: "grab-sample-01",
				RawEmail: domain.RawEmailMessage{
					MessageID:      "msg-grab-001",
					GmailAccountID: "acc-test",
					Sender:         "no-reply@grab.com",
					Subject:        "Your Grab E-Receipt",
					BodyHTML: `
						<div class="receipt">
							<div class="merchant-title">GrabFood - Bakmie GM</div>
							<div class="fare-total">IDR 85.500</div>
							<div class="timestamp">25/10/2026 19:10</div>
							<div class="paid-by">OVO</div>
						</div>
					`,
					ReceivedAt: time.Now(),
				},
				Expected: validator.ExpectedExtraction{
					EventType:  domain.EventTypeExpense,
					Amount:     85500.0,
					Currency:   "IDR",
					Merchant:   "Grabfood - Bakmie Gm",
					OccurredAt: "2026-10-25T19:10:00Z",
				},
			},
		},
	}

	return sets
}
