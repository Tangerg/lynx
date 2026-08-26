package repoarch

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

const modelsImportPrefix = "github.com/Tangerg/lynx/models/"

// TestProviderDependenciesAreOneWay prevents a concrete provider from
// importing another concrete provider. Reusable wire implementations belong
// under protocol, which may only depend on lower-level protocol packages.
func TestProviderDependenciesAreOneWay(t *testing.T) {
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
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		source := strings.Split(filepath.ToSlash(relative), "/")[0]
		file, err := parser.ParseFile(fset, filename, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			pathValue, err := strconv.Unquote(imported.Path.Value)
			if err != nil || !strings.HasPrefix(pathValue, modelsImportPrefix) {
				continue
			}
			target := strings.TrimPrefix(pathValue, modelsImportPrefix)
			targetRoot := strings.Split(target, "/")[0]
			switch {
			case source == "internal" && targetRoot != "internal" && targetRoot != "protocol":
				t.Errorf("%s:%d internal implementation must not depend on public provider %q", filepath.ToSlash(relative), fset.Position(imported.Pos()).Line, targetRoot)
			case source != "internal" && targetRoot != "internal" && targetRoot != "protocol" && targetRoot != source:
				t.Errorf("%s:%d provider %q must not depend on peer provider %q", filepath.ToSlash(relative), fset.Position(imported.Pos()).Line, source, targetRoot)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestProviderAPIsHideProtocolDetails locks the public boundary: constructors,
// configuration, and data structures cannot expose protocol DTOs. An exact
// shared OpenAI/Anthropic model implementation may be promoted by type alias;
// provider-private internal implementations may not.
func TestProviderAPIsHideProtocolDetails(t *testing.T) {
	t.Parallel()

	root := modelsRoot(t)
	entries, err := filepath.Glob(filepath.Join(root, "*", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, filename := range entries {
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filename, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		protocolAliases := make(map[string]struct{})
		sharedProtocolAliases := make(map[string]struct{})
		for _, imported := range file.Imports {
			pathValue, err := strconv.Unquote(imported.Path.Value)
			if err != nil || !strings.HasPrefix(pathValue, modelsImportPrefix) {
				continue
			}
			relativeImport := strings.TrimPrefix(pathValue, modelsImportPrefix)
			shared := strings.HasPrefix(relativeImport, "protocol/")
			private := strings.Contains("/"+relativeImport+"/", "/internal/protocol/")
			if !shared && !private {
				continue
			}
			name := filepath.Base(pathValue)
			if imported.Name != nil {
				name = imported.Name.Name
			}
			protocolAliases[name] = struct{}{}
			if shared {
				sharedProtocolAliases[name] = struct{}{}
			}
		}
		if len(protocolAliases) == 0 {
			continue
		}
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if value.Name.IsExported() {
					rejectProtocolSelectors(t, fset, filename, value.Type, protocolAliases)
				}
			case *ast.GenDecl:
				for _, specification := range value.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if !ok || !typeSpec.Name.IsExported() {
						continue
					}
					if typeSpec.Assign.IsValid() && isImportedSelector(typeSpec.Type, sharedProtocolAliases) {
						continue
					}
					switch exportedType := typeSpec.Type.(type) {
					case *ast.StructType:
						for _, field := range exportedType.Fields.List {
							if len(field.Names) == 0 || field.Names[0].IsExported() {
								rejectProtocolSelectors(t, fset, filename, field.Type, protocolAliases)
							}
						}
					default:
						rejectProtocolSelectors(t, fset, filename, typeSpec.Type, protocolAliases)
					}
				}
			}
		}
	}
}

func isImportedSelector(expression ast.Expr, aliases map[string]struct{}) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	_, imported := aliases[qualifier.Name]
	return imported
}

func rejectProtocolSelectors(t *testing.T, fset *token.FileSet, filename string, node ast.Node, aliases map[string]struct{}) {
	t.Helper()
	ast.Inspect(node, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if _, protocol := aliases[qualifier.Name]; protocol {
			t.Errorf("%s:%d exported API leaks protocol implementation type %s.%s", filepath.Base(filename), fset.Position(selector.Pos()).Line, qualifier.Name, selector.Sel.Name)
		}
		return true
	})
}
