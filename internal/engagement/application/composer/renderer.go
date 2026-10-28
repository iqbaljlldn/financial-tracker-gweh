package composer

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
)

// TemplateDefinition defines template ID, text template, and default action buttons.
type TemplateDefinition struct {
	TemplateID string
	Body       string
	Buttons    []domain.ActionButton
}

var catalog = map[domain.NotificationType]TemplateDefinition{
	domain.NotificationTypeDailySummary: {
		TemplateID: "daily_summary_v1",
		Body: strings.TrimSpace(`
📊 *Ringkasan Harian Keuangan*
Halo! Berikut ringkasan transaksi Anda kemarin:

• Total Pengeluaran: *{{.totalSpendYesterday}}*
• Kategori Terbesar: *{{.topCategory}}*
• Jumlah Transaksi: *{{.eventCount}}*

_Pantau keuangan sehat Anda di Financial Tracker._
`),
	},
	domain.NotificationTypeWeeklySummary: {
		TemplateID: "weekly_summary_v1",
		Body: strings.TrimSpace(`
📈 *Laporan Mingguan Keuangan*
Ringkasan keuangan minggu ini:

• Total Pemasukan: *{{.totalIncome}}*
• Total Pengeluaran: *{{.totalExpense}}*
• Kategori Terbesar: *{{.topCategory}}*
• Merchant Terbanyak: *{{.topMerchant}}*

_Tetap jaga arus kas positif minggu ini!_
`),
	},
	domain.NotificationTypeMonthlyReport: {
		TemplateID: "monthly_report_v1",
		Body: strings.TrimSpace(`
📅 *Evaluasi Bulanan Keuangan*
Laporan performa keuangan bulan ini:

• Arus Kas Bersih (Net Flow): *{{.netFlow}}*
• Realisasi Budget: *{{.budgetPerformancePercent}}%*
• Tren Dibanding Bulan Lalu: *{{.comparisonToLastMonth}}*
`),
	},
	domain.NotificationTypeBudgetAlert: {
		TemplateID: "budget_alert_v1",
		Body: strings.TrimSpace(`
⚠️ *Peringatan Budget!*
Pengeluaran kategori *{{.categoryOrGlobal}}* telah mencapai *{{.percentUsed}}%* dari alokasi budget.

• Ambang Batas: *{{.thresholdAmount}}*
• Total Pengeluaran: *{{.currentSpend}}*

_Kendalikan belanja Anda untuk menjaga kesehatan finansial._
`),
	},
	domain.NotificationTypeReviewRequest: {
		TemplateID: "review_request_v1",
		Body: strings.TrimSpace(`
🔍 *Verifikasi Transaksi Baru*
Sistem mendeteksi transaksi yang membutuhkan konfirmasi Anda:

• Nominal: *{{.amount}}*
• Merchant: *{{.merchant}}*
• Kategori: *{{.suggestedCategory}}*

👉 Ketik *Y* untuk Konfirmasi, atau *N* untuk Koreksi.
`),
		Buttons: []domain.ActionButton{
			{ID: "confirm", Label: "Confirm"},
			{ID: "edit", Label: "Edit"},
		},
	},
	domain.NotificationTypeTransactionFinalized: {
		TemplateID: "transaction_finalized_v1",
		Body: strings.TrimSpace(`
💳 *Transaksi Berhasil Dicatat*
Transaksi baru Anda telah berhasil dicatat ke pembukuan:

• Jenis: *{{.eventType}}*
• Nominal: *{{.amount}}*
• Merchant: *{{.merchant}}*
• Kategori: *{{.category}}*
• Waktu: *{{.occurredAt}}*

_Catatan keuangan Anda telah otomatis diperbarui di Financial Tracker._
`),
	},
}

// TemplateRenderer renders notification messages with formatted parameters.
type TemplateRenderer struct {
	templates map[domain.NotificationType]*template.Template
}

// NewTemplateRenderer initializes compiled Go text templates for all notification types.
func NewTemplateRenderer() (*TemplateRenderer, error) {
	compiled := make(map[domain.NotificationType]*template.Template, len(catalog))
	for nType, def := range catalog {
		tmpl, err := template.New(def.TemplateID).Parse(def.Body)
		if err != nil {
			return nil, fmt.Errorf("composer: parsing template %s: %w", def.TemplateID, err)
		}
		compiled[nType] = tmpl
	}
	return &TemplateRenderer{templates: compiled}, nil
}

// Render compiles template and substitutes placeholder values.
func (r *TemplateRenderer) Render(
	nType domain.NotificationType,
	recipientPhone string,
	params map[string]string,
	buttonEventID string,
) (*domain.OutboundMessage, error) {
	def, ok := catalog[nType]
	if !ok {
		return nil, fmt.Errorf("composer: no template definition mapped for %s", nType)
	}

	tmpl, ok := r.templates[nType]
	if !ok {
		return nil, fmt.Errorf("composer: template %s not compiled", def.TemplateID)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return nil, fmt.Errorf("composer: executing template %s: %w", def.TemplateID, err)
	}

	buttons := make([]domain.ActionButton, len(def.Buttons))
	for i, b := range def.Buttons {
		btnID := b.ID
		if buttonEventID != "" {
			btnID = fmt.Sprintf("%s:%s", b.ID, buttonEventID)
		}
		buttons[i] = domain.ActionButton{
			ID:    btnID,
			Label: b.Label,
		}
	}

	return &domain.OutboundMessage{
		RecipientPhone:   recipientPhone,
		NotificationType: nType,
		TemplateID:       def.TemplateID,
		Parameters:       params,
		RenderedText:     buf.String(),
		Buttons:          buttons,
	}, nil
}
