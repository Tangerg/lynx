package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopHostBootstrap(t *testing.T) {
	home := t.TempDir()
	host := newDesktopHost(home)
	if err := os.MkdirAll(filepath.Join(home, ".lyra", "plugins", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".lyra", "local-token"), []byte("token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".lyra", "plugins", "alpha", "index.js"), []byte("export default {};"), 0o600); err != nil {
		t.Fatal(err)
	}

	bootstrap, err := host.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.LocalRuntime.Endpoint != localRuntimeEndpoint || bootstrap.LocalRuntime.LocalToken != "token" {
		t.Fatalf("local runtime = %#v", bootstrap.LocalRuntime)
	}
	if len(bootstrap.SideloadedPlugins) != 1 || bootstrap.SideloadedPlugins[0].ID != "alpha" {
		t.Fatalf("plugins = %#v", bootstrap.SideloadedPlugins)
	}
}

func TestDesktopHostBootstrapAllowsMissingState(t *testing.T) {
	bootstrap, err := newDesktopHost(t.TempDir()).Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.LocalRuntime.LocalToken != "" {
		t.Fatalf("local token = %q, want empty", bootstrap.LocalRuntime.LocalToken)
	}
	if bootstrap.SideloadedPlugins == nil || bootstrap.SideloadIssues == nil {
		t.Fatalf("empty collections must encode as arrays: %#v", bootstrap)
	}
}
