package planning

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestPlanningWireBaseline(t *testing.T) {
	shape := planningWireShape()
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(shape)))
	const want = "aa6dc54ede1bc93e4c532377127c962f008729b25bcb7945857a54f12c34c0ff"
	if got != want {
		t.Fatalf("Planning wire changed: got %s, want %s\n%s", got, want, shape)
	}
}

func TestPlanningWireBaselineCoversEveryPrivateJSONStruct(t *testing.T) {
	assertPrivateJSONStructsCovered(t, planningWireTypes())
}

func planningWireShape() string {
	types := planningWireTypes()
	slices.SortFunc(types, func(left, right reflect.Type) int {
		return strings.Compare(left.Name(), right.Name())
	})
	var shape strings.Builder
	for _, wireType := range types {
		fmt.Fprintf(&shape, "%s\n", wireType.Name())
		for field := range wireType.Fields() {
			fmt.Fprintf(&shape, "  %s %s json=%q\n", field.Name, field.Type.String(), field.Tag.Get("json"))
		}
	}
	return shape.String()
}

func planningWireTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeFor[actionCall](),
		reflect.TypeFor[actionResultWire](),
		reflect.TypeFor[conditionWire](),
		reflect.TypeFor[effectEnvelope](),
		reflect.TypeFor[executionState](),
		reflect.TypeFor[observationResult](),
		reflect.TypeFor[planWire](),
		reflect.TypeFor[signalEnvelope](),
		reflect.TypeFor[worldStateWire](),
	}
}

func assertPrivateJSONStructsCovered(t *testing.T, wireTypes []reflect.Type) {
	t.Helper()
	covered := make(map[string]struct{}, len(wireTypes))
	for _, wireType := range wireTypes {
		covered[wireType.Name()] = struct{}{}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok || typeSpec.Name.IsExported() {
					continue
				}
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok || !structHasJSONTag(structure) {
					continue
				}
				if _, found := covered[typeSpec.Name.Name]; !found {
					t.Errorf("%s: private JSON struct %s is absent from the Planning wire baseline", name, typeSpec.Name.Name)
				}
			}
		}
	}
}

func structHasJSONTag(structure *ast.StructType) bool {
	for _, field := range structure.Fields.List {
		if field.Tag != nil && strings.Contains(field.Tag.Value, "json:") {
			return true
		}
	}
	return false
}
