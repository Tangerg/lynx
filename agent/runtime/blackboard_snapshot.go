package runtime

import (
	"maps"
	"slices"
)

// BlackboardSnapshotter is the optional capture surface a custom
// [core.Blackboard] implementation exposes so [AgentProcess.Snapshot] can
// persist its full state. The three returned values mirror
// [core.ProcessSnapshot]'s Blackboard / Conditions / Objects fields.
// Implementations are free to return nil for any value.
type BlackboardSnapshotter interface {
	Snapshot() (named map[string]any, conditions map[string]bool, objects []any)
}

// BlackboardRestorer is the optional restore surface. The runtime passes back
// whatever [BlackboardSnapshotter.Snapshot] previously produced.
// Implementations may apply selective filtering.
type BlackboardRestorer interface {
	Restore(named map[string]any, conditions map[string]bool, objects []any)
}

// Snapshot implements [BlackboardSnapshotter]. Hidden + protected markers are
// deliberately omitted: protected re-applies naturally at restore time, and
// Hide markers are a transient view filter with no portable wire form.
func (b *inMemoryBlackboard) Snapshot() (map[string]any, map[string]bool, []any) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return maps.Clone(b.named), maps.Clone(b.conditions), slices.Clone(b.objects)
}

// Restore implements [BlackboardRestorer]. Existing bindings are cleared first;
// protected / hidden markers are reset because they have no portable wire form.
func (b *inMemoryBlackboard) Restore(named map[string]any, conditions map[string]bool, objects []any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	clear(b.named)
	maps.Copy(b.named, named)
	clear(b.conditions)
	maps.Copy(b.conditions, conditions)
	b.objects = slices.Clone(objects)
	b.hidden = b.hidden[:0]
	clear(b.protected)
}
