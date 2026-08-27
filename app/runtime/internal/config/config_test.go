package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesOnlyExplicitAbsoluteSearchDirectories(t *testing.T) {
	if _, err := Load([]string{"relative/config"}); err == nil {
		t.Fatal("Load accepted a relative config search directory")
	}

	directory := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(directory, "config.yaml"),
		[]byte("provider: anthropic\nmodel: explicit-model\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SCOPEAPP_PROVIDER", "")
	t.Setenv("SCOPEAPP_MODEL", "")
	settings, err := Load([]string{directory})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.Provider != "anthropic" || settings.Model != "explicit-model" {
		t.Fatalf("settings = %+v, want explicit config file values", settings)
	}
}
