// Package arch contains architecture fitness tests for the tiktoken adapter module.
package arch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleHasNoReplaceDirective(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\nreplace ") || strings.Contains(string(data), "\nreplace(") {
		t.Fatal("tiktoken go.mod must not contain replace directives")
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("go.mod not found")
		}
		directory = parent
	}
}
