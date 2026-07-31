// Package architecture guards module boundaries that are easy to violate with
// otherwise valid provider code.
package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestProviderBoundary(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	err := filepath.WalkDir(moduleRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != moduleRoot && filepath.Base(path) == "third_party" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			return nil
		}
		checkFile(t, moduleRoot, path, file)
		return nil
	})
	if err != nil {
		t.Fatalf("walk vectorstores module: %v", err)
	}
}

func checkFile(t *testing.T, moduleRoot, path string, file *ast.File) {
	t.Helper()

	relativePath := relative(moduleRoot, path)
	isProviderFile := strings.Contains(relativePath, "/") && !strings.HasPrefix(relativePath, "internal/")
	imports := make(map[string]string, len(file.Imports))
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Errorf("%s: unquote import: %v", relative(moduleRoot, path), err)
			continue
		}
		if strings.HasPrefix(importPath, "go.opentelemetry.io/otel") {
			t.Errorf("%s: provider modules must not import OpenTelemetry; decorate stores in the otel module", relativePath)
		}
		if isProviderFile && strings.HasPrefix(importPath, "github.com/Tangerg/lynx/vectorstores/") &&
			!strings.HasPrefix(importPath, "github.com/Tangerg/lynx/vectorstores/internal/") {
			t.Errorf("%s: provider packages must not depend on sibling provider %q", relativePath, importPath)
		}
		name := filepath.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = importPath
	}

	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.TypeSpec:
			if node.Name.Name == "StoreConfig" {
				checkStoreConfig(t, moduleRoot, path, imports, node)
			}
		case *ast.CallExpr:
			selector, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				break
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok || imports[pkg.Name] != "github.com/google/uuid" {
				break
			}
			if selector.Sel.Name == "New" || selector.Sel.Name == "NewString" {
				t.Errorf("%s: stores must preserve caller-assigned document IDs, not generate UUIDs", relative(moduleRoot, path))
			}
		}
		return true
	})
}

func checkStoreConfig(t *testing.T, moduleRoot, path string, imports map[string]string, spec *ast.TypeSpec) {
	t.Helper()

	structure, ok := spec.Type.(*ast.StructType)
	if !ok {
		return
	}
	for _, field := range structure.Fields.List {
		for _, name := range field.Names {
			if name.Name == "StoreDocumentContent" {
				t.Errorf("%s: writable stores must always persist text so Search can return valid documents", relative(moduleRoot, path))
			}
		}
		selector, ok := field.Type.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Context" {
			continue
		}
		pkg, ok := selector.X.(*ast.Ident)
		if ok && imports[pkg.Name] == "context" {
			t.Errorf("%s: StoreConfig must not retain context.Context; pass operation context explicitly", relative(moduleRoot, path))
		}
	}
}

func relative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
