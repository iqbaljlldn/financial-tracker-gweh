package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckBoundaries_Violations(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "boundary-linter-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create test tree:
	// internal/
	//   identity/domain/user.go
	//   budget/application/evaluate.go (imports identity/domain -> forbidden cross-module!)
	//   financialcore/domain/event.go (imports financialcore/adapter -> forbidden Clean Arch!)
	//   financialcore/adapter/repo.go
	//   financialcore/application/command.go (imports apitypes -> allowed!)

	writeGoFile := func(subPath, content string) {
		fullPath := filepath.Join(tempDir, subPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to mkdir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	writeGoFile("identity/domain/user.go", `package domain
type User struct{ ID string }
`)

	writeGoFile("financialcore/adapter/repo.go", `package adapter
type Repo struct{}
`)

	writeGoFile("budget/application/evaluate.go", `package application
import "github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
type Evaluator struct{ U domain.User }
`)

	writeGoFile("financialcore/domain/event.go", `package domain
import "github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/adapter"
type Event struct{ R adapter.Repo }
`)

	writeGoFile("financialcore/application/command.go", `package application
import "github.com/iqbaljlldn/financial-tracker-gweh/internal/apitypes"
type Cmd struct{ M apitypes.Money }
`)

	violations, err := CheckBoundaries(tempDir)
	if err != nil {
		t.Fatalf("unexpected error checking boundaries: %v", err)
	}

	if len(violations) != 2 {
		t.Fatalf("expected exactly 2 violations, got %d: %+v", len(violations), violations)
	}

	// Verify that budget importing identity was caught
	foundCrossModule := false
	foundCleanArch := false
	for _, v := range violations {
		if v.Imported == "github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain" {
			foundCrossModule = true
		}
		if v.Imported == "github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/adapter" {
			foundCleanArch = true
		}
	}

	if !foundCrossModule {
		t.Errorf("did not detect cross-module import violation")
	}
	if !foundCleanArch {
		t.Errorf("did not detect Clean Architecture violation (domain importing adapter)")
	}
}

func TestCheckBoundaries_CleanRepo(t *testing.T) {
	// Running on the current repository's internal/ directory should produce 0 violations
	violations, err := CheckBoundaries("../../internal")
	if err != nil {
		t.Fatalf("unexpected error checking boundaries on internal/: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations in clean repo, got %d: %+v", len(violations), violations)
	}
}
