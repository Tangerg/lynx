package arch_test

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

var validatedModalityImports = map[string]struct{}{
	"github.com/Tangerg/lynx/core/embedding":     {},
	"github.com/Tangerg/lynx/core/image":         {},
	"github.com/Tangerg/lynx/core/moderation":    {},
	"github.com/Tangerg/lynx/core/speech":        {},
	"github.com/Tangerg/lynx/core/transcription": {},
}

// TestModalityModelBoundariesValidateRequests prevents adapters from
// dereferencing or translating a Core request before its complete protocol
// value has been checked. Stream implementations may delegate to their
// validated Call method.
func TestModalityModelBoundariesValidateRequests(t *testing.T) {
	t.Parallel()

	root := modelsRoot(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(filename, ".go") || strings.HasSuffix(filename, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, filename, nil, 0)
		if err != nil {
			return err
		}
		aliases := modalityImportAliases(file)
		if len(aliases) == 0 {
			return nil
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || function.Body == nil || (function.Name.Name != "Call" && function.Name.Name != "Stream") {
				continue
			}
			requestName, ok := coreRequestParameter(function, aliases)
			if !ok {
				continue
			}
			if validatesOrDelegates(function.Body, requestName) {
				continue
			}
			relative, err := filepath.Rel(root, filename)
			if err != nil {
				return err
			}
			t.Errorf("%s:%d %s must validate %s before crossing the provider boundary", filepath.ToSlash(relative), fset.Position(function.Pos()).Line, function.Name.Name, requestName)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProviderOptionKeysAreNamespaced(t *testing.T) {
	t.Parallel()

	root := modelsRoot(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(filename, ".go") || strings.HasSuffix(filename, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, filename, nil, 0)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		provider := strings.Split(filepath.ToSlash(relative), "/")[0]
		want := provider + "/options"
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, specification := range general.Specs {
				values := specification.(*ast.ValueSpec)
				for index, name := range values.Names {
					if name.Name != "OptionsKey" || index >= len(values.Values) {
						continue
					}
					literal, ok := values.Values[index].(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						t.Errorf("%s:%d OptionsKey must be a string literal", filepath.ToSlash(relative), fset.Position(name.Pos()).Line)
						continue
					}
					got, err := strconv.Unquote(literal.Value)
					if err != nil || got != want {
						t.Errorf("%s:%d OptionsKey = %q, want %q", filepath.ToSlash(relative), fset.Position(name.Pos()).Line, got, want)
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

func modelsRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
}

func modalityImportAliases(file *ast.File) map[string]struct{} {
	aliases := make(map[string]struct{})
	for _, imported := range file.Imports {
		pathValue, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			continue
		}
		if _, target := validatedModalityImports[pathValue]; !target {
			continue
		}
		name := filepath.Base(pathValue)
		if imported.Name != nil {
			name = imported.Name.Name
		}
		aliases[name] = struct{}{}
	}
	return aliases
}

func coreRequestParameter(function *ast.FuncDecl, aliases map[string]struct{}) (string, bool) {
	for _, field := range function.Type.Params.List {
		pointer, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		selector, ok := pointer.X.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Request" {
			continue
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			continue
		}
		if _, target := aliases[qualifier.Name]; !target || len(field.Names) != 1 {
			continue
		}
		return field.Names[0].Name, true
	}
	return "", false
}

func validatesOrDelegates(body *ast.BlockStmt, requestName string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return !found
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return !found
		}
		if receiver, ok := selector.X.(*ast.Ident); ok && receiver.Name == requestName && selector.Sel.Name == "Validate" {
			found = true
			return false
		}
		if selector.Sel.Name != "Call" {
			return !found
		}
		for _, argument := range call.Args {
			if identifier, ok := argument.(*ast.Ident); ok && identifier.Name == requestName {
				found = true
				return false
			}
		}
		return !found
	})
	return found
}
