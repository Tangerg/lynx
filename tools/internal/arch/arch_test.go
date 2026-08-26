package arch_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRootToolsStayPackageFree(t *testing.T) {
	root := toolsRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" {
			t.Errorf("tools is a module namespace, not an importable package: %s", entry.Name())
		}
	}
}

func toolsRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller did not report test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
