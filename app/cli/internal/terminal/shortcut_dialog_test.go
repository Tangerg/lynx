package terminal

import (
	"testing"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

func TestCollectShortcutRowsUsesProvidedKeymaps(t *testing.T) {
	applicationKeys := &keymap.Map{}
	applicationKeys.Bind(showShortcuts, input.Chord{Code: input.F1})
	transcriptKeys := &keymap.Map{}
	transcriptKeys.Bind(commandPalette, input.Chord{Code: input.Character, Rune: '!'})

	got := collectShortcutRows(applicationKeys, transcriptKeys, nil)
	if len(got) != 2 {
		t.Fatalf("collectShortcutRows returned %d entries, want only the two bound actions: %+v", len(got), got)
	}
	if got[0].area != "Application" || got[0].bindings != "f1" || got[0].description != "open this shortcut guide" {
		t.Fatalf("application shortcut = %+v", got[0])
	}
	if got[1].area != "Transcript" || got[1].bindings != "!" || got[1].description != "open command palette" {
		t.Fatalf("transcript shortcut = %+v", got[1])
	}
}
