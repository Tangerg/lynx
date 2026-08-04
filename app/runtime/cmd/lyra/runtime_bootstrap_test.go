package main

import (
	"errors"
	"testing"
)

func TestBootstrapRuntimeRejectsBuildIdentityFailureBeforeExternalSetup(t *testing.T) {
	want := errors.New("executable unreadable")
	_, _, err := bootstrapRuntimeWithBuildID(t.Context(), runtimePaths{
		userHome:             t.TempDir(),
		defaultWorkspacePath: t.TempDir(),
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

	paths, err := resolveRuntimePaths()
	if err != nil {
		t.Fatalf("resolveRuntimePaths: %v", err)
	}
	if paths.userHome != userHome || paths.defaultWorkspacePath != userHome {
		t.Fatalf("runtime paths = %+v, want user home and default workspace %q", paths, userHome)
	}
}

func TestResolveRuntimePathsRejectsMissingUserHome(t *testing.T) {
	t.Setenv("HOME", "")
	if _, err := resolveRuntimePaths(); err == nil {
		t.Fatal("resolveRuntimePaths accepted an unavailable user home")
	}
}
