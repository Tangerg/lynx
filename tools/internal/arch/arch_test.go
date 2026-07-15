package arch_test

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

func TestRootToolContractStaysMinimal(t *testing.T) {
	typeOf := reflect.TypeFor[tools.Tool]()
	if typeOf.Kind() != reflect.Interface || typeOf.NumMethod() != 2 {
		t.Fatalf("Tool shape = %v with %d methods, want two-method interface", typeOf, typeOf.NumMethod())
	}
	want := map[string]reflect.Type{
		"Call":       reflect.TypeFor[func(context.Context, string) (string, error)](),
		"Definition": reflect.TypeFor[func() chat.ToolDefinition](),
	}
	for name, signature := range want {
		method, ok := typeOf.MethodByName(name)
		if !ok || method.Type != signature {
			t.Errorf("Tool.%s = %v (present %v), want %v", name, method.Type, ok, signature)
		}
	}
}

func TestRootToolsDoNotImportLegacyModelOrRuntime(t *testing.T) {
	root := toolsRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(root, name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if forbiddenImport(importPath) {
				t.Errorf("root tools production file %s imports forbidden legacy/runtime package %s", name, importPath)
			}
		}
	}
}

func forbiddenImport(importPath string) bool {
	for _, prefix := range []string{
		"github.com/Tangerg/lynx/agent",
		"github.com/Tangerg/lynx/chatclient",
	} {
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return true
		}
	}
	return false
}

func toolsRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller did not report test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
