package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EventType is the FinancialEvent taxonomy (04_domain_model.md §5.2).
// The 14 leaf types plus the Income reporting grouping are all peers sharing one envelope (§5.3).
type EventType string

const (
	EventTypeIncome              EventType = "Income"
	EventTypeExpense             EventType = "Expense"
	EventTypeTransfer            EventType = "Transfer"
	EventTypeRefund              EventType = "Refund"
	EventTypeCashback            EventType = "Cashback"
	EventTypeSubscriptionRenewal EventType = "SubscriptionRenewal"
	EventTypeFailedPayment       EventType = "FailedPayment"
	EventTypeSalary              EventType = "Salary"
	EventTypeInvestment          EventType = "Investment"
	EventTypeDividend            EventType = "Dividend"
	EventTypeLoan                EventType = "Loan"
	EventTypeCreditCardBill      EventType = "CreditCardBill"
	EventTypeTax                 EventType = "Tax"
	EventTypeDonation            EventType = "Donation"
	EventTypeOthers              EventType = "Others"
)

// payloadKind enumerates the value shapes a type-specific attribute may take.
type payloadKind int

const (
	kindString payloadKind = iota
	kindBool
	kindDate
	kindMoney
	kindUUID
	kindEnum
)

type attributeSpec struct {
	kind   payloadKind
	values []string // for kindEnum
}

var (
	specString = attributeSpec{kind: kindString}
	specBool   = attributeSpec{kind: kindBool}
	specDate   = attributeSpec{kind: kindDate}
	specMoney  = attributeSpec{kind: kindMoney}
	specUUID   = attributeSpec{kind: kindUUID}
)

func enumSpec(values ...string) attributeSpec { return attributeSpec{kind: kindEnum, values: values} }

// typePayloadSpecs encodes the documented attributes for every event type
// (04_domain_model.md §5.1.2) [GATE: all taxonomy values carry their documented payload attributes].
var typePayloadSpecs = map[EventType]map[string]attributeSpec{
	EventTypeIncome: {
		"income_source": specString,
		"is_recurring":  specBool,
	},
	EventTypeExpense: {
		"expense_category_hint": specString,
		"is_recurring":          specBool,
	},
	EventTypeTransfer: {
		"counterparty_account_hint": specString,
		"direction":                 enumSpec("outgoing", "incoming"),
		"linked_transfer_event_id":  specUUID,
	},
	EventTypeRefund: {
		"original_event_ref": specString,
	},
	EventTypeCashback: {
		"originating_expense_ref": specString,
		"program_name":            specString,
	},
	EventTypeSubscriptionRenewal: {
		"subscription_name":  specString,
		"renewal_cycle":      enumSpec("monthly", "annual"),
		"next_expected_date": specDate,
	},
	EventTypeFailedPayment: {
		"failure_reason_hint": specString,
		"related_merchant":    specString,
		"attempted_amount":    specMoney,
	},
	EventTypeSalary: {
		"employer":   specString,
		"pay_period": specString,
	},
	EventTypeInvestment: {
		"instrument_name":   specString,
		"investment_action": enumSpec("buy", "sell", "contribution"),
	},
	EventTypeDividend: {
		"instrument_name": specString,
		"payer":           specString,
	},
	EventTypeLoan: {
		"loan_action": enumSpec("disbursement", "repayment"),
		"lender":      specString,
	},
	EventTypeCreditCardBill: {
		"statement_period": specString,
		"minimum_due":      specMoney,
		"total_due":        specMoney,
		"due_date":         specDate,
	},
	EventTypeTax: {
		"tax_type_hint": specString,
		"tax_authority": specString,
	},
	EventTypeDonation: {
		"recipient_organization": specString,
	},
	EventTypeOthers: {
		"raw_hint": specString,
	},
}

// AllEventTypes returns the full taxonomy in stable alphabetical order.
func AllEventTypes() []EventType {
	types := make([]EventType, 0, len(typePayloadSpecs))
	for t := range typePayloadSpecs {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	return types
}

// ParseEventType validates a taxonomy value.
func ParseEventType(s string) (EventType, error) {
	t := EventType(strings.TrimSpace(s))
	if _, ok := typePayloadSpecs[t]; !ok {
		return "", fmt.Errorf("%w: %q", ErrInvalidEventType, s)
	}
	return t, nil
}

// PayloadAttributes returns the documented attribute names for an event type (sorted).
func PayloadAttributes(t EventType) []string {
	specs := typePayloadSpecs[t]
	names := make([]string, 0, len(specs))
	for name := range specs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

var isoDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func normalizeDate(v any) (any, bool) {
	s, ok := v.(string)
	if !ok {
		return nil, false
	}
	if isoDatePattern.MatchString(s) {
		if _, err := time.Parse("2006-01-02", s); err == nil {
			return s, true
		}
		return nil, false
	}
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return s, true
	}
	return nil, false
}

func normalizeMoney(v any) (any, bool) {
	switch n := v.(type) {
	case string:
		norm, err := NormalizeDecimal(n)
		return norm, err == nil
	case float64:
		norm, err := NormalizeDecimal(strconv.FormatFloat(n, 'f', 4, 64))
		return norm, err == nil
	default:
		return nil, false
	}
}

func normalizeAttribute(spec attributeSpec, v any) (any, bool) {
	switch spec.kind {
	case kindString:
		s, ok := v.(string)
		return s, ok && strings.TrimSpace(s) != ""
	case kindBool:
		b, ok := v.(bool)
		return b, ok
	case kindDate:
		return normalizeDate(v)
	case kindMoney:
		return normalizeMoney(v)
	case kindUUID:
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		_, err := uuid.Parse(s)
		return s, err == nil
	case kindEnum:
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		for _, allowed := range spec.values {
			if s == allowed {
				return s, true
			}
		}
		return nil, false
	}
	return nil, false
}

// NormalizePayload returns a cleaned copy of a type-specific payload plus the names of documented
// attributes that were rejected for having the wrong shape. Undocumented attributes are retained
// untouched so the taxonomy stays extensible (04_domain_model.md §5.1.2 design note).
// Candidate ingestion uses this lenient form so one malformed optional attribute never drops a transaction.
func NormalizePayload(t EventType, payload map[string]any) (map[string]any, []string) {
	clean := make(map[string]any, len(payload))
	var rejected []string
	specs := typePayloadSpecs[t]

	for key, val := range payload {
		spec, documented := specs[key]
		if !documented {
			clean[key] = val
			continue
		}
		norm, ok := normalizeAttribute(spec, val)
		if !ok {
			rejected = append(rejected, key)
			continue
		}
		clean[key] = norm
	}
	sort.Strings(rejected)
	return clean, rejected
}

// ValidatePayload is the strict form used for user-supplied (manual) events: any documented
// attribute with the wrong shape is an error.
func ValidatePayload(t EventType, payload map[string]any) (map[string]any, error) {
	if _, ok := typePayloadSpecs[t]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidEventType, t)
	}
	clean, rejected := NormalizePayload(t, payload)
	if len(rejected) > 0 {
		return nil, fmt.Errorf("%w: invalid attribute(s) %s for %s", ErrInvalidPayload, strings.Join(rejected, ", "), t)
	}
	return clean, nil
}
