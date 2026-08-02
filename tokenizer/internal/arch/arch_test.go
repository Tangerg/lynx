// Package arch contains architecture fitness tests for the tokenizer module.
package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/tokenizer"
)

func TestCapabilitiesRemainSmall(t *testing.T) {
	want := map[reflect.Type][]string{
		reflect.TypeFor[tokenizer.TextEstimator](): {"EstimateText"},
		reflect.TypeFor[tokenizer.Encoder]():       {"Encode"},
		reflect.TypeFor[tokenizer.Decoder]():       {"Decode"},
		reflect.TypeFor[tokenizer.Tokenizer]():     {"Decode", "Encode"},
	}
	for capability, methods := range want {
		if capability.NumMethod() != len(methods) {
			t.Errorf("%v has %d methods, want %d", capability, capability.NumMethod(), len(methods))
			continue
		}
		for index, method := range methods {
			if got := capability.Method(index).Name; got != method {
				t.Errorf("%v method %d = %s, want %s", capability, index, got, method)
			}
		}
	}
}

func TestProductionDependencyBoundary(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	for _, path := range productionGoFiles(t, root) {
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		relative, _ := filepath.Rel(root, path)
		for _, specification := range file.Imports {
			importPath := strings.Trim(specification.Path.Value, `"`)
			if isStandardImport(importPath) {
				continue
			}
			t.Errorf("tokenizer contract imports non-stdlib package %q: %s", importPath, relative)
		}
	}
}

func productionGoFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root {
				if entry.Name() == "vendor" || strings.HasPrefix(entry.Name(), ".") {
					return filepath.SkipDir
				}
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk tokenizer module: %v", err)
	}
	return files
}

func TestContractModuleHasNoRequirementsOrReplacements(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	contents := string(data)
	if strings.Contains(contents, "\nrequire ") || strings.Contains(contents, "\nrequire(") {
		t.Fatal("tokenizer contract module must not require another module")
	}
	if strings.Contains(contents, "\nreplace ") || strings.Contains(contents, "\nreplace(") {
		t.Fatal("tokenizer contract module must not contain replace directives")
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
