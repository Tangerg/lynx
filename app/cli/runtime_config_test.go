package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeConfigDirectoriesPreferExplicitSource(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	explicitDirectory := t.TempDir()
	t.Setenv(runtimeConfigDirectoryEnvironment, explicitDirectory)

	directories, err := runtimeConfigDirectories(runtimeDirectory)
	if err != nil {
		t.Fatalf("runtimeConfigDirectories: %v", err)
	}
	want := []string{explicitDirectory, runtimeDirectory}
	if len(directories) != len(want) || directories[0] != want[0] || directories[1] != want[1] {
		t.Fatalf("directories = %v, want %v", directories, want)
	}
}

func TestRuntimeConfigDirectoriesRejectRelativeExplicitSource(t *testing.T) {
	t.Setenv(runtimeConfigDirectoryEnvironment, "relative/config")
	if _, err := runtimeConfigDirectories(filepath.Join(t.TempDir(), "runtime")); err == nil {
		t.Fatal("relative runtime config directory was accepted")
	}
}

func TestRuntimeConfigDirectoriesFindWorktreeDevelopmentConfig(t *testing.T) {
	t.Setenv(runtimeConfigDirectoryEnvironment, "")
	root := t.TempDir()
	configDirectory := filepath.Join(root, "app", "runtime", "config")
	cliDirectory := filepath.Join(root, "app", "cli")
	for _, directory := range []string{configDirectory, cliDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(root, "go.work"):                  "go 1.27.0\n",
		filepath.Join(root, "app", "runtime", "go.mod"): "module example/runtime\n",
		filepath.Join(configDirectory, "config.yaml"):   "provider: deepseek\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(cliDirectory)

	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	directories, err := runtimeConfigDirectories(runtimeDirectory)
	if err != nil {
		t.Fatalf("runtimeConfigDirectories: %v", err)
	}
	want := []string{runtimeDirectory, configDirectory}
	if len(directories) != len(want) || directories[0] != want[0] || directories[1] != want[1] {
		t.Fatalf("directories = %v, want %v", directories, want)
	}
}
