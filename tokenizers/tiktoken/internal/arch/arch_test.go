// Package arch contains architecture fitness tests for the tiktoken adapter module.
package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionDependencyBoundary(t *testing.T) {
	allowed := map[string]bool{
		"github.com/Tangerg/lynx/core/tokenizer": true,
		"github.com/pkoukk/tiktoken-go":          true,
	}
	root := moduleRoot(t)
	fset := token.NewFileSet()
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == "vendor" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, specification := range file.Imports {
			importPath := strings.Trim(specification.Path.Value, `"`)
			if !isStandardImport(importPath) && !allowed[importPath] {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("tiktoken production import %q is outside its adapter boundary: %s", importPath, relative)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("walk tiktoken module: %v", err)
	}
}

func TestModuleHasNoReplaceDirective(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\nreplace ") || strings.Contains(string(data), "\nreplace(") {
		t.Fatal("tiktoken go.mod must not contain replace directives")
	}
}

func isStandardImport(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("go.mod not found")
		}
		directory = parent
	}
}
