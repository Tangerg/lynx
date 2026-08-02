package arch_test

import (
	"context"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

func TestToolContractStaysMinimal(t *testing.T) {
	typeOf := reflect.TypeFor[tool.Tool]()
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

func TestProductionDependenciesStayAtCore(t *testing.T) {
	root := moduleRoot(t)
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == "vendor" || entry.Name() == "internal" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			first, _, _ := strings.Cut(importPath, "/")
			if !strings.Contains(first, ".") || importPath == "github.com/Tangerg/lynx/core" || strings.HasPrefix(importPath, "github.com/Tangerg/lynx/core/") {
				continue
			}
			t.Errorf("tool production file %s imports dependency above Core: %s", path, importPath)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk tool module: %v", walkErr)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller did not report test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
