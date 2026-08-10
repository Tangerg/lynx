package terminal

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"

	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

type keyBindings struct {
	editor      *keymap.Map
	application *keymap.Map
	global      *keymap.Map
}

func configuredKeyBindings(configured settings.Config) (keyBindings, error) {
	bindings := keyBindings{
		editor:      headless.DefaultEditorKeys(),
		application: &keymap.Map{},
		global:      &keymap.Map{},
	}
	actions := slices.Sorted(func(yield func(string) bool) {
		for action := range configured.Keys {
			if !yield(action) {
				return
			}
		}
	})
	seen := make(map[string]string)
	for _, name := range actions {
		action := keymap.Action(name)
		for _, encoded := range configured.Keys[name] {
			sequence, ok := input.ParseKeys(encoded)
			if !ok {
				return keyBindings{}, fmt.Errorf("session settings: keys.%s contains invalid binding %q", name, encoded)
			}
			identity := sequence.String()
			if owner, duplicate := seen[identity]; duplicate && owner != name {
				return keyBindings{}, fmt.Errorf("session settings: key %q is bound to both %s and %s", identity, owner, name)
			}
			seen[identity] = name
			bindings.editor.Bind(action, sequence...)
			bindings.application.Bind(action, sequence...)
			if action == cancelRun || action == quitApp {
				bindings.global.Bind(action, sequence...)
			}
		}
	}
	return bindings, nil
}

func (b keyBindings) setResolver(resolve keymap.Resolver) {
	for _, keys := range []*keymap.Map{b.editor, b.application, b.global} {
		if keys != nil {
			keys.Resolve = resolve
		}
	}
}

func formatKeyBindings(keys *keymap.Map, action keymap.Action, separator string) string {
	sequences := keys.Keys(action)
	names := make([]string, len(sequences))
	for i, sequence := range sequences {
		names[i] = sequence.String()
	}
	return strings.Join(names, separator)
}
