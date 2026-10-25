package domain

import (
	"regexp"
	"strings"
)

var (
	// Currency patterns: $, €, £, ¥, Rp, IDR, USD, EUR, SGD, GBP or decimal amounts
	currencyRegex = regexp.MustCompile(`(?i)(\$|€|£|¥|rp\.?|usd|idr|eur|sgd|gbp)\s*[\d,\.]+|\b[\d,\.]+\s*(usd|idr|eur|sgd|gbp)\b`)

	// Transactional keywords (multi-language: English & Indonesian)
	transactionalKeywords = []string{
		"receipt", "invoice", "payment", "charged", "transferred", "transfer",
		"order confirmation", "purchase", "subscription", "billed", "statement",
		"pembayaran", "tagihan", "struk", "kwitansi", "transaksi", "saldo", "debit", "kredit",
	}

	// Negative markers: marketing newsletters, promo digests, notifications without financial substance
	marketingKeywords = []string{
		"unsubscribe", "berhenti berlangganan", "newsletter", "weekly digest",
		"special offer", "diskon hingga", "promo menarik", "flash sale",
	}
)

// SenderClassifier defines the interface for evaluating message transactional heuristics.
type SenderClassifier interface {
	Classify(msg *RawMessage, isKnownSender bool) RoutingDecision
}

// Classifier evaluates raw message content against transactional heuristics per 12_gmail_connector.md §7.1.
// Permissive-on-ambiguity rule [GATE]: ambiguous cases route to ai_extractor, never silently discarded.
type Classifier struct{}

// NewClassifier creates a new heuristic classifier.
func NewClassifier() *Classifier {
	return &Classifier{}
}

// Classify determines the RoutingDecision for an ingested raw message.
func (c *Classifier) Classify(msg *RawMessage, isKnownSender bool) RoutingDecision {
	if isKnownSender {
		return RoutingTemplateParser
	}

	combined := extractTextPreview(msg)
	hasCurrency := currencyRegex.MatchString(combined)
	hasTxKeyword := hasAnyKeyword(combined, transactionalKeywords)

	// Case 1 & 2: Currency or transaction keyword present -> route to ai_extractor [GATE]
	if hasCurrency || hasTxKeyword {
		return RoutingAIExtractor
	}

	hasUnsubscribeHeader := hasListUnsubscribeHeader(msg.Headers)
	hasMarketingKeyword := hasAnyKeyword(combined, marketingKeywords)

	// Case 3: Strong negative signal (List-Unsubscribe header + marketing keywords + zero currency/tx keywords)
	if (hasUnsubscribeHeader || hasMarketingKeyword) && !hasCurrency && !hasTxKeyword {
		return RoutingDiscarded
	}

	// Case 4: Ambiguous heuristic (permissive-on-ambiguity rule [GATE] -> ai_extractor)
	return RoutingAIExtractor
}

func extractTextPreview(msg *RawMessage) string {
	subject := strings.ToLower(msg.Subject)
	snippet := strings.ToLower(msg.Snippet)
	body := strings.ToLower(msg.BodyText)
	if len(body) > 2048 {
		body = body[:2048]
	}
	return subject + " " + snippet + " " + body
}

func hasAnyKeyword(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func hasListUnsubscribeHeader(headers map[string]string) bool {
	if headers == nil {
		return false
	}
	_, ok := headers["List-Unsubscribe"]
	return ok
}
