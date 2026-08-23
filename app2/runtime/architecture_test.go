package runtime_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/Tangerg/lynx/app2/runtime"
const oldAppImportPrefix = "github.com/Tangerg/lynx/" + "app/"

type sourceImport struct {
	file string
	path string
}

func TestArchitectureDependencyDirections(t *testing.T) {
	root := runtimeRoot(t)
	imports := goImports(t, root)
	for _, value := range imports {
		relative, err := filepath.Rel(root, value.file)
		if err != nil {
			t.Fatal(err)
		}
		relative = filepath.ToSlash(relative)
		switch {
		case strings.HasPrefix(value.path, oldAppImportPrefix):
			t.Errorf("%s imports old app package %s", relative, value.path)
		case strings.HasPrefix(value.path, "github.com/Tangerg/lynx/agent") &&
			!strings.HasPrefix(relative, "agentexec/"):
			t.Errorf("%s bypasses the agentexec boundary with %s", relative, value.path)
		case strings.HasPrefix(relative, "domain/") && !allowedDomainImport(value.path):
			t.Errorf("domain source %s depends on non-domain package %s", relative, value.path)
		case strings.HasPrefix(relative, "protocol/") && !allowedProtocolImport(value.path):
			t.Errorf("protocol source %s depends on outer package %s", relative, value.path)
		case value.path == modulePath+"/sqlite" &&
			!strings.HasPrefix(relative, "runtimehost/") &&
			!strings.HasPrefix(relative, "sqlite/"):
			t.Errorf("%s bypasses the composition root to import sqlite", relative)
		}
	}
}

func TestNoOldAppOrRawWailsReferences(t *testing.T) {
	runtimeRoot := runtimeRoot(t)
	assertSourceTextAbsent(t, runtimeRoot, oldAppImportPrefix)

	frontendRoot := filepath.Join(runtimeRoot, "..", "desktop", "frontend", "src")
	err := filepath.WalkDir(frontendRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || (!strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx")) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(frontendRoot, path)
		if err != nil {
			return err
		}
		text := string(body)
		if strings.Contains(text, oldAppImportPrefix) {
			t.Errorf("frontend source %s references the old app", relative)
		}
		if strings.Contains(text, "@wailsio/runtime") && filepath.ToSlash(relative) != "runtime/desktopBridge.ts" {
			t.Errorf("frontend source %s bypasses desktopBridge", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func allowedDomainImport(path string) bool {
	return standardLibrary(path) ||
		strings.HasPrefix(path, modulePath+"/domain/") ||
		path == "github.com/robfig/cron/v3"
}

func allowedProtocolImport(path string) bool {
	return standardLibrary(path) || path == modulePath+"/internal/contractshape"
}

func standardLibrary(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

func runtimeRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate architecture test")
	}
	return filepath.Dir(file)
}

func goImports(t *testing.T, root string) []sourceImport {
	t.Helper()
	set := token.NewFileSet()
	values := make([]sourceImport, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(set, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			value, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			values = append(values, sourceImport{file: path, path: value})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return values
}

func assertSourceTextAbsent(t *testing.T, root, forbidden string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), forbidden) {
			t.Errorf("%s contains forbidden reference %q", path, forbidden)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
