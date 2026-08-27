package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestBootstrapRuntimeRejectsBuildIdentityFailureBeforeExternalSetup(t *testing.T) {
	want := errors.New("executable unreadable")
	_, _, err := bootstrapRuntimeWithBuildID(t.Context(), runtimePaths{
		userHome:             t.TempDir(),
		defaultWorkspacePath: t.TempDir(),
		dataDirectory:        t.TempDir(),
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
	t.Setenv("LYRA_HOME", "")

	paths, err := resolveRuntimePaths()
	if err != nil {
		t.Fatalf("resolveRuntimePaths: %v", err)
	}
	if paths.userHome != userHome || paths.defaultWorkspacePath != userHome ||
		paths.dataDirectory != filepath.Join(userHome, ".lyra") || !filepath.IsAbs(paths.launchDirectory) {
		t.Fatalf("runtime paths = %+v, want user home and default workspace %q", paths, userHome)
	}
}

func TestResolveRuntimePathsUsesExplicitAbsoluteDataDirectory(t *testing.T) {
	userHome := t.TempDir()
	dataDirectory := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("LYRA_HOME", dataDirectory)

	paths, err := resolveRuntimePaths()
	if err != nil {
		t.Fatalf("resolveRuntimePaths: %v", err)
	}
	if paths.dataDirectory != dataDirectory {
		t.Fatalf("data directory = %q, want %q", paths.dataDirectory, dataDirectory)
	}
}

func TestResolveRuntimePathsRejectsRelativeDataDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LYRA_HOME", "relative/data")
	if _, err := resolveRuntimePaths(); err == nil {
		t.Fatal("resolveRuntimePaths accepted a relative LYRA_HOME")
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
