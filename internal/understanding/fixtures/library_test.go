package fixtures_test

import (
	"testing"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/validator"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/fixtures"
)

// TestCuratedTemplates_SampleSetValidation validates that each curated template passes
// exact-match sample-set evaluation before it can ever be published or marked active [GATE].
func TestCuratedTemplates_SampleSetValidation(t *testing.T) {
	harness := validator.NewValidationHarness()
	templates := fixtures.GetCuratedTemplates()
	sampleSets := fixtures.GetSampleSets()

	if len(templates) < 5 {
		t.Fatalf("expected at least 5 curated templates, got: %d", len(templates))
	}

	for _, tmpl := range templates {
		sampleSet, exists := sampleSets[tmpl.SenderPattern()]
		if !exists {
			t.Fatalf("missing sample-set fixture for curated template: %s", tmpl.SenderPattern())
		}

		report := harness.ValidateTemplate(tmpl, sampleSet)
		if !report.Passed {
			t.Errorf("template %s (v%d) FAILED sample-set validation: %d failed of %d samples. Discrepancies:",
				tmpl.SenderPattern(), tmpl.Version(), report.FailedSamples, report.TotalSamples)
			for _, f := range report.Failures {
				t.Errorf("  - Sample %s [%s]: %s (Expected: %s, Got: %s)", f.SampleID, f.Field, f.Reason, f.Expected, f.Got)
			}
		} else {
			t.Logf("PASS: Template %s (v%d) validated against %d sample(s)", tmpl.SenderPattern(), tmpl.Version(), report.TotalSamples)
		}
	}
}
