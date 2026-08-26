package repoarch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var retiredProviderSymbols = map[string]struct{}{
	"API":          {},
	"APIConfig":    {},
	"FetchNative":  {},
	"NewAPI":       {},
	"Request":      {},
	"Response":     {},
	"SearchNative": {},
}

// TestModelProvidersOwnTheirPublicSurface prevents provider facades from
// exporting transport owners, wire DTOs, or third-party SDK types. SDK values
// may exist behind a provider boundary, but consumers must only see Core,
// stdlib, and provider-owned semantic types.
func TestModelProvidersOwnTheirPublicSurface(t *testing.T) {
	t.Parallel()

	root := filepath.Join(repositoryRoot(t), "models")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "catalog" || entry.Name() == "internal" || entry.Name() == "protocol" {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if hasProductionGoFiles(t, dir) {
			assertOwnedPublicSurface(t, dir, retiredProviderSymbols)
		}
	}

	// Reusable wire protocols are public infrastructure rather than provider
	// facades, but their semantic APIs must still hide SDK-owned types.
	assertOwnedPublicSurface(t, filepath.Join(root, "protocol", "anthropic"), nil)
	assertOwnedPublicSurface(t, filepath.Join(root, "protocol", "openai"), nil)
}

// TestWebProvidersExposeOnlyNormalizedTransport locks each provider package to
// Config + Client + the family SPI. Provider wire DTOs stay private, and every
// provider keeps an offline test without environment-dependent credentials.
func TestWebProvidersExposeOnlyNormalizedTransport(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	family := "tools/web"
	familyDir := filepath.Join(root, filepath.FromSlash(family))
	entries, err := os.ReadDir(familyDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "internal" {
			continue
		}
		dir := filepath.Join(familyDir, entry.Name())
		if !hasProductionGoFiles(t, dir) {
			continue
		}
		t.Run(strings.ReplaceAll(family+"/"+entry.Name(), "/", "_"), func(t *testing.T) {
			assertOwnedPublicSurface(t, dir, retiredProviderSymbols)
			assertProviderTestsAreOffline(t, dir)
		})
	}
}

// TestFixedConstructionStateUsesConfig prevents stable constructor
// dependencies from being hidden again behind one-shot option closures.
func TestFixedConstructionStateUsesConfig(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, relative := range []string{
		"app/runtime/internal/delivery/dispatch",
		"core/chatclient",
		"documentreaders/html",
		"documentreaders/markdown",
		"documentreaders/pdf",
	} {
		dir := filepath.Join(root, filepath.FromSlash(relative))
		for _, file := range parseImmediateProductionFiles(t, dir) {
			for _, declaration := range file.Decls {
				switch value := declaration.(type) {
				case *ast.FuncDecl:
					if value.Name.IsExported() && strings.HasPrefix(value.Name.Name, "With") {
						t.Errorf("%s exports %s; fixed construction state belongs in Config", relative, value.Name.Name)
					}
				case *ast.GenDecl:
					for _, specification := range value.Specs {
						typeSpec, ok := specification.(*ast.TypeSpec)
						if ok && typeSpec.Name.Name == "Option" {
							t.Errorf("%s exports Option; fixed construction state belongs in Config", relative)
						}
					}
				}
			}
		}
	}
}

func assertOwnedPublicSurface(t *testing.T, dir string, retired map[string]struct{}) {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		thirdParty := thirdPartyImportAliases(file)
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if !value.Name.IsExported() || !receiverIsPublic(value.Recv) {
					continue
				}
				rejectRetiredProviderSymbol(t, fset, path, value.Name, retired)
				rejectThirdPartySelectors(t, fset, path, value.Type, thirdParty)
			case *ast.GenDecl:
				for _, specification := range value.Specs {
					switch spec := specification.(type) {
					case *ast.TypeSpec:
						if !spec.Name.IsExported() {
							continue
						}
						rejectRetiredProviderSymbol(t, fset, path, spec.Name, retired)
						rejectThirdPartySelectors(t, fset, path, spec.Type, thirdParty)
					case *ast.ValueSpec:
						if spec.Type == nil || !hasExportedName(spec.Names) {
							continue
						}
						rejectThirdPartySelectors(t, fset, path, spec.Type, thirdParty)
					}
				}
			}
		}
	}
}

func receiverIsPublic(receiver *ast.FieldList) bool {
	if receiver == nil {
		return true
	}
	if len(receiver.List) != 1 {
		return false
	}
	typeExpression := receiver.List[0].Type
	if pointer, ok := typeExpression.(*ast.StarExpr); ok {
		typeExpression = pointer.X
	}
	identifier, ok := typeExpression.(*ast.Ident)
	return ok && identifier.IsExported()
}

func assertProviderTestsAreOffline(t *testing.T, dir string) {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		found = true
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		osAliases := importAliases(file, "os")
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			qualifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			if _, imported := osAliases[qualifier.Name]; imported && (selector.Sel.Name == "Getenv" || selector.Sel.Name == "LookupEnv" || selector.Sel.Name == "Environ") {
				t.Errorf("%s:%d provider tests must be offline; found os.%s", filepath.ToSlash(path), fset.Position(selector.Pos()).Line, selector.Sel.Name)
			}
			return true
		})
	}
	if !found {
		t.Errorf("%s has no provider-owned tests", filepath.ToSlash(dir))
	}
}

func parseImmediateProductionFiles(t *testing.T, dir string) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	return files
}

func thirdPartyImportAliases(file *ast.File) map[string]struct{} {
	aliases := make(map[string]struct{})
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil || isRepositoryImport(path) || !isThirdPartyImport(path) {
			continue
		}
		name := filepath.Base(path)
		if imported.Name != nil {
			name = imported.Name.Name
		}
		if name != "_" && name != "." {
			aliases[name] = struct{}{}
		}
	}
	return aliases
}

func importAliases(file *ast.File, wantPath string) map[string]struct{} {
	aliases := make(map[string]struct{})
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil || path != wantPath {
			continue
		}
		name := filepath.Base(path)
		if imported.Name != nil {
			name = imported.Name.Name
		}
		aliases[name] = struct{}{}
	}
	return aliases
}

func rejectRetiredProviderSymbol(t *testing.T, fset *token.FileSet, path string, name *ast.Ident, retired map[string]struct{}) {
	t.Helper()
	if _, forbidden := retired[name.Name]; forbidden {
		t.Errorf("%s:%d exports retired transport symbol %s", filepath.ToSlash(path), fset.Position(name.Pos()).Line, name.Name)
	}
}

func rejectThirdPartySelectors(t *testing.T, fset *token.FileSet, path string, node ast.Node, aliases map[string]struct{}) {
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
		if _, thirdParty := aliases[qualifier.Name]; thirdParty {
			t.Errorf("%s:%d public API leaks third-party type %s.%s", filepath.ToSlash(path), fset.Position(selector.Pos()).Line, qualifier.Name, selector.Sel.Name)
		}
		return true
	})
}

func hasExportedName(names []*ast.Ident) bool {
	for _, name := range names {
		if name.IsExported() {
			return true
		}
	}
	return false
}
