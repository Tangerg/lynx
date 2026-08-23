package main

import (
	"path/filepath"
	"testing"
)

func TestDefaultSupervisorConfigUsesExplicitApp2Home(t *testing.T) {
	root := t.TempDir()
	runtimeBinary := filepath.Join(root, "lyra-runtime")
	t.Setenv("LYRA2_USER_HOME", root)
	t.Setenv("LYRA2_RUNTIME_BINARY", runtimeBinary)

	config, err := defaultSupervisorConfig()
	if err != nil {
		t.Fatalf("defaultSupervisorConfig() error = %v", err)
	}
	if config.UserHome != root || config.DefaultWorkspace != root {
		t.Fatalf("home projection = user %q workspace %q, want %q", config.UserHome, config.DefaultWorkspace, root)
	}
	if config.DataHome != filepath.Join(root, ".lyra-app2") {
		t.Fatalf("DataHome = %q", config.DataHome)
	}
	if config.RuntimeBinary != runtimeBinary {
		t.Fatalf("RuntimeBinary = %q", config.RuntimeBinary)
	}
}

func TestDefaultSupervisorConfigRejectsRelativeOverrides(t *testing.T) {
	t.Setenv("LYRA2_USER_HOME", "relative/home")
	t.Setenv("LYRA2_RUNTIME_BINARY", "/tmp/lyra-runtime")
	if _, err := defaultSupervisorConfig(); err == nil {
		t.Fatal("relative LYRA2_USER_HOME was accepted")
	}
}
