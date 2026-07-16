package runtime

import (
	"maps"

	"github.com/Tangerg/lynx/agent/core"
)

// Clone produces a child blackboard inheriting the parent's full state: named
// keys, protected entries, conditions, the objects list, and the hidden
// markers. Visibility is part of the inherited state for live child processes.
func (b *inMemoryBlackboard) Clone() core.Blackboard {
	b.mu.RLock()
	defer b.mu.RUnlock()

	child := newInMemoryBlackboard()
	maps.Copy(child.named, b.named)
	maps.Copy(child.transientNamed, b.transientNamed)
	maps.Copy(child.protected, b.protected)
	maps.Copy(child.conditions, b.conditions)
	child.objects = append(child.objects, b.objects...)
	child.durableObjects = append(child.durableObjects, b.durableObjects...)
	child.hidden = append(child.hidden, b.hidden...)
	return child
}

// ClearWorkingState removes ordinary state while preserving protected entries.
func (b *inMemoryBlackboard) ClearWorkingState() {
	b.mu.Lock()
	defer b.mu.Unlock()

	preserved := make(map[string]any, len(b.protected))
	for key := range b.protected {
		if value, ok := b.named[key]; ok {
			preserved[key] = value
		}
	}

	clear(b.named)
	maps.Copy(b.named, preserved)
	clear(b.transientNamed)
	b.objects = b.objects[:0]
	b.durableObjects = b.durableObjects[:0]
	b.hidden = b.hidden[:0]
	clear(b.conditions)
}
