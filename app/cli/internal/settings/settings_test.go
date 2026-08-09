package settings

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func TestDefaultIsValidAndCloned(t *testing.T) {
	defaults := Default()
	if err := defaults.Validate(); err != nil {
		t.Fatalf("Default.Validate: %v", err)
	}
	clone := defaults.Clone()
	clone.Keys[ActionQuit][0] = "q"
	defaults.Plugins.Directories = []string{"one"}
	clone = defaults.Clone()
	clone.Plugins.Directories[0] = "two"
	if defaults.Keys[ActionQuit][0] != "ctrl+c" {
		t.Fatal("Clone leaked a keybinding slice")
	}
	if defaults.Plugins.Directories[0] != "one" {
		t.Fatal("Clone leaked a plugin directory slice")
	}
	if options := defaults.RunOptions(); options.Model != "mock-balanced" || options.Mode != client.ModeBuild {
		t.Fatalf("RunOptions = %+v", options)
	}
}

func TestValidationReportsAllIndependentProblems(t *testing.T) {
	settings := Default()
	settings.Model = ""
	settings.Mode = "magic"
	settings.Permission = "unsafe"
	settings.UI.TranscriptRetain = 0
	settings.Plugins.Directories = []string{"", "/plugins", "/plugins"}
	settings.Keys["unknown"] = []string{""}
	err := settings.Validate()
	if err == nil {
		t.Fatal("invalid settings were accepted")
	}
	for _, want := range []string{"model is required", "mode", "permission", "transcript-retain", "empty path", "repeats", "unknown", "empty binding"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("validation error %q does not mention %q", err, want)
		}
	}
}
