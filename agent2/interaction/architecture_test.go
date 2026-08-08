package interaction

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/core/chat"
)

func TestActiveDelegateChildContainsOnlyStrategyAttribution(t *testing.T) {
	typeOf := reflect.TypeOf(ActiveDelegateChild{})
	want := []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "modelCallSequence", typeOf: reflect.TypeFor[uint32]()},
		{name: "toolCallIndex", typeOf: reflect.TypeFor[uint32]()},
		{name: "toolCall", typeOf: reflect.TypeFor[chat.ToolCall]()},
		{name: "childKey", typeOf: reflect.TypeFor[agent.ChildKey]()},
		{name: "processID", typeOf: reflect.TypeFor[agent.ProcessID]()},
	}
	if typeOf.NumField() != len(want) {
		t.Fatalf("ActiveDelegateChild fields = %d, want %d", typeOf.NumField(), len(want))
	}
	for index, expected := range want {
		field := typeOf.Field(index)
		if field.IsExported() || field.Name != expected.name || field.Type != expected.typeOf {
			t.Fatalf("ActiveDelegateChild field %d = %s %v", index, field.Name, field.Type)
		}
	}
}

func TestDispatcherCannotOwnOrStartManagedProcesses(t *testing.T) {
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "dispatcher.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]struct{}{
		"Engine": {}, "Process": {}, "StartChild": {}, "WaitForChildren": {},
	}
	ast.Inspect(file, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if _, blocked := forbidden[identifier.Name]; blocked {
			t.Errorf("dispatcher.go references forbidden managed-lifecycle identifier %q", identifier.Name)
		}
		return true
	})
}
