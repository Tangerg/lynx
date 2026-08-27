package arch

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

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

var allowedExternalProductionImports = map[string]struct{}{
	"github.com/samber/lo": {},
}

func TestTargetPackagesStayWithinDependencyBudget(t *testing.T) {
	root := coreRoot(t)
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
			if isAllowedProductionImport(importPath) {
				continue
			}
			t.Errorf("target package %s imports %q outside the production dependency budget in %s", budgetRoot, importPath, relativePath)
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

func TestCoreProductionDependenciesMatchAllowlist(t *testing.T) {
	fset := token.NewFileSet()
	for _, path := range productionGoFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if isAllowedProductionImport(importPath) {
				continue
			}
			rel, _ := filepath.Rel(coreRoot(t), path)
			t.Errorf("core production import %q in %s is outside the explicit dependency allowlist", importPath, rel)
		}
	}
}

func isAllowedProductionImport(importPath string) bool {
	if strings.HasPrefix(importPath, "github.com/Tangerg/lynx/core") || isStandardImport(importPath) {
		return true
	}
	_, ok := allowedExternalProductionImports[importPath]
	return ok
}

func isStandardImport(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}
