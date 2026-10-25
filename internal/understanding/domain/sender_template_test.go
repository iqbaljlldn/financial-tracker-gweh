package domain_test

import (
	"testing"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

func TestSenderTemplate_Matches(t *testing.T) {
	tmpl, err := domain.NewSenderTemplate(
		"tmpl-1",
		"*@bca.co.id",
		1,
		domain.ExtractionRules{},
		true,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("unexpected error creating template: %v", err)
	}

	positiveCases := []string{
		"e-statement@bca.co.id",
		"kartukredit@bca.co.id",
		"HaloBCA <halobca@bca.co.id>",
		"NOTIFIKASI@BCA.CO.ID",
	}

	for _, c := range positiveCases {
		if !tmpl.Matches(c) {
			t.Errorf("expected pattern %q to match %q", tmpl.SenderPattern(), c)
		}
	}

	negativeCases := []string{
		"alert@fake-bca.com",
		"user@gmail.com",
		"no-reply@mandiri.co.id",
		"",
	}

	for _, c := range negativeCases {
		if tmpl.Matches(c) {
			t.Errorf("expected pattern %q NOT to match %q", tmpl.SenderPattern(), c)
		}
	}
}

func TestSenderTemplate_WildcardSubdomain(t *testing.T) {
	tmpl, _ := domain.NewSenderTemplate(
		"tmpl-2",
		"*.notifications@examplebank.com",
		1,
		domain.ExtractionRules{},
		true,
		time.Now(),
	)

	if !tmpl.Matches("alerts.notifications@examplebank.com") {
		t.Errorf("expected match for subdomain wildcard")
	}
	if tmpl.Matches("other@examplebank.com") {
		t.Errorf("expected no match for other@examplebank.com")
	}
}
