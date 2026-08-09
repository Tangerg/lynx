package session

import (
	"fmt"
	"slices"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"

	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

func configuredKeys(configured settings.Settings) (*keymap.Map, error) {
	keys := headless.DefaultEditorKeys()
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
				return nil, fmt.Errorf("session settings: keys.%s contains invalid binding %q", name, encoded)
			}
			identity := sequence.String()
			if owner, duplicate := seen[identity]; duplicate && owner != name {
				return nil, fmt.Errorf("session settings: key %q is bound to both %s and %s", identity, owner, name)
			}
			seen[identity] = name
			keys.Bind(action, sequence...)
		}
	}
	return keys, nil
}
