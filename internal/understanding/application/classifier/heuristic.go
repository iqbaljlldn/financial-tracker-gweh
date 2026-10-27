package classifier

import (
	"regexp"
	"strings"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

var (
	// Currency / amount pattern: matches Rp 50.000, IDR 50000, $45.00, 50.00 USD, etc.
	currencyRegex = regexp.MustCompile(`(?i)(?:Rp\.?\s*\d[\d\.,]*|\bIDR\s*\d[\d\.,]*|\$\s*\d[\d\.,]*|\bUSD\s*\d[\d\.,]*|\bEUR\s*\d[\d\.,]*|\bGBP\s*\d[\d\.,]*|\bSGD\s*\d[\d\.,]*|\d[\d\.,]*\s*(?:IDR|USD|EUR|GBP|SGD))`)

	// Explicit transaction confirmation phrases in Indonesian & English
	positiveConfirmationRegex = regexp.MustCompile(`(?i)(\breceipt\b|\binvoice\b|\bstruk\b|bukti\s+transfer|transfer\s+berhasil|pembayaran\s+berhasil|transaksi\s+berhasil|berhasil\s+dibayar|telah\s+dibayar|telah\s+menerima\s+pembayaran|rincian\s+pembayaran|struk\s+transaksi|receipt\s+for|payment\s+received|payment\s+successful|order\s+confirmed|order\s+confirmation|invoice\s+paid|you\s+paid|charged\s+\$|debit\s+alert|kredit\s+alert|dana\s+masuk|uang\s+masuk)`)

	// Strong negative signals in Subject
	negativeSubjectRegex = regexp.MustCompile(`(?i)(kode\s+otp|kode\s+verifikasi|verification\s+code|verifikasi\s+akun|verify\s+your\s+email|konfirmasi\s+email|email\s+verification|reset\s+password|atur\s+ulang\s+kata\s+sandi|password\s+reset|security\s+alert|peringatan\s+keamanan|login\s+alert|peringatan\s+masuk|masuk\s+dari\s+perangkat|new\s+login|two-factor|2fa|authenticator|ticket\s*#|tiket\s*#|tiket\s+bantuan|support\s+ticket|tiket\s+telah\s+dijawab|tiket\s+ditutup|ticket\s+closed|ticket\s+resolved|selamat\s+datang|welcome\s+to|aktivasi\s+akun|activate\s+your\s+account|pendaftaran\s+berhasil|registration\s+successful|device\s+connected|perangkat\s+terhubung|device\s+disconnected|newsletter|weekly\s+digest|monthly\s+digest|survei|survey|kuesioner|kebijakan\s+privasi|privacy\s+policy|terms\s+of\s+service|syarat\s+dan\s+ketentuan)`)

	// Common non-transactional sender keywords
	negativeSenderRegex = regexp.MustCompile(`(?i)(no-reply|noreply|donotreply|newsletter|support@|care@|help@|billing-update@|security@|auth@|notifications@)`)
)

// EvaluateTransactionalContent evaluates an email using lightweight deterministic heuristics per 12_gmail_connector.md §7.1.
// Returns isClearlyNonTransactional, reason.
func EvaluateTransactionalContent(raw domain.RawEmailMessage) (isNonTransactional bool, reason string) {
	subject := strings.TrimSpace(raw.Subject)
	body := strings.TrimSpace(raw.BodyText)
	if body == "" {
		body = raw.BodyHTML
	}
	combined := subject + " " + body

	hasCurrency := currencyRegex.MatchString(combined)
	hasPositiveConfirmation := positiveConfirmationRegex.MatchString(combined)

	// If it has positive confirmation AND explicit currency, it is a genuine transaction
	if hasPositiveConfirmation && hasCurrency {
		return false, "has_positive_transaction_confirmation_and_currency"
	}

	// 1. Negative subject match
	if negativeSubjectRegex.MatchString(subject) {
		// Even if negative keyword is in subject, if there's no positive confirmation and currency, discard
		if !hasPositiveConfirmation {
			return true, "negative_subject_match"
		}
	}

	// 2. Unsubscribe header check (List-Unsubscribe is standard for marketing/newsletters)
	hasUnsubscribeHeader := false
	if raw.Headers != nil {
		for k := range raw.Headers {
			if strings.EqualFold(k, "List-Unsubscribe") || strings.EqualFold(k, "List-Unsubscribe-Post") {
				hasUnsubscribeHeader = true
				break
			}
		}
	}
	if hasUnsubscribeHeader && !hasPositiveConfirmation && !hasCurrency {
		return true, "list_unsubscribe_header_without_transaction"
	}

	// 3. Negative sender pattern without currency/amount
	if negativeSenderRegex.MatchString(raw.Sender) && !hasCurrency && !hasPositiveConfirmation {
		return true, "negative_sender_without_currency_or_confirmation"
	}

	// 4. If there is NO currency/amount pattern at all and NO positive confirmation, it's not a financial transaction
	if !hasCurrency && !hasPositiveConfirmation {
		return true, "no_currency_or_transaction_confirmation_found"
	}

	return false, "ambiguous_or_potential_transaction"
}
