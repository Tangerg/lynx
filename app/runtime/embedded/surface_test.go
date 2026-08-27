package embedded

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"iter"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
)

func TestPublicMethodsCoverExactOperationContract(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	directory := filepath.Dir(source)
	operationNames := declaredOperationNames(t, filepath.Join(directory, "..", "internal", "delivery", "operation"))
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read embedded package: %v", err)
	}

	bindings := make(map[string]string)
	files := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(files, filepath.Join(directory, entry.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parse embedded source %s: %v", entry.Name(), err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || receiverName(function.Recv) != "Runtime" || !function.Name.IsExported() {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) < 2 {
					return true
				}
				name := calledName(call.Fun)
				if name != "invoke" && name != "invokeAck" && name != "invokeStream" {
					return true
				}
				selector, ok := call.Args[1].(*ast.SelectorExpr)
				if !ok {
					t.Errorf("%s must call the operation through its operation.Name constant", function.Name.Name)
					return false
				}
				owner, ownerOK := selector.X.(*ast.Ident)
				if !ownerOK || owner.Name != "operation" {
					t.Errorf("%s must call the operation through its operation.Name constant", function.Name.Name)
					return false
				}
				operationName, declared := operationNames[selector.Sel.Name]
				if !declared {
					t.Errorf("%s references unknown operation name constant %s", function.Name.Name, selector.Sel.Name)
					return false
				}
				if previous := bindings[operationName]; previous != "" {
					t.Errorf("operation %q is exposed by both %s and %s", operationName, previous, function.Name.Name)
				}
				bindings[operationName] = function.Name.Name
				return false
			})
		}
	}

	runtimeType := reflect.TypeFor[*Runtime]()
	errorType := reflect.TypeFor[error]()
	contextType := reflect.TypeFor[context.Context]()
	for _, meta := range operation.Contract().Metas() {
		methodName := bindings[meta.Name.String()]
		if methodName == "" {
			t.Errorf("operation %q has no public embedded method", meta.Name)
			continue
		}
		method, ok := runtimeType.MethodByName(methodName)
		if !ok {
			t.Errorf("embedded method %s is not exported", methodName)
			continue
		}
		if method.Type.In(1) != contextType {
			t.Errorf("%s context parameter = %s", methodName, method.Type.In(1))
		}
		expectedInputs := 4
		if meta.Params == reflect.TypeFor[struct{}]() {
			expectedInputs = 3
		}
		if method.Type.NumIn() != expectedInputs {
			t.Errorf("%s inputs = %d, want %d", methodName, method.Type.NumIn(), expectedInputs)
			continue
		}
		requestIndex := 2
		if meta.Params == reflect.TypeFor[struct{}]() {
			requestIndex = -1
		} else if method.Type.In(2) != meta.Params {
			t.Errorf("%s request = %s, want %s", methodName, method.Type.In(2), meta.Params)
		}
		optionIndex := 2
		if requestIndex >= 0 {
			optionIndex = 3
		}
		if got, want := method.Type.In(optionIndex), optionType(meta); got != want {
			t.Errorf("%s options = %s, want %s", methodName, got, want)
		}

		if meta.Kind == operation.KindStream {
			if method.Type.NumOut() != 3 || method.Type.Out(0) != meta.Result || method.Type.Out(2) != errorType {
				t.Errorf("%s stream result does not match %s", methodName, meta.Name)
				continue
			}
			assertIteratorEvent(t, methodName, method.Type.Out(1), meta.Event)
			continue
		}
		if meta.Result == nil {
			if method.Type.NumOut() != 1 || method.Type.Out(0) != errorType {
				t.Errorf("%s acknowledgement result does not match %s", methodName, meta.Name)
			}
			continue
		}
		if method.Type.NumOut() != 2 || method.Type.Out(0) != meta.Result || method.Type.Out(1) != errorType {
			t.Errorf("%s result does not match %s", methodName, meta.Name)
		}
	}
	if len(bindings) != len(operation.Contract().Metas()) {
		t.Fatalf("embedded bindings = %d, operations = %d", len(bindings), len(operation.Contract().Metas()))
	}
}

func optionType(meta operation.MethodMeta) reflect.Type {
	switch {
	case meta.Kind == operation.KindStream && meta.Operation == operation.OperationCommand:
		return reflect.TypeFor[RunCommandOptions]()
	case meta.ReplayCursor == operation.ReplayCursorRun:
		return reflect.TypeFor[RunSubscriptionOptions]()
	case meta.Kind == operation.KindStream:
		return reflect.TypeFor[SubscriptionOptions]()
	case meta.Operation == operation.OperationCommand:
		return reflect.TypeFor[CommandOptions]()
	default:
		return reflect.TypeFor[CallOptions]()
	}
}

func assertIteratorEvent(t *testing.T, methodName string, iterator, event reflect.Type) {
	t.Helper()
	want := reflect.TypeFor[iter.Seq2[struct{}, error]]()
	if iterator.Kind() != want.Kind() || iterator.NumIn() != 1 {
		t.Errorf("%s stream = %s, want iter.Seq2", methodName, iterator)
		return
	}
	yield := iterator.In(0)
	if yield.Kind() != reflect.Func || yield.NumIn() != 2 || yield.In(0) != event || yield.In(1) != reflect.TypeFor[error]() {
		t.Errorf("%s iterator event = %s, want %s", methodName, yield, event)
	}
}

func receiverName(fields *ast.FieldList) string {
	if fields == nil || len(fields.List) != 1 {
		return ""
	}
	typeExpression := fields.List[0].Type
	if pointer, ok := typeExpression.(*ast.StarExpr); ok {
		typeExpression = pointer.X
	}
	identifier, _ := typeExpression.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}

func declaredOperationNames(t *testing.T, directory string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read operation package: %v", err)
	}
	names := make(map[string]string)
	files := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(files, filepath.Join(directory, entry.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parse operation source %s: %v", entry.Name(), err)
		}
		for _, declaration := range file.Decls {
			group, ok := declaration.(*ast.GenDecl)
			if !ok || group.Tok != token.CONST {
				continue
			}
			for _, spec := range group.Specs {
				value, ok := spec.(*ast.ValueSpec)
				typeName, typed := value.Type.(*ast.Ident)
				if !ok || !typed || typeName.Name != "Name" || len(value.Names) != 1 || len(value.Values) != 1 {
					continue
				}
				literal, ok := value.Values[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				wireName, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatalf("parse operation constant %s: %v", value.Names[0].Name, err)
				}
				names[value.Names[0].Name] = wireName
			}
		}
	}
	return names
}

func calledName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.IndexExpr:
		return calledName(value.X)
	case *ast.IndexListExpr:
		return calledName(value.X)
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}
