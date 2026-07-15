package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

// TestWireDTOFieldsExcludeArbitraryRuntimeValues keeps protocol DTOs safe at
// assignment time. Provider SDK objects, functions, readers, and other
// runtime-only values must be converted at an adapter boundary instead of
// surviving in Core until json.Marshal discovers them.
func TestWireDTOFieldsExcludeArbitraryRuntimeValues(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	fset := token.NewFileSet()
	for _, filename := range productionGoFiles(t) {
		packagePath, err := filepath.Rel(root, filepath.Dir(filename))
		if err != nil {
			t.Fatal(err)
		}
		packagePath = filepath.ToSlash(packagePath)
		if _, public := targetPublicPackages[packagePath]; !public {
			continue
		}

		file, err := parser.ParseFile(fset, filename, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok || !ast.IsExported(typeSpec.Name.Name) || !hasJSONTag(structure) {
					continue
				}
				assertWireFieldsAreSafe(t, packagePath, typeSpec.Name.Name, structure)
			}
		}
	}
}

func assertWireFieldsAreSafe(t *testing.T, packagePath, typeName string, structure *ast.StructType) {
	t.Helper()
	for _, field := range structure.Fields.List {
		if wireFieldIgnored(field) {
			continue
		}
		for _, name := range field.Names {
			if !ast.IsExported(name.Name) {
				continue
			}
			qualified := packagePath + "." + typeName + "." + name.Name
			if name.Name == "Params" {
				t.Errorf("%s must not reintroduce a provider parameter bag; use typed Options plus JSON-safe metadata", qualified)
			}
			if containsArbitraryRuntimeValue(field.Type) {
				t.Errorf("%s contains any/interface{}; encode provider extensions as metadata.Map at the adapter boundary", qualified)
			}
		}
	}
}

func wireFieldIgnored(field *ast.Field) bool {
	if field.Tag == nil {
		return false
	}
	tag, err := strconv.Unquote(field.Tag.Value)
	return err == nil && reflect.StructTag(tag).Get("json") == "-"
}

func containsArbitraryRuntimeValue(expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.Ident:
			if typed.Name == "any" {
				found = true
				return false
			}
		case *ast.InterfaceType:
			found = true
			return false
		}
		return !found
	})
	return found
}
