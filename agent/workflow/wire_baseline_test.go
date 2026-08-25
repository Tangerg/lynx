package workflow

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

func TestWorkflowWireBaseline(t *testing.T) {
	shape := workflowWireShape()
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(shape)))
	const want = "2d4d33d0c9996077abb594f3d2aab47d37059c6e126ad5e971ee8d484bb4442d"
	if got != want {
		t.Fatalf("Workflow wire changed: got %s, want %s\n%s", got, want, shape)
	}
}

func TestWorkflowWireBaselineCoversEveryPrivateJSONStruct(t *testing.T) {
	assertPrivateJSONStructsCovered(t, workflowWireTypes())
}

func workflowWireShape() string {
	types := workflowWireTypes()
	slices.SortFunc(types, func(left, right reflect.Type) int {
		return strings.Compare(left.Name(), right.Name())
	})
	var shape strings.Builder
	fmt.Fprintf(&shape, "execution_state=%d\n", executionStateSchemaVersion)
	for _, wireType := range types {
		fmt.Fprintf(&shape, "%s\n", wireType.Name())
		for index := range wireType.NumField() {
			field := wireType.Field(index)
			fmt.Fprintf(&shape, "  %s %s json=%q\n", field.Name, field.Type.String(), field.Tag.Get("json"))
		}
	}
	return shape.String()
}

func workflowWireTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeOf(executionState{}),
		reflect.TypeOf(fanoutChildState{}),
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
					t.Errorf("%s: private JSON struct %s is absent from the Workflow wire baseline", name, typeSpec.Name.Name)
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
