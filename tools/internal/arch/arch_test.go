package arch_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRootToolsOwnOnlyCollections(t *testing.T) {
	root := toolsRoot(t)
	allowedExports := map[string]bool{
		"ErrDuplicateTool":   true,
		"ErrInvalidRegistry": true,
		"NewRegistry":        true,
		"Registry":           true,
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(root, name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if strings.HasPrefix(importPath, "github.com/Tangerg/lynx/") &&
				importPath != "github.com/Tangerg/lynx/core/chat" &&
				importPath != "github.com/Tangerg/lynx/core/tool" {
				t.Errorf("root tools production file %s imports non-collection dependency %s", name, importPath)
			}
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if declaration.Recv == nil && ast.IsExported(declaration.Name.Name) && !allowedExports[declaration.Name.Name] {
					t.Errorf("root tools exports non-collection function %s", declaration.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range declaration.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						if ast.IsExported(spec.Name.Name) && !allowedExports[spec.Name.Name] {
							t.Errorf("root tools exports non-collection type %s", spec.Name.Name)
						}
					case *ast.ValueSpec:
						for _, identifier := range spec.Names {
							if ast.IsExported(identifier.Name) && !allowedExports[identifier.Name] {
								t.Errorf("root tools exports non-collection value %s", identifier.Name)
							}
						}
					}
				}
			}
		}
	}
}

func toolsRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller did not report test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
