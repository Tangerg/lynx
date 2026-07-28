package protocol

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestWireEnumsAreComplete checks the declared value sets against the constants
// this package actually defines.
//
// [wireEnums] is a second statement of something the code already says, and the
// only reason it may exist is that reflection cannot read a const block. That
// makes it exactly the kind of table that rots: someone adds a RunStatus and the
// generated schema keeps publishing two values. So the constants are read here —
// by a TEST, not by the generator, which is why contract §11.2's "AST only reads
// godoc" rule is not bent: nothing in the artifact pipeline infers vocabulary
// from source layout; the pipeline reads the declaration, and this proves the
// declaration is the whole truth.
func TestWireEnumsAreComplete(t *testing.T) {
	declared := constantsByType(t)

	for name, values := range declared {
		goType, ok := typeByName(name)
		if !ok {
			t.Errorf("no reflect.Type found for %s; the test's type index needs it", name)
			continue
		}
		registered, ok := WireEnum(goType)
		if !ok {
			t.Errorf("%s has %d constants but no wireEnums entry — every generated schema would type it as a bare string", name, len(values))
			continue
		}
		if !slices.Equal(registered, values) {
			t.Errorf("%s: wireEnums says %v, the constants say %v", name, registered, values)
		}
	}

	// The other direction: an entry for a type whose constants were deleted would
	// keep publishing a value the runtime no longer produces.
	for goType := range wireEnums {
		if _, ok := declared[goType.Name()]; !ok {
			t.Errorf("wireEnums declares %s, which has no string constants in this package", goType.Name())
		}
	}
}

// constantsByType reads every `type X string` in the package's non-test files and
// the string constants declared with that type, in source order.
func constantsByType(t *testing.T) map[string][]string {
	t.Helper()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob package files: %v", err)
	}
	slices.Sort(files)

	stringTypes := make(map[string]bool)
	var parsed []*ast.File
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		syntax, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		parsed = append(parsed, syntax)
		for _, spec := range typeSpecs(syntax) {
			if ident, ok := spec.Type.(*ast.Ident); ok && ident.Name == "string" {
				stringTypes[spec.Name.Name] = true
			}
		}
	}

	out := make(map[string][]string)
	for _, syntax := range parsed {
		for _, decl := range syntax.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || group.Tok != token.CONST {
				continue
			}
			for _, spec := range group.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || value.Type == nil || len(value.Values) != 1 {
					continue
				}
				ident, ok := value.Type.(*ast.Ident)
				if !ok || !stringTypes[ident.Name] {
					continue
				}
				literal, ok := value.Values[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				text, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatalf("unquote %s: %v", literal.Value, err)
				}
				out[ident.Name] = append(out[ident.Name], text)
			}
		}
	}
	return out
}

func typeSpecs(syntax *ast.File) []*ast.TypeSpec {
	var out []*ast.TypeSpec
	for _, decl := range syntax.Decls {
		group, ok := decl.(*ast.GenDecl)
		if !ok || group.Tok != token.TYPE {
			continue
		}
		for _, spec := range group.Specs {
			if typeSpec, ok := spec.(*ast.TypeSpec); ok {
				out = append(out, typeSpec)
			}
		}
	}
	return out
}

// typeByName maps a source type name back to its reflect.Type through the
// registered set. A type absent from wireEnums is reported by the caller, so this
// only has to resolve the ones already there.
func typeByName(name string) (reflect.Type, bool) {
	for goType := range wireEnums {
		if goType.Name() == name {
			return goType, true
		}
	}
	return nil, false
}
