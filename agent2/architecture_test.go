package agent2

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestProcessAdmissionContainsOnlyFrameworkStartContracts(t *testing.T) {
	typeOf := reflect.TypeFor[ProcessAdmission]()
	want := []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "relation", typeOf: reflect.TypeFor[ProcessRelation]()},
		{name: "deployment", typeOf: reflect.TypeFor[DeploymentRef]()},
		{name: "descriptor", typeOf: reflect.TypeFor[Descriptor]()},
		{name: "budget", typeOf: reflect.TypeFor[Budget]()},
		{name: "capabilities", typeOf: reflect.TypeFor[CapabilitySet]()},
	}
	if typeOf.NumField() != len(want) {
		t.Fatalf("ProcessAdmission fields = %d, want %d", typeOf.NumField(), len(want))
	}
	for index, expected := range want {
		field := typeOf.Field(index)
		if field.IsExported() || field.Name != expected.name || field.Type != expected.typeOf {
			t.Fatalf("ProcessAdmission field %d = %s %v", index, field.Name, field.Type)
		}
	}
}

func TestEventContainsOnlyFrameworkObservationContracts(t *testing.T) {
	typeOf := reflect.TypeFor[Event]()
	want := []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "sequence", typeOf: reflect.TypeFor[uint64]()},
		{name: "processID", typeOf: reflect.TypeFor[ProcessID]()},
		{name: "deployment", typeOf: reflect.TypeFor[DeploymentRef]()},
		{name: "relation", typeOf: reflect.TypeFor[ProcessRelation]()},
		{name: "step", typeOf: reflect.TypeFor[uint64]()},
		{name: "effectID", typeOf: reflect.TypeFor[EffectID]()},
		{name: "name", typeOf: reflect.TypeFor[string]()},
		{name: "phase", typeOf: reflect.TypeFor[EventPhase]()},
		{name: "occurredAt", typeOf: reflect.TypeFor[time.Time]()},
		{name: "payload", typeOf: reflect.TypeFor[json.RawMessage]()},
	}
	if typeOf.NumField() != len(want) {
		t.Fatalf("Event fields = %d, want %d", typeOf.NumField(), len(want))
	}
	for index, expected := range want {
		field := typeOf.Field(index)
		if field.IsExported() || field.Name != expected.name || field.Type != expected.typeOf {
			t.Fatalf("Event field %d = %s %v", index, field.Name, field.Type)
		}
	}
}

func TestFrameworkRootExcludesLegacyAndHostDependencies(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	forbiddenIdentifiers := map[string]bool{
		"Store": true, "Repository": true, "Transaction": true, "Lease": true,
	}
	forbiddenFragments := []string{"Session", "Conversation", "Workspace", "WriteSet"}
	files := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(files, filepath.Clean(name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if path == "github.com/Tangerg/lynx/agent" || strings.HasPrefix(path, "github.com/Tangerg/lynx/agent/") ||
				path == "github.com/Tangerg/lynx/app" || strings.HasPrefix(path, "github.com/Tangerg/lynx/app/") {
				t.Errorf("%s imports forbidden legacy or Host package %q", name, path)
			}
			if strings.HasPrefix(path, "go.opentelemetry.io/otel") {
				t.Errorf("%s imports OpenTelemetry into the Kernel instead of the otel adapter: %q", name, path)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			for _, identifier := range declaredIdentifiers(node) {
				forbidden := forbiddenIdentifiers[identifier.Name]
				for _, fragment := range forbiddenFragments {
					forbidden = forbidden || strings.Contains(identifier.Name, fragment)
				}
				if forbidden {
					t.Errorf("%s declares forbidden Host abstraction identifier %q", name, identifier.Name)
				}
			}
			return true
		})
	}
}

func declaredIdentifiers(node ast.Node) []*ast.Ident {
	switch declaration := node.(type) {
	case *ast.TypeSpec:
		return []*ast.Ident{declaration.Name}
	case *ast.FuncDecl:
		return []*ast.Ident{declaration.Name}
	case *ast.ValueSpec:
		return declaration.Names
	case *ast.Field:
		return declaration.Names
	default:
		return nil
	}
}

func TestProcessConstructionRemainsEngineOwned(t *testing.T) {
	files := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(files, filepath.Clean(name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				assertProcessFieldsArePrivate(t, name, declaration)
			case *ast.FuncDecl:
				if returnsProcessPointer(declaration.Type.Results) &&
					(declaration.Recv == nil || receiverTypeName(declaration.Recv) != "Engine") {
					t.Errorf("%s exports non-Engine Process construction through %s", name, declaration.Name.Name)
				}
			}
		}
	}
}

func assertProcessFieldsArePrivate(t *testing.T, filename string, declaration *ast.GenDecl) {
	t.Helper()
	for _, specification := range declaration.Specs {
		typeSpec, ok := specification.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != "Process" {
			continue
		}
		structure, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			t.Fatalf("%s declares Process as a non-struct", filename)
		}
		for _, field := range structure.Fields.List {
			for _, name := range field.Names {
				if name.IsExported() {
					t.Errorf("%s exposes mutable Process field %s", filename, name.Name)
				}
			}
		}
	}
}

func returnsProcessPointer(results *ast.FieldList) bool {
	if results == nil {
		return false
	}
	for _, result := range results.List {
		pointer, ok := result.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		identifier, ok := pointer.X.(*ast.Ident)
		if ok && identifier.Name == "Process" {
			return true
		}
	}
	return false
}

func receiverTypeName(receiver *ast.FieldList) string {
	if receiver == nil || len(receiver.List) != 1 {
		return ""
	}
	value := receiver.List[0].Type
	if pointer, ok := value.(*ast.StarExpr); ok {
		value = pointer.X
	}
	identifier, _ := value.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}
