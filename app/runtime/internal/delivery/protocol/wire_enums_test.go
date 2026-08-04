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

func TestWireEnumReturnsCallerOwnedValues(t *testing.T) {
	goType := reflect.TypeFor[RunStatus]()
	first, ok := WireEnum(goType)
	if !ok || len(first) == 0 {
		t.Fatal("RunStatus has no registered wire values")
	}
	want := first[0]
	first[0] = "rewritten-by-caller"

	second, ok := WireEnum(goType)
	if !ok || len(second) == 0 {
		t.Fatal("RunStatus disappeared after caller mutation")
	}
	if second[0] != want {
		t.Fatalf("WireEnum leaked registry ownership: got %q, want %q", second[0], want)
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

	// Two passes, because a constant may be declared as a conversion of another
	// one — the way a union containing another union spells the values it inherits
	// instead of repeating their text. The literals have to be known before a
	// conversion can be resolved to one, and Go's declaration order does not
	// promise the source came first.
	literals := make(map[string]string)
	type conversion struct{ enum, constant string }
	var conversions []conversion
	out := make(map[string][]string)
	for _, syntax := range parsed {
		for _, decl := range syntax.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || group.Tok != token.CONST {
				continue
			}
			for _, spec := range group.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Values) != 1 || len(value.Names) != 1 {
					continue
				}
				// Keep every package string constant available as a conversion
				// source, including untyped vocabularies such as FeatureSubagents.
				// Only constants whose declared type is a wire enum enter out.
				if literal, ok := value.Values[0].(*ast.BasicLit); ok && literal.Kind == token.STRING {
					text, err := strconv.Unquote(literal.Value)
					if err != nil {
						t.Fatalf("unquote %s: %v", literal.Value, err)
					}
					literals[value.Names[0].Name] = text
				}
				if ident, ok := value.Type.(*ast.Ident); ok && stringTypes[ident.Name] {
					literal, ok := value.Values[0].(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						continue
					}
					text, err := strconv.Unquote(literal.Value)
					if err != nil {
						t.Fatalf("unquote %s: %v", literal.Value, err)
					}
					out[ident.Name] = append(out[ident.Name], text)
					continue
				}
				// `X = EnumType(Y)` — the value is Y's, published under EnumType too.
				if value.Type != nil {
					continue
				}
				call, ok := value.Values[0].(*ast.CallExpr)
				if !ok || len(call.Args) != 1 {
					continue
				}
				enum, ok := call.Fun.(*ast.Ident)
				if !ok || !stringTypes[enum.Name] {
					continue
				}
				source, ok := call.Args[0].(*ast.Ident)
				if !ok {
					continue
				}
				conversions = append(conversions, conversion{enum: enum.Name, constant: source.Name})
			}
		}
	}
	for _, converted := range conversions {
		text, ok := literals[converted.constant]
		if !ok {
			t.Errorf("%s converts %s, which is not a string constant of this package", converted.enum, converted.constant)
			continue
		}
		out[converted.enum] = append(out[converted.enum], text)
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
