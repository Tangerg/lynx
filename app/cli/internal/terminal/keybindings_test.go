package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"

	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

func TestConfiguredKeysBindProductActions(t *testing.T) {
	configured := settings.Default()
	configured.Keys[settings.ActionSessions] = []string{"g s"}
	keys, err := configuredKeys(configured)
	if err != nil {
		t.Fatal(err)
	}
	var matcher keymap.Matcher
	var got keymap.Action
	for _, chord := range []input.Chord{{Rune: 'g', Code: input.Character}, {Rune: 's', Code: input.Character}} {
		matcher.Handle(keys, input.Key{Code: chord.Code, Rune: chord.Rune}, func(action keymap.Action) bool {
			got = action
			return true
		})
	}
	if got != showSessions {
		t.Fatalf("sequence resolved to %q, want %q", got, showSessions)
	}
}

func TestConfiguredKeysExposeMultilineAndTranscriptNavigation(t *testing.T) {
	keys, err := configuredKeys(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		chord input.Chord
		want  keymap.Action
	}{
		{chord: input.Chord{Code: input.Enter, Mods: input.Shift}, want: headless.InsertNewline},
		{chord: input.Chord{Code: input.PageUp}, want: scrollPageUp},
		{chord: input.Chord{Code: input.PageDown}, want: scrollPageDown},
		{chord: input.Chord{Code: input.Home, Mods: input.Ctrl}, want: scrollTop},
		{chord: input.Chord{Code: input.End, Mods: input.Ctrl}, want: scrollBottom},
		{chord: input.Chord{Code: input.Character, Rune: ';', Mods: input.Ctrl}, want: manageQueue},
		{chord: input.Chord{Code: input.Character, Rune: 'g', Mods: input.Ctrl}, want: manageQueue},
		{chord: input.Chord{Code: input.Character, Rune: 'x', Mods: input.Ctrl}, want: showShortcuts},
	}
	for _, test := range tests {
		got, ok := keys.Action(test.chord)
		if !ok || got != test.want {
			t.Errorf("binding %s = %q, %v; want %q", test.chord, got, ok, test.want)
		}
	}
}

func TestConfiguredKeysRejectInvalidAndDuplicateBindings(t *testing.T) {
	configured := settings.Default()
	configured.Keys[settings.ActionQuit] = []string{"not-a-key"}
	if _, err := configuredKeys(configured); err == nil || !strings.Contains(err.Error(), "invalid binding") {
		t.Fatalf("invalid binding error = %v", err)
	}
	configured = settings.Default()
	configured.Keys[settings.ActionQuit] = []string{"ctrl+r"}
	if _, err := configuredKeys(configured); err == nil || !strings.Contains(err.Error(), "both") {
		t.Fatalf("duplicate binding error = %v", err)
	}
}
