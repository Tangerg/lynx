package repoarch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestReceiversAreTheirTypeInitial keeps one receiver spelling across the
// repository. A receiver named after a word inside the type reads fine in
// isolation but makes two methods on sibling types look unrelated, and a
// receiver spelled out in full collides with the domain vocabulary used for
// locals in the same body.
func TestReceiversAreTheirTypeInitial(t *testing.T) {
	t.Parallel()
	forEachGoFile(t, func(path string, fset *token.FileSet, file *ast.File) {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 {
				continue
			}
			names := function.Recv.List[0].Names
			if len(names) != 1 || names[0].Name == "_" {
				continue
			}
			typeName := receiverTypeName(function.Recv.List[0].Type)
			if typeName == "" {
				continue
			}
			want := strings.ToLower(typeName[:1])
			if names[0].Name != want {
				t.Errorf("%s:%d %s.%s uses receiver %q; want %q, the type initial",
					path, fset.Position(function.Pos()).Line,
					typeName, function.Name.Name, names[0].Name, want)
			}
		}
	})
}

// TestParametersDoNotShadowImportedPackages keeps an identifier from meaning
// two things inside one body. A parameter named like an imported package makes
// `filter.Predicate` and `filter.Accept` read alike while resolving to
// different objects, which survives review and misleads the next reader.
func TestParametersDoNotShadowImportedPackages(t *testing.T) {
	t.Parallel()
	forEachGoFile(t, func(path string, fset *token.FileSet, file *ast.File) {
		imported := importedPackageNames(file)
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Type.Params == nil {
				continue
			}
			for _, parameter := range function.Type.Params.List {
				for _, name := range parameter.Names {
					if !imported[name.Name] {
						continue
					}
					t.Errorf("%s:%d parameter %q of %s shadows the imported package of the same name",
						path, fset.Position(name.Pos()).Line, name.Name, function.Name.Name)
				}
			}
		}
	})
}

func importedPackageNames(file *ast.File) map[string]bool {
	names := make(map[string]bool, len(file.Imports))
	for _, specification := range file.Imports {
		path := strings.Trim(specification.Path.Value, `"`)
		name := path[strings.LastIndex(path, "/")+1:]
		if specification.Name != nil {
			name = specification.Name.Name
		}
		if name != "_" && name != "." {
			names[name] = true
		}
	}
	return names
}

func receiverTypeName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(typed.X)
	case *ast.Ident:
		return typed.Name
	case *ast.IndexExpr:
		return receiverTypeName(typed.X)
	case *ast.IndexListExpr:
		return receiverTypeName(typed.X)
	}
	return ""
}

// forEachGoFile visits every Go source file in the repository, including tests,
// because a naming rule that stops at production code drifts in the tests that
// document it.
func forEachGoFile(t *testing.T, visit func(path string, fset *token.FileSet, file *ast.File)) {
	t.Helper()
	root := repositoryRoot(t)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			name := entry.Name()
			if name == "node_modules" || name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		visit(filepath.ToSlash(relative), fset, file)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
