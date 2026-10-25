package executor

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// Outcome represents template execution outcome per 13_email_parser.md §5.2.
type Outcome string

const (
	OutcomeFullMatch    Outcome = "full"
	OutcomePartialMatch Outcome = "partial"
	OutcomeFailed       Outcome = "failed"
)

// ExecutionResult models the outcome of running a SenderTemplate against a raw email message.
type ExecutionResult struct {
	Outcome             Outcome
	ConfidenceAggregate float64
	ConfidencePerField  map[string]float64
	Candidate           *domain.CandidateFinancialEvent
	MissingFields       []string
	ExtractedFields     map[string]string
	RawFields           map[string]string
	Err                 error
}

// ExtractionExecutor deterministically applies DSL rules against email DOM and text structures.
type ExtractionExecutor struct{}

// NewExtractionExecutor returns a new ExtractionExecutor.
func NewExtractionExecutor() *ExtractionExecutor {
	return &ExtractionExecutor{}
}

type fieldExtractionState struct {
	extracted          map[string]string
	raw                map[string]string
	confidencePerField map[string]float64
	missingFields      []string
	hasOptionalMissing bool
}

// Execute applies the template's rules to raw message content and constructs CandidateFinancialEvent.
func (e *ExtractionExecutor) Execute(
	tmpl *domain.SenderTemplate,
	msg *domain.RawEmailMessage,
	userID string,
) *ExecutionResult {
	if tmpl == nil {
		return &ExecutionResult{Outcome: OutcomeFailed, Err: domain.ErrTemplateNotFound}
	}
	if msg == nil {
		return &ExecutionResult{Outcome: OutcomeFailed, Err: errors.New("raw message is nil")}
	}

	state := e.extractAllFields(tmpl.Rules().Fields, msg)
	if len(state.missingFields) > 0 {
		return &ExecutionResult{
			Outcome:             OutcomeFailed,
			ConfidenceAggregate: 0.0,
			ConfidencePerField:  state.confidencePerField,
			MissingFields:       state.missingFields,
			ExtractedFields:     state.extracted,
			RawFields:           state.raw,
			Err:                 fmt.Errorf("%w: %v", domain.ErrRequiredFieldMissing, state.missingFields),
		}
	}

	aggregateConfidence := 1.0
	outcome := OutcomeFullMatch
	if state.hasOptionalMissing {
		outcome = OutcomePartialMatch
		aggregateConfidence = 0.95 // Slightly reduced confidence for partial match per 13_email_parser.md §5.2
	}

	candidate, err := buildCandidateEvent(tmpl, msg, userID, state.extracted, state.raw, aggregateConfidence, state.confidencePerField)
	if err != nil {
		return &ExecutionResult{
			Outcome:             OutcomeFailed,
			ConfidenceAggregate: 0.0,
			ConfidencePerField:  state.confidencePerField,
			MissingFields:       state.missingFields,
			ExtractedFields:     state.extracted,
			RawFields:           state.raw,
			Err:                 fmt.Errorf("%w: %v", domain.ErrExtractionExecutionFailed, err),
		}
	}

	return &ExecutionResult{
		Outcome:             outcome,
		ConfidenceAggregate: aggregateConfidence,
		ConfidencePerField:  state.confidencePerField,
		Candidate:           candidate,
		ExtractedFields:     state.extracted,
		RawFields:           state.raw,
	}
}

func (e *ExtractionExecutor) extractAllFields(
	rules map[string]domain.FieldRule,
	msg *domain.RawEmailMessage,
) *fieldExtractionState {
	state := &fieldExtractionState{
		extracted:          make(map[string]string),
		raw:                make(map[string]string),
		confidencePerField: make(map[string]float64),
	}

	var htmlDoc *goquery.Document
	getHTMLDoc := func() (*goquery.Document, error) {
		if htmlDoc != nil {
			return htmlDoc, nil
		}
		if strings.TrimSpace(msg.BodyHTML) == "" {
			return nil, errors.New("empty HTML body")
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(msg.BodyHTML))
		if err != nil {
			return nil, err
		}
		htmlDoc = doc
		return htmlDoc, nil
	}

	for fieldName, rule := range rules {
		e.processFieldRule(fieldName, rule, msg, getHTMLDoc, state)
	}

	return state
}

func (e *ExtractionExecutor) processFieldRule(
	fieldName string,
	rule domain.FieldRule,
	msg *domain.RawEmailMessage,
	getHTMLDoc func() (*goquery.Document, error),
	state *fieldExtractionState,
) {
	rawVal, extractErr := extractRawValue(rule, msg, getHTMLDoc)
	if extractErr != nil || strings.TrimSpace(rawVal) == "" {
		markFieldFailure(fieldName, rule.Required, state)
		return
	}

	state.raw[fieldName] = rawVal

	processedVal, procErr := domain.ApplyPostProcessing(rawVal, rule.PostProcessing)
	if procErr != nil || strings.TrimSpace(processedVal) == "" {
		markFieldFailure(fieldName, rule.Required, state)
		return
	}

	state.extracted[fieldName] = processedVal
	state.confidencePerField[fieldName] = 1.0
}

func markFieldFailure(fieldName string, required bool, state *fieldExtractionState) {
	state.confidencePerField[fieldName] = 0.0
	if required {
		state.missingFields = append(state.missingFields, fieldName)
	} else {
		state.hasOptionalMissing = true
	}
}

func extractRawValue(
	rule domain.FieldRule,
	msg *domain.RawEmailMessage,
	getHTMLDoc func() (*goquery.Document, error),
) (string, error) {
	switch rule.SelectorType {
	case domain.SelectorStatic:
		return rule.StaticValue, nil
	case domain.SelectorHeader:
		return extractHeaderValue(rule.SelectorExpression, msg)
	case domain.SelectorCSS:
		return extractCSSValue(rule, getHTMLDoc)
	case domain.SelectorRegex:
		return extractRegexValue(rule, msg, getHTMLDoc)
	default:
		return "", fmt.Errorf("%w: %s", domain.ErrUnknownSelectorType, rule.SelectorType)
	}
}

func extractHeaderValue(headerExpr string, msg *domain.RawEmailMessage) (string, error) {
	headerName := strings.ToLower(headerExpr)
	for k, v := range msg.Headers {
		if strings.ToLower(k) == headerName {
			return v, nil
		}
	}
	if headerName == "from" && msg.Sender != "" {
		return msg.Sender, nil
	}
	if headerName == "subject" && msg.Subject != "" {
		return msg.Subject, nil
	}
	return "", fmt.Errorf("header %q not found", headerExpr)
}

func extractCSSValue(rule domain.FieldRule, getHTMLDoc func() (*goquery.Document, error)) (string, error) {
	doc, err := getHTMLDoc()
	if err != nil {
		return "", err
	}
	sel := doc.Find(rule.SelectorExpression)
	if sel.Length() == 0 {
		return "", fmt.Errorf("CSS selector %q matched 0 elements", rule.SelectorExpression)
	}
	if rule.Attribute != "" {
		val, exists := sel.First().Attr(rule.Attribute)
		if !exists {
			return "", fmt.Errorf("attribute %q not found on element", rule.Attribute)
		}
		return val, nil
	}
	return sel.First().Text(), nil
}

func extractRegexValue(rule domain.FieldRule, msg *domain.RawEmailMessage, getHTMLDoc func() (*goquery.Document, error)) (string, error) {
	re, err := regexp.Compile(rule.SelectorExpression)
	if err != nil {
		return "", fmt.Errorf("invalid regex %q: %w", rule.SelectorExpression, err)
	}
	targetText := msg.BodyText
	if targetText == "" {
		if doc, err := getHTMLDoc(); err == nil {
			targetText = doc.Text()
		}
	}
	matches := re.FindStringSubmatch(targetText)
	if len(matches) == 0 {
		return "", fmt.Errorf("regex %q matched no text", rule.SelectorExpression)
	}
	if rule.ExtractionGroup > 0 && rule.ExtractionGroup < len(matches) {
		return matches[rule.ExtractionGroup], nil
	}
	return matches[0], nil
}

func buildCandidateEvent(
	tmpl *domain.SenderTemplate,
	msg *domain.RawEmailMessage,
	userID string,
	extracted map[string]string,
	raw map[string]string,
	aggregateConfidence float64,
	confidencePerField map[string]float64,
) (*domain.CandidateFinancialEvent, error) {
	amountVal, err := strconv.ParseFloat(extracted["amount"], 64)
	if err != nil || amountVal <= 0 {
		return nil, fmt.Errorf("invalid extracted amount: %q", extracted["amount"])
	}

	currency := extracted["currency"]
	if currency == "" {
		currency = "IDR"
	}

	occurredAt, err := time.Parse(time.RFC3339, extracted["occurredAt"])
	if err != nil {
		if !msg.ReceivedAt.IsZero() {
			occurredAt = msg.ReceivedAt
		} else {
			occurredAt = time.Now().UTC()
		}
	}

	rawMerchant := raw["merchant"]
	normMerchant := extracted["merchant"]
	if normMerchant == "" {
		normMerchant = domain.TitleCase(domain.TrimWhitespace(rawMerchant))
	}
	if normMerchant == "" {
		normMerchant = "Unknown Merchant"
	}

	eventType := extracted["eventTypeHint"]
	if eventType == "" {
		eventType = domain.EventTypeExpense
	}

	candidate := &domain.CandidateFinancialEvent{
		UserID:    userID,
		EventType: eventType,
		Amount: domain.Money{
			Value:    amountVal,
			Currency: currency,
		},
		OccurredAt: occurredAt,
		Merchant: domain.Merchant{
			NormalizedName: normMerchant,
			RawName:        rawMerchant,
		},
		PaymentMethod: domain.PaymentMethod{
			Type:   extracted["paymentMethodType"],
			Detail: extracted["paymentMethod"],
			Last4:  extracted["paymentMethodLast4"],
		},
		Confidence: domain.Confidence{
			Aggregate: aggregateConfidence,
			PerField:  confidencePerField,
		},
		Provenance: domain.Provenance{
			ConnectorType:      "gmail",
			ConnectorAccountID: msg.GmailAccountID,
			SourceMessageRef:   msg.MessageID,
			ExtractionMethod:   domain.ExtractionMethodTemplate,
			TemplateVersion:    fmt.Sprintf("v%d", tmpl.Version()),
			ExtractedAt:        time.Now().UTC(),
		},
		TypeSpecificPayload: buildTypeSpecificPayload(extracted),
	}

	if err := candidate.Validate(); err != nil {
		return nil, fmt.Errorf("validating candidate event: %w", err)
	}

	return candidate, nil
}

func buildTypeSpecificPayload(extracted map[string]string) map[string]any {
	typeSpecific := make(map[string]any)
	if catHint := extracted["categoryHint"]; catHint != "" {
		typeSpecific["expense_category_hint"] = catHint
	}
	if isRecurring := extracted["isRecurring"]; isRecurring != "" {
		typeSpecific["is_recurring"] = (isRecurring == "true" || isRecurring == "1")
	}
	return typeSpecific
}
