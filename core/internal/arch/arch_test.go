// Package arch holds architecture-fitness tests for the core module.
package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreDoesNotImportUpperLynxModules(t *testing.T) {
	const lynxPrefix = "github.com/Tangerg/lynx/"
	root := moduleRoot(t)
	fset := token.NewFileSet()

	violations := 0
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			ip := strings.Trim(imp.Path.Value, `"`)
			rest, ok := strings.CutPrefix(ip, lynxPrefix)
			if !ok {
				continue
			}
			if strings.HasPrefix(rest, "core/") || rest == "core" || strings.HasPrefix(rest, "pkg/") || rest == "pkg" {
				continue
			}
			violations++
			rel, _ := filepath.Rel(root, path)
			t.Errorf("core must not import upper lynx module %q: %s", ip, rel)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk core: %v", walkErr)
	}
	if violations == 0 {
		t.Log("core import boundary holds: only core/pkg lynx imports found")
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from test dir")
		}
		dir = parent
	}
}
