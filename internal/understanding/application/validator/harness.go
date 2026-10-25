package validator

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/executor"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// ExpectedExtraction defines exact-match assertion targets per sample email (13_email_parser.md §6.1).
type ExpectedExtraction struct {
	EventType     string                 `json:"eventType"`
	Amount        float64                `json:"amount"`
	Currency      string                 `json:"currency"`
	Merchant      string                 `json:"merchant"`
	OccurredAt    string                 `json:"occurredAt"` // RFC3339
	PaymentMethod string                 `json:"paymentMethod,omitempty"`
	TypeSpecific  map[string]interface{} `json:"typeSpecific,omitempty"`
}

// SampleEmail represents an anonymized real email fixture and its expected extraction.
type SampleEmail struct {
	ID       string                 `json:"id"`
	RawEmail domain.RawEmailMessage `json:"rawEmail"`
	Expected ExpectedExtraction     `json:"expected"`
}

// SampleSet represents a collection of sample emails for a specific senderPattern.
type SampleSet struct {
	SenderPattern string        `json:"senderPattern"`
	Samples       []SampleEmail `json:"samples"`
}

// SampleFailure captures exact discrepancies when a sample fails validation.
type SampleFailure struct {
	SampleID string `json:"sampleId"`
	Reason   string `json:"reason"`
	Field    string `json:"field,omitempty"`
	Expected string `json:"expected,omitempty"`
	Got      string `json:"got,omitempty"`
}

// ValidationReport summarizes the result of executing a template against a sample set.
type ValidationReport struct {
	SenderPattern string          `json:"senderPattern"`
	Version       int             `json:"version"`
	TotalSamples  int             `json:"totalSamples"`
	PassedSamples int             `json:"passedSamples"`
	FailedSamples int             `json:"failedSamples"`
	Passed        bool            `json:"passed"`
	Failures      []SampleFailure `json:"failures,omitempty"`
}

// ValidationHarness executes sample-set regression validation [GATE].
type ValidationHarness struct {
	exec *executor.ExtractionExecutor
}

// NewValidationHarness creates a new ValidationHarness.
func NewValidationHarness() *ValidationHarness {
	return &ValidationHarness{
		exec: executor.NewExtractionExecutor(),
	}
}

// ValidateTemplate runs a SenderTemplate against its sample-set fixture.
// All sample emails must produce exact-match extractions for the template to pass [GATE].
func (h *ValidationHarness) ValidateTemplate(tmpl *domain.SenderTemplate, sampleSet *SampleSet) *ValidationReport {
	report := &ValidationReport{
		SenderPattern: tmpl.SenderPattern(),
		Version:       tmpl.Version(),
		TotalSamples:  len(sampleSet.Samples),
		Passed:        true,
	}

	for _, sample := range sampleSet.Samples {
		res := h.exec.Execute(tmpl, &sample.RawEmail, "eval-user-id")
		if res.Outcome == executor.OutcomeFailed || res.Candidate == nil {
			report.Passed = false
			report.FailedSamples++
			report.Failures = append(report.Failures, SampleFailure{
				SampleID: sample.ID,
				Reason:   fmt.Sprintf("extraction failed: %v, missing fields: %v", res.Err, res.MissingFields),
			})
			continue
		}

		// Exact-match assertions per 13_email_parser.md §6.1
		cand := res.Candidate
		exp := sample.Expected

		var fieldFailures []SampleFailure

		// 1. Amount
		if math.Abs(cand.Amount.Value-exp.Amount) > 0.001 {
			fieldFailures = append(fieldFailures, SampleFailure{
				SampleID: sample.ID,
				Field:    "amount",
				Reason:   "amount mismatch",
				Expected: fmt.Sprintf("%f", exp.Amount),
				Got:      fmt.Sprintf("%f", cand.Amount.Value),
			})
		}

		// 2. Currency
		if exp.Currency != "" && !strings.EqualFold(cand.Amount.Currency, exp.Currency) {
			fieldFailures = append(fieldFailures, SampleFailure{
				SampleID: sample.ID,
				Field:    "currency",
				Reason:   "currency mismatch",
				Expected: exp.Currency,
				Got:      cand.Amount.Currency,
			})
		}

		// 3. Merchant
		if exp.Merchant != "" && !strings.EqualFold(cand.Merchant.NormalizedName, exp.Merchant) {
			fieldFailures = append(fieldFailures, SampleFailure{
				SampleID: sample.ID,
				Field:    "merchant",
				Reason:   "merchant normalized name mismatch",
				Expected: exp.Merchant,
				Got:      cand.Merchant.NormalizedName,
			})
		}

		// 4. EventType
		if exp.EventType != "" && cand.EventType != exp.EventType {
			fieldFailures = append(fieldFailures, SampleFailure{
				SampleID: sample.ID,
				Field:    "eventType",
				Reason:   "eventType mismatch",
				Expected: exp.EventType,
				Got:      cand.EventType,
			})
		}

		// 5. OccurredAt
		if exp.OccurredAt != "" {
			expectedTime, err := time.Parse(time.RFC3339, exp.OccurredAt)
			if err == nil && !cand.OccurredAt.Equal(expectedTime) {
				fieldFailures = append(fieldFailures, SampleFailure{
					SampleID: sample.ID,
					Field:    "occurredAt",
					Reason:   "occurredAt timestamp mismatch",
					Expected: exp.OccurredAt,
					Got:      cand.OccurredAt.Format(time.RFC3339),
				})
			}
		}

		if len(fieldFailures) > 0 {
			report.Passed = false
			report.FailedSamples++
			report.Failures = append(report.Failures, fieldFailures...)
		} else {
			report.PassedSamples++
		}
	}

	return report
}
