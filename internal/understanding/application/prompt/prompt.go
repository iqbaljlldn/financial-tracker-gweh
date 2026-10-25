package prompt

import (
	"fmt"
	"strings"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// PromptVersion represents an immutable prompt configuration version (11_ai_blueprint.md §4).
type PromptVersion struct {
	VersionID         string
	SystemInstruction string
	SchemaDefinition  string
	FewShotExamples   []FewShotExample
}

// FewShotExample represents an input email and its corresponding target JSON output.
type FewShotExample struct {
	EmailContent string
	TargetJSON   string
}

// BuildExtractionPrompt composes the 4 sections into a list of LLMMessages for the chat completions API.
func (pv *PromptVersion) BuildExtractionPrompt(sanitizedEmail string) []domain.LLMMessage {
	// 1. System Instruction & Schema Definition
	systemContent := fmt.Sprintf("%s\n\n### REQUIRED JSON SCHEMA:\n%s",
		strings.TrimSpace(pv.SystemInstruction),
		strings.TrimSpace(pv.SchemaDefinition),
	)

	messages := []domain.LLMMessage{
		{Role: "system", Content: systemContent},
	}

	// 2. Few-Shot Examples (input -> assistant response)
	for _, ex := range pv.FewShotExamples {
		messages = append(messages,
			domain.LLMMessage{Role: "user", Content: fmt.Sprintf("Extract financial transaction:\n\n%s", ex.EmailContent)},
			domain.LLMMessage{Role: "assistant", Content: ex.TargetJSON},
		)
	}

	// 3. Current sanitized email content
	messages = append(messages, domain.LLMMessage{
		Role:    "user",
		Content: fmt.Sprintf("Extract financial transaction:\n\n%s", sanitizedEmail),
	})

	return messages
}

// BuildRePromptMessage creates the single corrective re-prompt message on schema validation failure (11_ai_blueprint.md §5.1).
func BuildRePromptMessage(validationErr string) domain.LLMMessage {
	return domain.LLMMessage{
		Role: "user",
		Content: fmt.Sprintf(
			"Your previous response was not valid JSON matching the required schema (Error: %s). Return ONLY strictly valid JSON, no markdown code fences, no explanatory text outside the JSON structure.",
			validationErr,
		),
	}
}
