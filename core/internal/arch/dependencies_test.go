package arch

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

var temporaryExternalImports = map[string]string{
	"github.com/invopop/jsonschema": "P6-05",
}

func TestExternalImportsDoNotExceedMigrationBudget(t *testing.T) {
	fset := token.NewFileSet()
	for _, path := range productionGoFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if strings.HasPrefix(importPath, "github.com/Tangerg/lynx/core") || isStandardImport(importPath) {
				continue
			}
			deadline, ok := temporaryExternalImports[importPath]
			if !ok {
				rel, _ := filepath.Rel(moduleRoot(t), path)
				t.Errorf("external import %q in %s is outside the migration budget", importPath, rel)
				continue
			}
			t.Logf("temporary external import %s remains until %s", importPath, deadline)
		}
	}
}

func isStandardImport(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}
