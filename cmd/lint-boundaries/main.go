// Package main implements the custom module-boundary import linter
// enforcing the Clean Architecture layers and bounded-context separation
// specified in 06_architecture.md §4-5, 10_backend_blueprint.md §3, and 21_project_structure.md §5.
package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const modulePrefix = "github.com/iqbaljlldn/financial-tracker-gweh/internal/"

var domainModules = map[string]bool{
	"identity":       true,
	"ingestion":      true,
	"understanding":  true,
	"financialcore":  true,
	"budget":         true,
	"analytics":      true,
	"engagement":     true,
	"administration": true,
}

var sharedPackages = map[string]bool{
	"platform": true,
	"apitypes": true,
}

type Violation struct {
	File     string
	Line     int
	Imported string
	Message  string
}

func main() {
	var rootDir string
	flag.StringVar(&rootDir, "dir", ".", "Repository root directory to inspect")
	flag.Parse()

	internalDir := filepath.Join(rootDir, "internal")
	if _, err := os.Stat(internalDir); os.IsNotExist(err) {
		fmt.Printf("No internal/ directory found in %s, skipping boundary linting\n", rootDir)
		os.Exit(0)
	}

	violations, err := CheckBoundaries(internalDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error linting boundaries: %v\n", err)
		os.Exit(2)
	}

	if len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "Found %d module boundary violation(s):\n\n", len(violations))
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "  %s:%d: %s\n    Imported: %s\n\n", v.File, v.Line, v.Message, v.Imported)
		}
		os.Exit(1)
	}

	fmt.Println("Module-boundary and Clean Architecture import checks passed: 0 violations.")
}

// CheckBoundaries scans Go source files inside the internal directory and checks for import violations.
func CheckBoundaries(internalDir string) ([]Violation, error) {
	var violations []Violation
	fset := token.NewFileSet()

	err := filepath.WalkDir(internalDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		rel, err := filepath.Rel(internalDir, path)
		if err != nil {
			return err
		}

		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) == 0 {
			return nil
		}
		callerModule := parts[0]

		callerLayer := ""
		if len(parts) > 1 {
			callerLayer = parts[1] // domain, application, adapter, etc.
		}

		fileNode, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}

		for _, imp := range fileNode.Imports {
			if v := inspectImport(path, fset.Position(imp.Pos()).Line, imp.Path.Value, callerModule, callerLayer); v != nil {
				violations = append(violations, *v)
			}
		}

		return nil
	})

	return violations, err
}

func inspectImport(filePath string, line int, rawImport, callerModule, callerLayer string) *Violation {
	importPath := strings.Trim(rawImport, `"`)
	if !strings.HasPrefix(importPath, modulePrefix) {
		return nil
	}

	subPath := strings.TrimPrefix(importPath, modulePrefix)
	subParts := strings.Split(subPath, "/")
	if len(subParts) == 0 {
		return nil
	}
	targetModule := subParts[0]

	targetLayer := ""
	if len(subParts) > 1 {
		targetLayer = subParts[1]
	}

	// Rule 1: Shared platform cannot import domain modules
	if callerModule == "platform" && domainModules[targetModule] {
		return &Violation{
			File:     filePath,
			Line:     line,
			Imported: importPath,
			Message:  "Platform shared code cannot import domain module (21_project_structure.md §5.1: zero domain logic rule)",
		}
	}

	// Rule 2: Allowed imports into internal/platform or internal/apitypes
	if sharedPackages[targetModule] {
		return nil
	}

	// Rule 3: Cross-module imports between domain modules
	if v := checkCrossModule(filePath, line, importPath, callerModule, targetModule, targetLayer); v != nil {
		return v
	}

	// Rule 4: Intra-module Clean Architecture dependency rule
	return checkIntraModule(filePath, line, importPath, callerModule, targetModule, callerLayer, targetLayer)
}

// Sanctioned cross-module interface imports per 10_backend_blueprint.md §6:
// Only domain interfaces may be imported by a consuming module's application or adapter layer.
var sanctionedCrossModuleImports = map[string]map[string]bool{
	"ingestion": {
		"identity": true, // TokenProvider (10_backend_blueprint.md §6)
	},
	"engagement": {
		"analytics": true, // AnalyticsSnapshotReader (10_backend_blueprint.md §6)
	},
}

func checkCrossModule(filePath string, line int, importPath, callerModule, targetModule, targetLayer string) *Violation {
	if domainModules[callerModule] && domainModules[targetModule] && callerModule != targetModule {
		// Check if this is a sanctioned cross-module domain interface import per 10_backend_blueprint.md §6
		if sanctionedCrossModuleImports[callerModule] != nil && sanctionedCrossModuleImports[callerModule][targetModule] {
			if targetLayer == "domain" {
				return nil // Sanctioned interface import allowed
			}
		}

		return &Violation{
			File:     filePath,
			Line:     line,
			Imported: importPath,
			Message: fmt.Sprintf("Forbidden cross-module import: module %q cannot import module %q (%s) per 06_architecture.md §5.2",
				callerModule, targetModule, targetLayer),
		}
	}
	return nil
}

func checkIntraModule(filePath string, line int, importPath, callerModule, targetModule, callerLayer, targetLayer string) *Violation {
	if callerModule != targetModule {
		return nil
	}
	if callerLayer == "domain" && (targetLayer == "application" || targetLayer == "adapter") {
		return &Violation{
			File:     filePath,
			Line:     line,
			Imported: importPath,
			Message: fmt.Sprintf("Clean Architecture violation: %s/domain cannot import %s/%s (10_backend_blueprint.md §3)",
				callerModule, targetModule, targetLayer),
		}
	}
	if callerLayer == "application" && targetLayer == "adapter" {
		return &Violation{
			File:     filePath,
			Line:     line,
			Imported: importPath,
			Message: fmt.Sprintf("Clean Architecture violation: %s/application cannot import %s/adapter (10_backend_blueprint.md §3)",
				callerModule, targetModule),
		}
	}
	return nil
}
