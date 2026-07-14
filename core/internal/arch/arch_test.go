// Package arch holds architecture-fitness tests for the core module.
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetChatSPIExcludesDefaultsAndIdentity(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "chat")
	fset := token.NewFileSet()
	allowed := map[string]map[string]bool{
		"Model":    {"Call": true},
		"Streamer": {"Stream": true},
	}
	found := make(map[string]bool, len(allowed))

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read target chat package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if importPath == "github.com/Tangerg/lynx/core/model" || strings.HasPrefix(importPath, "github.com/Tangerg/lynx/core/model/") {
				t.Errorf("target core/chat must not depend on legacy generic model layer %q: %s", importPath, path)
			}
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				if typeSpec.Name.Name == "ModelMetadata" {
					t.Errorf("target core/chat must not expose provider identity type ModelMetadata: %s", path)
				}
				methods, tracked := allowed[typeSpec.Name.Name]
				if !tracked {
					continue
				}
				found[typeSpec.Name.Name] = true
				if typeSpec.TypeParams != nil && len(typeSpec.TypeParams.List) != 0 {
					t.Errorf("core/chat.%s must not use type parameters", typeSpec.Name.Name)
				}
				iface, ok := typeSpec.Type.(*ast.InterfaceType)
				if !ok {
					t.Errorf("core/chat.%s must remain an interface", typeSpec.Name.Name)
					continue
				}
				for _, field := range iface.Methods.List {
					if len(field.Names) == 0 {
						t.Errorf("core/chat.%s must not embed another interface", typeSpec.Name.Name)
						continue
					}
					for _, name := range field.Names {
						if !methods[name.Name] {
							t.Errorf("core/chat.%s must not require %s", typeSpec.Name.Name, name.Name)
						}
					}
				}
			}
		}
	}
	for name := range allowed {
		if !found[name] {
			t.Errorf("target core/chat.%s declaration not found", name)
		}
	}
}

func TestCoreDoesNotImportUpperLynxModules(t *testing.T) {
	const lynxPrefix = "github.com/Tangerg/lynx/"
	fset := token.NewFileSet()

	violations := 0
	for _, path := range productionGoFiles(t) {
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", path, err)
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
			rel, _ := filepath.Rel(moduleRoot(t), path)
			t.Errorf("core must not import upper lynx module %q: %s", ip, rel)
		}
	}
	if violations == 0 {
		t.Log("core import boundary holds: only core/pkg lynx imports found")
	}
}

func productionGoFiles(t *testing.T) []string {
	t.Helper()
	root := moduleRoot(t)
	var files []string
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
		files = append(files, path)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk core: %v", walkErr)
	}
	return files
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
