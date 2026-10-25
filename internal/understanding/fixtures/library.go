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

	// 6. Bank Syariah Indonesia (BSI / BYOND by BSI)
	bsiRules := domain.ExtractionRules{
		SenderPattern: "*@bankbsi.co.id",
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `(?i)Total\s*(?:Rp\s*)?([0-9.,]+)`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDecimal,
				},
				Required: true,
			},
			"occurredAt": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `(?i)Tanggal\s*([0-9]{1,2}\s+[a-zA-Z]{3,9}\s+[0-9]{4})`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDate + ":02 Jan 2006",
				},
				Required: true,
			},
			"merchant": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `(?i)(?:Nama Merchant|Pembayaran|Pembelian)\s*([^\n\r<]+)`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingTitleCase,
				},
				Required: true,
			},
			"paymentMethod": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `(?i)(?:Terminal\s*|Rekening Sumber[\s\S]{0,100}?)(\*{4,}\d{4})`,
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
	tmplBSI, _ := domain.NewSenderTemplate("tmpl-bsi-001", "*@bankbsi.co.id", 1, bsiRules, true, time.Now())

	// 7. Bank Rakyat Indonesia (BRI / BRImo)
	briRules := domain.ExtractionRules{
		SenderPattern: "*@bri.co.id",
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `(?i)Total Transaksi[\s\S]*?(?:Rp\s*)?([0-9.,]+)`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDecimal,
				},
				Required: true,
			},
			"occurredAt": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `([0-9]{1,2}\s+[a-zA-Z]{3,9}\s+[0-9]{4},\s+[0-9]{2}:[0-9]{2}:[0-9]{2})`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDate + ":02 January 2006, 15:04:05",
				},
				Required: true,
			},
			"merchant": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `(?i)Jenis Transaksi\s*([A-Za-z0-9 ]+)`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingTitleCase,
				},
				Required: true,
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
	tmplBRI, _ := domain.NewSenderTemplate("tmpl-bri-001", "*@bri.co.id", 1, briRules, true, time.Now())

	// 8. by.U (Telkomsel)
	byuRules := domain.ExtractionRules{
		SenderPattern: "*@byu.id",
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `(?i)Total Pembayaran[\s\S]*?(?:Rp\s*)?([0-9.,]+)`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDecimal,
				},
				Required: true,
			},
			"occurredAt": {
				SelectorType:       domain.SelectorRegex,
				SelectorExpression: `Tanggal Pembayaran\s*([0-9]{1,2}\s+[a-zA-Z]{3,9}\s+[0-9]{4}\s+[0-9]{2}:[0-9]{2}:[0-9]{2})`,
				ExtractionGroup:    1,
				PostProcessing: []string{
					domain.PostProcessingTrimWhitespace,
					domain.PostProcessingParseDate + ":02 January 2006 15:04:05",
				},
				Required: true,
			},
			"merchant": {
				SelectorType: domain.SelectorStatic,
				StaticValue:  "by.U",
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
	tmplByu, _ := domain.NewSenderTemplate("tmpl-byu-001", "*@byu.id", 1, byuRules, true, time.Now())

	return []*domain.SenderTemplate{
		tmplBCA,
		tmplMandiri,
		tmplGojek,
		tmplOVO,
		tmplGrab,
		tmplBSI,
		tmplBRI,
		tmplByu,
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

	// BSI Samples
	sets["*@bankbsi.co.id"] = &validator.SampleSet{
		SenderPattern: "*@bankbsi.co.id",
		Samples: []validator.SampleEmail{
			{
				ID: "bsi-sample-01",
				RawEmail: domain.RawEmailMessage{
					MessageID:      "msg-bsi-001",
					GmailAccountID: "acc-test",
					Sender:         "nonereply.byondbybsi@bankbsi.co.id",
					Subject:        "Transaksi Pembayaran QRIS MPM Kamu Berhasil",
					BodyHTML: `
						<table class="content">
							<tr>
								<td class="content-label">Nominal Transaksi</td>
								<td class="content-value">Rp 15.000,00</td>
							</tr>
							<tr>
								<td class="content-label">Tanggal</td>
								<td class="content-value">24 Okt 2026</td>
							</tr>
							<tr>
								<td class="content-label">Jam</td>
								<td class="content-value">10:40:22 WIB</td>
							</tr>
							<tr>
								<td class="content-label">Nama Merchant</td>
								<td class="content-value">Masita (Ricebowl Salad &</td>
							</tr>
							<tr>
								<td class="content-label">Total</td>
								<td class="content-value">Rp 15.000,00</td>
							</tr>
							<tr>
								<td class="content-label-no-border-bottom">Terminal</td>
								<td class="content-value-no-border-bottom">*********1309</td>
							</tr>
						</table>
					`,
					ReceivedAt: time.Now(),
				},
				Expected: validator.ExpectedExtraction{
					EventType:  domain.EventTypeExpense,
					Amount:     15000.0,
					Currency:   "IDR",
					Merchant:   "Masita (Ricebowl Salad &",
					OccurredAt: "2026-10-24T00:00:00Z",
				},
			},
		},
	}

	// BRI Samples
	sets["*@bri.co.id"] = &validator.SampleSet{
		SenderPattern: "*@bri.co.id",
		Samples: []validator.SampleEmail{
			{
				ID: "bri-sample-01",
				RawEmail: domain.RawEmailMessage{
					MessageID:      "msg-bri-001",
					GmailAccountID: "acc-test",
					Sender:         "BankBRI@bri.co.id",
					Subject:        "Top Up DANA Berhasil",
					BodyHTML: `
						<div>
							<strong>05 Februari 2026, 13:14:18 WIB</strong>
							Total Transaksi <br><strong style="font-weight:700;color:#00529c;font-size:18px">Rp20.000</strong>
							<table>
								<tr>
									<td>Jenis Transaksi</td>
									<td>DANA</td>
								</tr>
							</table>
						</div>
					`,
					ReceivedAt: time.Now(),
				},
				Expected: validator.ExpectedExtraction{
					EventType:  domain.EventTypeExpense,
					Amount:     20000.0,
					Currency:   "IDR",
					Merchant:   "DANA",
					OccurredAt: "2026-02-05T13:14:18Z",
				},
			},
		},
	}

	// by.U Samples
	sets["*@byu.id"] = &validator.SampleSet{
		SenderPattern: "*@byu.id",
		Samples: []validator.SampleEmail{
			{
				ID: "byu-sample-01",
				RawEmail: domain.RawEmailMessage{
					MessageID:      "msg-byu-001",
					GmailAccountID: "acc-test",
					Sender:         "noreply@byu.id",
					Subject:        "Yay! Pembayaran berhasil.",
					BodyHTML: `
						<div>
							<p>Tanggal Pembayaran</p>
							<p>12 February 2026 12:42:34</p>
							<p>Total Pembayaran Barang/Jasa</p>
							<p>Rp 28000</p>
						</div>
					`,
					ReceivedAt: time.Now(),
				},
				Expected: validator.ExpectedExtraction{
					EventType:  domain.EventTypeExpense,
					Amount:     28000.0,
					Currency:   "IDR",
					Merchant:   "by.U",
					OccurredAt: "2026-02-12T12:42:34Z",
				},
			},
		},
	}

	return sets
}
