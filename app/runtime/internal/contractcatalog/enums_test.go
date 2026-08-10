package contractcatalog

import (
	"go/ast"
	"go/token"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/protocol"
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
		registered, ok := EnumValues(goType)
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

func TestEnumValuesReturnsCallerOwnedValues(t *testing.T) {
	goType := reflect.TypeFor[protocol.RunStatus]()
	first, ok := EnumValues(goType)
	if !ok || len(first) == 0 {
		t.Fatal("RunStatus has no registered wire values")
	}
	want := first[0]
	first[0] = "rewritten-by-caller"

	second, ok := EnumValues(goType)
	if !ok || len(second) == 0 {
		t.Fatal("RunStatus disappeared after caller mutation")
	}
	if second[0] != want {
		t.Fatalf("EnumValues leaked registry ownership: got %q, want %q", second[0], want)
	}
}

// constantsByType reads every `type X string` in the package's non-test files and
// the string constants declared with that type, in source order.
func constantsByType(t *testing.T) map[string][]string {
	t.Helper()
	parsed := parseProtocolSource(t)
	stringTypes := make(map[string]bool)
	for _, syntax := range parsed {
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
	var conversions []enumConversion
	out := make(map[string][]string)
	for _, syntax := range parsed {
		for _, value := range constantSpecs(syntax) {
			if len(value.Values) != 1 || len(value.Names) != 1 {
				continue
			}
			recordStringLiteral(t, value, literals)
			if recordTypedEnum(t, value, stringTypes, out) {
				continue
			}
			if conversion, ok := enumConversionFrom(value, stringTypes); ok {
				conversions = append(conversions, conversion)
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

type enumConversion struct{ enum, constant string }

func recordStringLiteral(t *testing.T, value *ast.ValueSpec, literals map[string]string) {
	t.Helper()
	literal, ok := value.Values[0].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return
	}
	text, err := strconv.Unquote(literal.Value)
	if err != nil {
		t.Fatalf("unquote %s: %v", literal.Value, err)
	}
	literals[value.Names[0].Name] = text
}

func recordTypedEnum(t *testing.T, value *ast.ValueSpec, stringTypes map[string]bool, out map[string][]string) bool {
	t.Helper()
	ident, ok := value.Type.(*ast.Ident)
	if !ok || !stringTypes[ident.Name] {
		return false
	}
	literal, ok := value.Values[0].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return true
	}
	text, err := strconv.Unquote(literal.Value)
	if err != nil {
		t.Fatalf("unquote %s: %v", literal.Value, err)
	}
	out[ident.Name] = append(out[ident.Name], text)
	return true
}

func enumConversionFrom(value *ast.ValueSpec, stringTypes map[string]bool) (enumConversion, bool) {
	if value.Type != nil {
		return enumConversion{}, false
	}
	call, ok := value.Values[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return enumConversion{}, false
	}
	enum, ok := call.Fun.(*ast.Ident)
	if !ok || !stringTypes[enum.Name] {
		return enumConversion{}, false
	}
	source, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return enumConversion{}, false
	}
	return enumConversion{enum: enum.Name, constant: source.Name}, true
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
