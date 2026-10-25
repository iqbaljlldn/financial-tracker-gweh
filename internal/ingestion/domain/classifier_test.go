package domain

import (
	"testing"
)

func TestClassifier_Classify(t *testing.T) {
	c := NewClassifier()

	tests := []struct {
		name          string
		msg           *RawMessage
		isKnownSender bool
		expected      RoutingDecision
	}{
		{
			name: "Known sender routes to template parser",
			msg: &RawMessage{
				Subject: "Your Amazon.com order of $45.00",
				Snippet: "Order confirmed",
			},
			isKnownSender: true,
			expected:      RoutingTemplateParser,
		},
		{
			name: "Clear transactional email with currency and receipt keyword",
			msg: &RawMessage{
				Subject: "Receipt for your payment to Stripe",
				Snippet: "You were charged $29.99 for your subscription",
			},
			isKnownSender: false,
			expected:      RoutingAIExtractor,
		},
		{
			name: "Indonesian transaction with Rp and struk",
			msg: &RawMessage{
				Subject: "Struk Pembayaran Tagihan Listrik",
				Snippet: "Total pembayaran Rp 150.000 berhasil didebit",
			},
			isKnownSender: false,
			expected:      RoutingAIExtractor,
		},
		{
			name: "Clear newsletter marketing with unsubscribe header and no currency",
			msg: &RawMessage{
				Subject: "Weekly digest: top 10 trends in tech",
				Snippet: "Check out our latest articles and special offer",
				Headers: map[string]string{
					"List-Unsubscribe": "<https://example.com/unsub>",
				},
			},
			isKnownSender: false,
			expected:      RoutingDiscarded,
		},
		{
			name: "Ambiguous message without clear negative markers routes to AI extractor [GATE]",
			msg: &RawMessage{
				Subject: "Account notice regarding recent activity",
				Snippet: "Please check your account portal for updates",
			},
			isKnownSender: false,
			expected:      RoutingAIExtractor, // Permissive-on-ambiguity rule
		},
		{
			name: "European invoice with EUR symbol and invoice keyword",
			msg: &RawMessage{
				Subject: "Invoice #INV-2026-881",
				Snippet: "Amount due: €120.50 due by next week",
			},
			isKnownSender: false,
			expected:      RoutingAIExtractor,
		},
		{
			name: "Singapore Grab ride receipt with SGD",
			msg: &RawMessage{
				Subject: "Your Grab E-Receipt",
				Snippet: "Total paid: 18.50 SGD via GrabPay",
			},
			isKnownSender: false,
			expected:      RoutingAIExtractor,
		},
		{
			name: "Marketing email with discount promo keyword and unsubscribe link",
			msg: &RawMessage{
				Subject: "Diskon hingga 70% flash sale akhir bulan!",
				Snippet: "Jangan lewatkan promo menarik kami. Berhenti berlangganan di sini.",
				Headers: map[string]string{
					"List-Unsubscribe": "<mailto:unsub@promo.com>",
				},
			},
			isKnownSender: false,
			expected:      RoutingDiscarded,
		},
		{
			name: "Marketing email that mentions a price inside a newsletter is ambiguous and NOT discarded [GATE]",
			msg: &RawMessage{
				Subject: "Weekly deals newsletter",
				Snippet: "Special offer: only $5 for first-time buyers!",
				Headers: map[string]string{
					"List-Unsubscribe": "<mailto:unsub@deals.com>",
				},
			},
			isKnownSender: false,
			expected:      RoutingAIExtractor, // Has currency, so never discarded
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Classify(tt.msg, tt.isKnownSender)
			if got != tt.expected {
				t.Fatalf("expected routing %s, got %s", tt.expected, got)
			}
		})
	}
}
