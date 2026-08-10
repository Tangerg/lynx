package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

func TestConfigurationPrecedenceFileEnvironmentFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.yaml")
	if err := os.WriteFile(path, []byte("model: file-model\nmode: plan\npermission: read-only\neffort: low\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LYRA_MODEL", "environment-model")
	out, _, err := executeCommand(t, instantRuntime(), "", "--config", path, "--mode", "review", "config", "show")
	if err != nil {
		t.Fatalf("config show: %v", err)
	}
	var got settings.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("config show JSON: %v\n%s", err, out)
	}
	if got.Model != "environment-model" || got.Mode != "review" || got.Permission != "read-only" || got.Effort != "low" {
		t.Fatalf("effective settings = %+v", got)
	}
}

func TestConfigurationRegistersEnvironmentOnlyKeysForUnmarshal(t *testing.T) {
	t.Setenv("LYRA_UI_TRANSCRIPT_RETAIN", "77")
	t.Setenv("LYRA_UI_TOOL_DETAILS", "true")
	t.Setenv("LYRA_APPROVAL_REMEMBER", "project")
	out, _, err := executeCommand(t, instantRuntime(), "", "config", "show")
	if err != nil {
		t.Fatal(err)
	}
	var got settings.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.UI.TranscriptRetain != 77 || !got.UI.ToolDetails || got.Approval.Remember != "project" {
		t.Fatalf("environment settings = %+v", got)
	}
}

func TestConfigurationAcceptsRepeatablePluginDirectories(t *testing.T) {
	out, _, err := executeCommand(t, instantRuntime(), "", "--plugin-dir", "/plugins/one", "--plugin-dir", "/plugins/two", "config", "show")
	if err != nil {
		t.Fatal(err)
	}
	var got settings.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Plugins.Directories) != 2 || got.Plugins.Directories[0] != "/plugins/one" || got.Plugins.Directories[1] != "/plugins/two" {
		t.Fatalf("plugin directories = %v", got.Plugins.Directories)
	}
}

func TestConfigurationMergesPartialKeyOverridesWithDefaultActions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.yaml")
	if err := os.WriteFile(path, []byte("keys:\n  sessions: [g s]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := executeCommand(t, instantRuntime(), "", "--config", path, "config", "show")
	if err != nil {
		t.Fatal(err)
	}
	var got settings.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if bindings := got.Keys[settings.ActionSessions]; len(bindings) != 1 || bindings[0] != "g s" {
		t.Fatalf("session bindings = %v", bindings)
	}
	if bindings := got.Keys[settings.ActionShortcuts]; len(bindings) != 1 || bindings[0] != "ctrl+x" {
		t.Fatalf("default shortcut bindings = %v", bindings)
	}
}

func TestConfigurationRejectsInvalidValuesAndMissingExplicitFile(t *testing.T) {
	if _, _, err := executeCommand(t, instantRuntime(), "", "--mode", "magic", "config", "show"); err == nil {
		t.Fatal("invalid mode was accepted")
	}
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if _, _, err := executeCommand(t, instantRuntime(), "", "--config", missing, "config", "show"); err == nil {
		t.Fatal("missing explicit configuration was ignored")
	}
}

func TestConfigurationRejectsUnknownKeys(t *testing.T) {
	for name, content := range map[string]string{
		"top level": "unknown-setting: value\n",
		"nested":    "ui:\n  transcript-retian: 80\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := executeCommand(t, instantRuntime(), "", "--config", path, "config", "show"); err == nil {
				t.Fatal("unknown configuration key was ignored")
			}
		})
	}
}

func TestCompletionGenerationDoesNotDependOnConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(path, []byte("unknown: value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := executeCommand(t, instantRuntime(), "", "--config", path, "completion", "bash")
	if err != nil {
		t.Fatalf("completion generation read configuration: %v", err)
	}
	if !strings.Contains(out, "bash completion") {
		t.Fatalf("completion output is incomplete:\n%s", out)
	}
}
