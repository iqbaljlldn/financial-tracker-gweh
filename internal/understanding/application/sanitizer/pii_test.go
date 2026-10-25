package sanitizer_test

import (
	"strings"
	"testing"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/sanitizer"
)

func TestSanitizeEmail_MaskingCardAndAccountNumbers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "16 digit credit card with spaces",
			input:    "Your payment using card 4111 2222 3333 4821 was successful.",
			expected: "Your payment using card ****4821 was successful.",
		},
		{
			name:     "16 digit credit card with dashes",
			input:    "Debit Card: 5412-7512-3412-9988 charged $45.00",
			expected: "Debit Card: ****9988 charged $45.00",
		},
		{
			name:     "Continuous 16 digit card",
			input:    "Card number 1234567812345678 debited.",
			expected: "Card number ****5678 debited.",
		},
		{
			name:     "10 digit bank account number",
			input:    "Rekening BCA 0123456789 atas nama John Doe.",
			expected: "Rekening BCA ****6789 atas nama John Doe.",
		},
		{
			name:     "Short numbers like amount or year should NOT be masked",
			input:    "Year 2026, amount $1250.00, order #987654",
			expected: "Year 2026, amount $1250.00, order #987654",
		},
		{
			name:     "IBAN masking",
			input:    "Transfer from IBAN DE89370400440532013000 completed.",
			expected: "Transfer from IBAN DE89****3000 completed.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := sanitizer.SanitizeEmail(tt.input)
			if actual != tt.expected {
				t.Errorf("SanitizeEmail() mismatch\nExpected: %q\nGot:      %q", tt.expected, actual)
			}
		})
	}
}

func TestSanitizeEmail_StripFootersAndBoilerplate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Strip unsubscribe link and footer",
			input: `Thank you for your order at Amazon. Total: $45.00.
---
To unsubscribe from marketing emails, click here to unsubscribe.
Amazon Services LLC.`,
			expected: "Thank you for your order at Amazon. Total: $45.00.",
		},
		{
			name: "Strip legal confidentiality notice",
			input: `Payment confirmation: $120.00 to Netflix.
This email and any attachments is confidential and intended solely for the recipient.
If you have received this message in error please notify us.`,
			expected: "Payment confirmation: $120.00 to Netflix.",
		},
		{
			name: "Strip Indonesian automated email boilerplate",
			input: `Transaksi GoPay Rp 25.000 berhasil di Kopi Kenangan.
Ini adalah email otomatis, mohon tidak membalas email ini.`,
			expected: "Transaksi GoPay Rp 25.000 berhasil di Kopi Kenangan.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := sanitizer.SanitizeEmail(tt.input)
			if actual != tt.expected {
				t.Errorf("SanitizeEmail() mismatch\nExpected: %q\nGot:      %q", tt.expected, actual)
			}
		})
	}
}

func TestSanitizeEmail_TokenBudgetTruncation(t *testing.T) {
	longBody := strings.Repeat("A very long receipt line with various items purchased. ", 100)
	cfg := sanitizer.PIIConfig{MaxCharacters: 200}
	sanitized := sanitizer.SanitizeEmail(longBody, cfg)

	if len(sanitized) > 250 { // 200 + truncated marker
		t.Errorf("expected truncated string to be around 200 chars, got length %d", len(sanitized))
	}
	if !strings.Contains(sanitized, "[Content truncated for token budget]") {
		t.Errorf("expected truncation marker in output")
	}
}
