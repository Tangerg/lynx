package agent2

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
		}
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok {
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
