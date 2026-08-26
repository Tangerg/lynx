// Package arch holds architecture-fitness tests for the embeddingclient package.
package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/embeddingclient"
)

func TestClientSurfaceStaysVectorFocused(t *testing.T) {
	typeOfClient := reflect.TypeFor[embeddingclient.Client]()
	methods := make([]string, 0, typeOfClient.NumMethod())
	for i := range typeOfClient.NumMethod() {
		methods = append(methods, typeOfClient.Method(i).Name)
	}
	slices.Sort(methods)
	if !slices.Equal(methods, []string{"Dimensions", "EmbedDocuments", "EmbedText", "EmbedTexts"}) {
		t.Fatalf("Client methods = %v", methods)
	}
}

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
			relative, _ := filepath.Rel(packageRoot(t), path)
			t.Errorf("embeddingclient production import %q is outside stdlib + Core boundary: %s", importPath, relative)
		}
	}
}

func isStandardImport(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}

func productionGoFiles(t *testing.T) []string {
	t.Helper()
	root := packageRoot(t)
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

func packageRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
