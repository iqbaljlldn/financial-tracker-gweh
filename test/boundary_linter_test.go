package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBoundaryLinterFailsOnDeliberatelyBadImport demonstrably verifies Exit Criterion 4:
// "Module-boundary linter is demonstrably active (test it with a deliberately-bad import, confirm it fails)"
func TestBoundaryLinterFailsOnDeliberatelyBadImport(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "deliberate-bad-import-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// 1. Create a bad import scenario in a dummy internal tree
	badFileDir := filepath.Join(tempDir, "internal", "financialcore", "application")
	if err := os.MkdirAll(badFileDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	badFile := filepath.Join(badFileDir, "bad_handler.go")
	badContent := `package application

// Deliberately-bad cross-module import: financialcore importing budget's domain package
import "github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"

type IllegalHandler struct {
	B budget_domain.Budget
}
`
	if err := os.WriteFile(badFile, []byte(badContent), 0644); err != nil {
		t.Fatalf("failed to write bad file: %v", err)
	}

	// 2. Run the boundary linter on this directory
	cmd := exec.Command("go", "run", "./cmd/lint-boundaries", "-dir", tempDir)
	// Set working directory to repository root so go run can find cmd/lint-boundaries
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()

	// 3. Confirm that the linter FAILS (exit code non-zero)
	if err == nil {
		t.Fatalf("expected module-boundary linter to fail on bad import, but it succeeded! Output:\n%s", string(out))
	}

	outStr := string(out)
	if !strings.Contains(outStr, "Forbidden cross-module import") {
		t.Errorf("expected output to mention 'Forbidden cross-module import', got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain") {
		t.Errorf("expected output to show the offending package import, got:\n%s", outStr)
	}

	t.Logf("Linter successfully caught deliberate cross-module violation:\n%s", outStr)
}
