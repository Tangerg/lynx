package interaction

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

func TestInteractionWireBaseline(t *testing.T) {
	shape := interactionWireShape()
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(shape)))
	const want = "3bd9278dd81f2c7d10aa0526db24c4e3ebcc7192ed2bb43a37b23e76b086fcee"
	if got != want {
		t.Fatalf("Interaction wire changed: got %s, want %s\n%s", got, want, shape)
	}
}

func TestInteractionWireBaselineCoversEveryPrivateJSONStruct(t *testing.T) {
	assertPrivateJSONStructsCovered(t, interactionWireTypes())
}

func interactionWireShape() string {
	types := interactionWireTypes()
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

func interactionWireTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeFor[artifactRecord](),
		reflect.TypeFor[delegateInvocationState](),
		reflect.TypeFor[delegateSegmentState](),
		reflect.TypeFor[effectEnvelope](),
		reflect.TypeFor[executionState](),
		reflect.TypeFor[inputRequestWire](),
		reflect.TypeFor[modelCall](),
		reflect.TypeFor[modelCallResult](),
		reflect.TypeFor[modelResponseDeltaWire](),
		reflect.TypeFor[signalEnvelope](),
		reflect.TypeFor[steerBatch](),
		reflect.TypeFor[steerInput](),
		reflect.TypeFor[toolBatchCall](),
		reflect.TypeFor[toolBatchResult](),
		reflect.TypeFor[toolCheckpoint](),
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
					t.Errorf("%s: private JSON struct %s is absent from the Interaction wire baseline", name, typeSpec.Name.Name)
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
