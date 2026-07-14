package arch

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

var temporaryExternalImports = map[string]string{
	"github.com/invopop/jsonschema": "P6-05",
}

var dependencyBudgetPackageRoots = []string{
	"chat",
	"document",
	"embedding",
	"image",
	"media",
	"metadata",
	"moderation",
	"speech",
	"transcription",
	"vectorstore",
}

func TestTargetPackagesHaveNoExternalDependencies(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	seen := make(map[string]bool, len(dependencyBudgetPackageRoots))
	for _, path := range productionGoFiles(t) {
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("make %s relative to module root: %v", path, err)
		}
		packagePath := filepath.ToSlash(filepath.Dir(relativePath))
		budgetRoot, ok := dependencyBudgetRoot(packagePath)
		if !ok {
			continue
		}
		seen[budgetRoot] = true

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if isStandardImport(importPath) || importPath == "github.com/Tangerg/lynx/core" || strings.HasPrefix(importPath, "github.com/Tangerg/lynx/core/") {
				continue
			}
			t.Errorf("target package %s has external production import %q in %s", budgetRoot, importPath, relativePath)
		}
	}
	for _, packageRoot := range dependencyBudgetPackageRoots {
		if !seen[packageRoot] {
			t.Errorf("dependency-budget package root %s has no production files", packageRoot)
		}
	}
}

func dependencyBudgetRoot(packagePath string) (string, bool) {
	for _, root := range dependencyBudgetPackageRoots {
		if packagePath == root || strings.HasPrefix(packagePath, root+"/") {
			return root, true
		}
	}
	return "", false
}

func TestExternalImportsDoNotExceedMigrationBudget(t *testing.T) {
	fset := token.NewFileSet()
	for _, path := range productionGoFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if strings.HasPrefix(importPath, "github.com/Tangerg/lynx/core") || isStandardImport(importPath) {
				continue
			}
			deadline, ok := temporaryExternalImports[importPath]
			if !ok {
				rel, _ := filepath.Rel(moduleRoot(t), path)
				t.Errorf("external import %q in %s is outside the migration budget", importPath, rel)
				continue
			}
			t.Logf("temporary external import %s remains until %s", importPath, deadline)
		}
	}
}

func isStandardImport(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}
