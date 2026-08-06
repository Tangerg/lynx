package interaction

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestInteractionDoesNotDependOnLegacyOrApplicationModules(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
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
				t.Errorf("%s imports forbidden legacy or application package %q", name, path)
			}
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
