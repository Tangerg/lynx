package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

func TestConfigurationPrecedenceFileEnvironmentFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.yaml")
	if err := os.WriteFile(path, []byte("model: file-model\nmode: plan\npermission: read-only\neffort: low\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LYRA_MODEL", "environment-model")
	out, _, err := exec(t, instant(), "", "--config", path, "--mode", "review", "config", "show")
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
	out, _, err := exec(t, instant(), "", "config", "show")
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
	out, _, err := exec(t, instant(), "", "--plugin-dir", "/plugins/one", "--plugin-dir", "/plugins/two", "config", "show")
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

func TestConfigurationRejectsInvalidValuesAndMissingExplicitFile(t *testing.T) {
	if _, _, err := exec(t, instant(), "", "--mode", "magic", "config", "show"); err == nil {
		t.Fatal("invalid mode was accepted")
	}
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if _, _, err := exec(t, instant(), "", "--config", missing, "config", "show"); err == nil {
		t.Fatal("missing explicit configuration was ignored")
	}
}
