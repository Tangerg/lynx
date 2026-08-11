package settings

import (
	"strings"
	"testing"
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
	if defaults.Keys[ActionQuit][0] != "ctrl+q" {
		t.Fatal("Clone leaked a keybinding slice")
	}
	if defaults.Plugins.Directories[0] != "one" {
		t.Fatal("Clone leaked a plugin directory slice")
	}
	if options := defaults.RunOptions(); options.Provider != DefaultProvider || options.Model != DefaultModel {
		t.Fatalf("RunOptions = %+v", options)
	}
	if defaults.Approval.Remember != RememberNone {
		t.Fatalf("default approval remember = %q", defaults.Approval.Remember)
	}
	if got := defaults.Keys[ActionManageQueue]; len(got) != 2 || got[0] != "ctrl+;" || got[1] != "ctrl+g" {
		t.Fatalf("manage queue bindings = %v", got)
	}
	if got := defaults.Keys[ActionShortcuts]; len(got) != 1 || got[0] != "ctrl+x" {
		t.Fatalf("shortcut bindings = %v", got)
	}
	if got := defaults.Keys[ActionCancelRun]; len(got) != 1 || got[0] != "ctrl+c" {
		t.Fatalf("cancel bindings = %v", got)
	}
}

func TestValidationReportsAllIndependentProblems(t *testing.T) {
	settings := Default()
	settings.Provider = "mock"
	settings.Model = ""
	settings.Run.MaxSteps = -1
	settings.UI.TranscriptRetain = 0
	settings.Plugins.Directories = []string{"", "/plugins", "/plugins"}
	delete(settings.Keys, ActionShortcuts)
	settings.Keys["unknown"] = []string{""}
	err := settings.Validate()
	if err == nil {
		t.Fatal("invalid settings were accepted")
	}
	for _, want := range []string{"selected together", "non-negative", "transcript-retain", "empty path", "repeats", "shortcuts is missing", "unknown", "empty binding"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("validation error %q does not mention %q", err, want)
		}
	}
}
