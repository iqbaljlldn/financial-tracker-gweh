package validator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/prompt"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/sanitizer"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

var (
	ErrPromptRegression = errors.New("candidate prompt version regressed on evaluation set [GATE]")
)

// EvalFixture models a held-out test fixture in test/fixtures/ai-eval-set/ (18_testing_strategy.md §8.1).
type EvalFixture struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email struct {
		Sender  string `json:"sender"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	} `json:"email"`
	Expected struct {
		EventType          string  `json:"eventType"`
		Amount             float64 `json:"amount"`
		Currency           string  `json:"currency"`
		MerchantNormalized string  `json:"merchantNormalized"`
		PaymentMethodLast4 string  `json:"paymentMethodLast4"`
	} `json:"expected"`
}

// EvalMetrics contains computed accuracy, validity, and calibration metrics for a prompt version (18_testing_strategy.md §8.2).
type EvalMetrics struct {
	TotalFixtures      int     `json:"totalFixtures"`
	SchemaValidCount   int     `json:"schemaValidCount"`
	SchemaValidityRate float64 `json:"schemaValidityRate"` // 0.0 - 1.0
	FieldAccuracyCount int     `json:"fieldAccuracyCount"` // total fields evaluated
	FieldMatchesCount  int     `json:"fieldMatchesCount"`  // matching fields
	FieldLevelAccuracy float64 `json:"fieldLevelAccuracy"` // 0.0 - 1.0
	CalibrationScore   float64 `json:"calibrationScore"`   // proxy: high confidence on correct fields
}

// LoadEvalFixtures reads all JSON fixtures from the given directory.
func LoadEvalFixtures(dirPath string) ([]EvalFixture, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading eval fixtures dir %s: %w", dirPath, err)
	}

	var fixtures []EvalFixture
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dirPath, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading fixture %s: %w", path, err)
		}
		var f EvalFixture
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("unmarshaling fixture %s: %w", path, err)
		}
		fixtures = append(fixtures, f)
	}

	if len(fixtures) == 0 {
		return nil, errors.New("no evaluation fixtures found in directory")
	}

	return fixtures, nil
}

// EvaluatePromptVersion runs the prompt version against the evaluation set and calculates metrics (18_testing_strategy.md §8.2).
func EvaluatePromptVersion(
	ctx context.Context,
	pv *prompt.PromptVersion,
	extractor domain.LLMExtractor,
	fixtures []EvalFixture,
) (*EvalMetrics, error) {
	metrics := &EvalMetrics{
		TotalFixtures: len(fixtures),
	}

	totalFields := 0
	correctFields := 0
	calibratedSum := 0.0

	for _, f := range fixtures {
		// 1. Sanitize email
		fullContent := fmt.Sprintf("Subject: %s\n\n%s", f.Email.Subject, f.Email.Body)
		sanitized := sanitizer.SanitizeEmail(fullContent)

		// 2. Build prompt
		messages := pv.BuildExtractionPrompt(sanitized)

		// 3. Extract
		resp, err := extractor.Extract(ctx, domain.ExtractionPromptRequest{
			Messages:       messages,
			ResponseFormat: "json_object",
			Temperature:    0.0,
		})
		if err != nil {
			continue
		}

		// 4. Validate schema
		rawOut, err := prompt.ValidateRawOutput(resp.RawJSON)
		if err != nil {
			continue // schema invalid on first attempt
		}
		metrics.SchemaValidCount++

		// 5. Evaluate field-level accuracy
		// Field 1: eventType
		totalFields++
		if strings.EqualFold(strings.TrimSpace(rawOut.EventType), strings.TrimSpace(f.Expected.EventType)) {
			correctFields++
			calibratedSum += 1.0
		}

		// Field 2: amount
		totalFields++
		parsedAmt, err := prompt.ParseAmountValue(rawOut.Amount.Value)
		if err == nil && math.Abs(parsedAmt-f.Expected.Amount) < 0.01 {
			correctFields++
			calibratedSum += rawOut.Amount.Confidence
		}

		// Field 3: currency
		totalFields++
		if strings.EqualFold(strings.TrimSpace(rawOut.Amount.Currency), strings.TrimSpace(f.Expected.Currency)) {
			correctFields++
			calibratedSum += 1.0
		}

		// Field 4: merchantNormalized
		totalFields++
		normMerch := strings.ToLower(strings.TrimSpace(rawOut.Merchant.NormalizedName))
		expMerch := strings.ToLower(strings.TrimSpace(f.Expected.MerchantNormalized))
		if normMerch != "" && (strings.Contains(normMerch, expMerch) || strings.Contains(expMerch, normMerch)) {
			correctFields++
			calibratedSum += rawOut.Merchant.Confidence
		}
	}

	metrics.FieldAccuracyCount = totalFields
	metrics.FieldMatchesCount = correctFields

	if metrics.TotalFixtures > 0 {
		metrics.SchemaValidityRate = float64(metrics.SchemaValidCount) / float64(metrics.TotalFixtures)
	}
	if totalFields > 0 {
		metrics.FieldLevelAccuracy = float64(correctFields) / float64(totalFields)
		metrics.CalibrationScore = calibratedSum / float64(totalFields)
	}

	return metrics, nil
}

// CompareAndGate checks whether candidatePrompt regressed against baselinePrompt (18_testing_strategy.md §8.3) [GATE].
func CompareAndGate(baseline, candidate *EvalMetrics) error {
	// Tolerance of 0.001 to handle float rounding
	if candidate.SchemaValidityRate < baseline.SchemaValidityRate-0.001 {
		return fmt.Errorf("%w: schema validity rate regressed (baseline: %.3f, candidate: %.3f)",
			ErrPromptRegression, baseline.SchemaValidityRate, candidate.SchemaValidityRate)
	}

	if candidate.FieldLevelAccuracy < baseline.FieldLevelAccuracy-0.001 {
		return fmt.Errorf("%w: field-level accuracy regressed (baseline: %.3f, candidate: %.3f)",
			ErrPromptRegression, baseline.FieldLevelAccuracy, candidate.FieldLevelAccuracy)
	}

	return nil
}
