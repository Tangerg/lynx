package runtime

import (
	"maps"
	"slices"
)

// BlackboardSnapshotter is the optional capture surface a custom
// [core.Blackboard] implementation exposes so [Process.Snapshot] can
// persist its full state. The three returned values mirror
// [core.ProcessSnapshot]'s Blackboard / Conditions / Objects fields.
// Implementations are free to return nil for any value.
type BlackboardSnapshotter interface {
	Snapshot() (named map[string]any, conditions map[string]bool, objects []any, err error)
}

// BlackboardRestorer is the optional restore surface. The runtime passes back
// whatever [BlackboardSnapshotter.Snapshot] previously produced.
// Implementations may apply selective filtering.
type BlackboardRestorer interface {
	Restore(named map[string]any, conditions map[string]bool, objects []any) error
}

// Snapshot implements [BlackboardSnapshotter]. Hidden + protected markers are
// deliberately omitted: protected re-applies naturally at restore time, and
// Hide markers are a transient view filter with no portable wire form.
func (b *inMemoryBlackboard) Snapshot() (map[string]any, map[string]bool, []any, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	named := make(map[string]any, len(b.named)-len(b.transientNamed))
	for key, value := range b.named {
		if _, transient := b.transientNamed[key]; !transient {
			named[key] = value
		}
	}
	objects := make([]any, 0, len(b.objects))
	for i, value := range b.objects {
		if i < len(b.durableObjects) && b.durableObjects[i] {
			objects = append(objects, value)
		}
	}
	return named, maps.Clone(b.conditions), objects, nil
}

// Restore implements [BlackboardRestorer]. Existing bindings are cleared first;
// protected / hidden markers are reset because they have no portable wire form.
func (b *inMemoryBlackboard) Restore(named map[string]any, conditions map[string]bool, objects []any) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	clear(b.named)
	maps.Copy(b.named, named)
	clear(b.transientNamed)
	clear(b.conditions)
	maps.Copy(b.conditions, conditions)
	b.objects = slices.Clone(objects)
	b.durableObjects = make([]bool, len(objects))
	for i := range b.durableObjects {
		b.durableObjects[i] = true
	}
	b.hidden = b.hidden[:0]
	clear(b.protected)
	return nil
}
