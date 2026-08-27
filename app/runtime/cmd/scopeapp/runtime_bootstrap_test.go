package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/scope/app/runtime/localruntime"
)

func mustDataDirectory(t *testing.T, path string) localruntime.DataDirectory {
	t.Helper()
	directory, err := localruntime.DataDirectoryAt(path)
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestBootstrapRuntimeRejectsBuildIdentityFailureBeforeExternalSetup(t *testing.T) {
	want := errors.New("executable unreadable")
	_, _, err := bootstrapRuntimeWithBuildID(t.Context(), runtimePaths{
		userHome:             t.TempDir(),
		defaultWorkspacePath: t.TempDir(),
		dataDirectory:        mustDataDirectory(t, t.TempDir()),
	}, func() (string, error) {
		return "", want
	})
	if !errors.Is(err, want) {
		t.Fatalf("bootstrapRuntimeWithBuildID error = %v, want %v", err, want)
	}
}

func TestResolveRuntimePathsUsesOneUserHomeSnapshot(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("SCOPEAPP_HOME", "")

	paths, err := resolveRuntimePaths()
	if err != nil {
		t.Fatalf("resolveRuntimePaths: %v", err)
	}
	if paths.userHome != userHome || paths.defaultWorkspacePath != userHome ||
		paths.dataDirectory.Path() != filepath.Join(userHome, ".scopeapp") || !filepath.IsAbs(paths.launchDirectory) {
		t.Fatalf("runtime paths = %+v, want user home and default workspace %q", paths, userHome)
	}
}

func TestResolveRuntimePathsUsesExplicitAbsoluteDataDirectory(t *testing.T) {
	userHome := t.TempDir()
	dataDirectory := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("SCOPEAPP_HOME", dataDirectory)

	paths, err := resolveRuntimePaths()
	if err != nil {
		t.Fatalf("resolveRuntimePaths: %v", err)
	}
	if paths.dataDirectory.Path() != dataDirectory {
		t.Fatalf("data directory = %q, want %q", paths.dataDirectory.Path(), dataDirectory)
	}
}

func TestResolveRuntimePathsRejectsRelativeDataDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SCOPEAPP_HOME", "relative/data")
	if _, err := resolveRuntimePaths(); err == nil {
		t.Fatal("resolveRuntimePaths accepted a relative SCOPEAPP_HOME")
	}
}

func TestResolveRuntimePathsRejectsMissingUserHome(t *testing.T) {
	t.Setenv("HOME", "")
	if _, err := resolveRuntimePaths(); err == nil {
		t.Fatal("resolveRuntimePaths accepted an unavailable user home")
	}
}

func TestResolveRuntimePathsRejectsRelativeUserHome(t *testing.T) {
	t.Setenv("HOME", "relative-home")
	if _, err := resolveRuntimePaths(); err == nil {
		t.Fatal("resolveRuntimePaths accepted a relative user home")
	}
}
