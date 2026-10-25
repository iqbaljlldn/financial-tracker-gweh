package prompt_test

import (
	"strings"
	"testing"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/prompt"
)

func TestPromptRegistry_ImmutabilityAndRollback(t *testing.T) {
	reg := prompt.NewRegistry()

	// Default active version should be v1
	if reg.ActiveVersionID() != "v1" {
		t.Fatalf("expected active version v1, got %s", reg.ActiveVersionID())
	}

	// Registering v1 again must fail [GATE - Immutability]
	err := reg.Register(prompt.GetV1PromptVersion())
	if err == nil {
		t.Fatal("expected error registering duplicate prompt version, got nil")
	}

	// Register candidate version v2
	v2 := &prompt.PromptVersion{
		VersionID:         "v2",
		SystemInstruction: "v2 system instruction",
		SchemaDefinition:  "v2 schema",
	}
	if err := reg.Register(v2); err != nil {
		t.Fatalf("unexpected error registering v2: %v", err)
	}

	// Activate v2
	if err := reg.ActivateVersion("v2"); err != nil {
		t.Fatalf("failed to activate v2: %v", err)
	}
	if reg.ActiveVersionID() != "v2" {
		t.Fatalf("expected active version v2, got %s", reg.ActiveVersionID())
	}

	// Rollback to v1 [GATE]
	if err := reg.ActivateVersion("v1"); err != nil {
		t.Fatalf("failed to rollback to v1: %v", err)
	}
	if reg.ActiveVersionID() != "v1" {
		t.Fatalf("expected rollback to v1, got %s", reg.ActiveVersionID())
	}
}

func TestPromptVersion_BuildExtractionPrompt(t *testing.T) {
	pv := prompt.GetV1PromptVersion()
	messages := pv.BuildExtractionPrompt("Order total $20 at Uber")

	if len(messages) < 4 { // system + few-shots (at least 2) + current user message
		t.Fatalf("expected at least 4 messages, got %d", len(messages))
	}

	// Check system message contains both system instruction and schema definition
	if !strings.Contains(messages[0].Content, "CONFIDENCE RUBRIC") {
		t.Errorf("system message missing confidence rubric")
	}
	if !strings.Contains(messages[0].Content, "REQUIRED JSON SCHEMA") {
		t.Errorf("system message missing schema definition")
	}

	// Last message is the current sanitized email content
	lastMsg := messages[len(messages)-1]
	if lastMsg.Role != "user" || !strings.Contains(lastMsg.Content, "Order total $20 at Uber") {
		t.Errorf("expected last message to contain email content, got %v", lastMsg)
	}
}

func TestValidateRawOutput_Validation(t *testing.T) {
	validJSON := `{
  "eventType": "Expense",
  "amount": { "value": "45.00", "currency": "USD", "confidence": 0.95 },
  "occurredAt": { "value": "2026-08-03T14:22:00Z", "confidence": 0.90 },
  "merchant": { "rawName": "AMZN Mktp US", "normalizedName": "Amazon", "confidence": 0.85 },
  "paymentMethod": { "type": "card", "hint": "4821", "confidence": 0.70 },
  "categoryHint": { "value": "Shopping", "confidence": 0.60 },
  "extractionNotes": "Clean extraction."
}`

	// Valid directly
	out, err := prompt.ValidateRawOutput(validJSON)
	if err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if out.EventType != "Expense" {
		t.Errorf("expected eventType Expense, got %s", out.EventType)
	}

	// Valid inside markdown fences
	wrapped := "```json\n" + validJSON + "\n```"
	outWrapped, err := prompt.ValidateRawOutput(wrapped)
	if err != nil {
		t.Fatalf("expected valid JSON inside code fences, got error: %v", err)
	}
	if outWrapped.Amount.Currency != "USD" {
		t.Errorf("expected currency USD, got %s", outWrapped.Amount.Currency)
	}

	// Invalid JSON syntax
	_, err = prompt.ValidateRawOutput("not json at all")
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}

	// Missing amount
	invalidAmount := `{
  "eventType": "Expense",
  "amount": { "value": -10, "currency": "USD", "confidence": 0.95 },
  "occurredAt": { "value": "2026-08-03T14:22:00Z", "confidence": 0.90 }
}`
	_, err = prompt.ValidateRawOutput(invalidAmount)
	if err == nil {
		t.Fatal("expected error on negative amount, got nil")
	}
}
