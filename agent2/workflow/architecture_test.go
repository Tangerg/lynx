package workflow

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWorkflowExcludesHostLegacyAndSecondRuntimeAbstractions(t *testing.T) {
	forbiddenNames := map[string]bool{
		"Store": true, "Journal": true, "Registry": true, "Graph": true,
		"Scheduler": true, "Repository": true, "Transaction": true, "Lease": true,
	}
	forbiddenFragments := []string{"Session", "Conversation", "Workspace", "WriteSet"}
	files := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(files, path, nil, 0)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if importPath == "github.com/Tangerg/flow" || strings.HasPrefix(importPath, "github.com/Tangerg/flow/") ||
				importPath == "github.com/Tangerg/lynx/agent" || strings.HasPrefix(importPath, "github.com/Tangerg/lynx/agent/") ||
				importPath == "github.com/Tangerg/lynx/app" || strings.HasPrefix(importPath, "github.com/Tangerg/lynx/app/") ||
				strings.HasPrefix(importPath, "go.opentelemetry.io/") || importPath == "log/slog" {
				t.Errorf("%s imports forbidden package %q", path, importPath)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			for _, name := range workflowDeclaredNames(node) {
				forbidden := forbiddenNames[name.Name]
				for _, fragment := range forbiddenFragments {
					forbidden = forbidden || strings.Contains(name.Name, fragment)
				}
				if forbidden {
					t.Errorf("%s declares forbidden abstraction %q", path, name.Name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowStageAlgebraRemainsSealed(t *testing.T) {
	files := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(files, path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if typeSpec.Name.IsExported() {
					if _, open := typeSpec.Type.(*ast.InterfaceType); open {
						t.Errorf("%s exposes open Workflow behavior interface %s", path, typeSpec.Name)
					}
				}
				if typeSpec.Name.Name != "Stage" {
					continue
				}
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					t.Errorf("%s declares Stage as a non-struct", path)
					continue
				}
				for _, field := range structure.Fields.List {
					for _, name := range field.Names {
						if name.IsExported() {
							t.Errorf("%s exposes mutable Stage field %s", path, name)
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func workflowDeclaredNames(node ast.Node) []*ast.Ident {
	switch declaration := node.(type) {
	case *ast.TypeSpec:
		return []*ast.Ident{declaration.Name}
	case *ast.FuncDecl:
		return []*ast.Ident{declaration.Name}
	case *ast.ValueSpec:
		return declaration.Names
	case *ast.Field:
		return declaration.Names
	default:
		return nil
	}
}
