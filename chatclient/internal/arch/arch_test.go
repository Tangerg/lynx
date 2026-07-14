// Package arch holds architecture-fitness tests for the chatclient module.
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

func TestProductionImportsOnlyStandardLibraryAndCore(t *testing.T) {
	fset := token.NewFileSet()
	for _, path := range productionGoFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", path, err)
		}
		for _, specification := range file.Imports {
			importPath := strings.Trim(specification.Path.Value, `"`)
			if isStandardImport(importPath) || importPath == "github.com/Tangerg/lynx/core" || strings.HasPrefix(importPath, "github.com/Tangerg/lynx/core/") {
				continue
			}
			relative, _ := filepath.Rel(moduleRoot(t), path)
			t.Errorf("chatclient production import %q is outside stdlib + Core boundary: %s", importPath, relative)
		}
	}
}

func isStandardImport(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}

func productionGoFiles(t *testing.T) []string {
	t.Helper()
	root := moduleRoot(t)
	var files []string
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
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk module: %v", err)
	}
	return files
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
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
