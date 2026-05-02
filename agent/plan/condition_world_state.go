// Package plan defines the planner-facing types: WorldState
// implementations, the Plan struct, the PlanningSystem (the agent
// capability set the planner reasons over), and the Planner interface.
// Concrete planners live in agent/planner/<algo>.
package plan

import (
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

// ConditionWorldState is the canonical immutable WorldState. Apply produces
// a new value; the receiver is never mutated.
type ConditionWorldState struct {
	stateMap  map[string]core.Determination
	timestamp time.Time
	hashCache string
}

// NewConditionWorldState seeds a state from a determination map. The map
// is copied defensively so subsequent mutations don't ripple in.
func NewConditionWorldState(state map[string]core.Determination) *ConditionWorldState {
	out := &ConditionWorldState{
		stateMap:  make(map[string]core.Determination, len(state)),
		timestamp: core.Now(),
	}
	maps.Copy(out.stateMap, state)
	return out
}

// EmptyWorldState is the all-Unknown starting point used at process start.
func EmptyWorldState() *ConditionWorldState {
	return NewConditionWorldState(nil)
}

// State returns a defensive copy of the underlying map — A* mutates result
// maps elsewhere, so we never hand out the live one.
func (w *ConditionWorldState) State() map[string]core.Determination {
	out := make(map[string]core.Determination, len(w.stateMap))
	maps.Copy(out, w.stateMap)
	return out
}

func (w *ConditionWorldState) Timestamp() time.Time { return w.timestamp }

// HashKey is computed lazily on first access — it's a stable serialization
// of (sorted) (key, det) pairs separated by '|'. Two ConditionWorldStates
// with the same condition map produce identical keys, which is exactly
// what A*'s closed set needs.
func (w *ConditionWorldState) HashKey() string {
	if w.hashCache != "" {
		return w.hashCache
	}

	keys := make([]string, 0, len(w.stateMap))
	for k := range w.stateMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		v := w.stateMap[k]
		if v == core.Unknown {
			// Unknown is the implicit default — skipping it keeps the key
			// compact and stable across "absent" vs "explicitly Unknown".
			continue
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(v.String())
		b.WriteByte('|')
	}
	w.hashCache = b.String()
	return w.hashCache
}

// Apply layers effects on top, returning a new state. Effect map entries
// equal to Unknown are skipped — Unknown is "no information" and shouldn't
// override a definite value already in the state.
func (w *ConditionWorldState) Apply(effects core.EffectSpec) core.WorldState {
	if len(effects) == 0 {
		return w
	}

	out := &ConditionWorldState{
		stateMap:  make(map[string]core.Determination, len(w.stateMap)+len(effects)),
		timestamp: core.Now(),
	}
	maps.Copy(out.stateMap, w.stateMap)
	for k, v := range effects {
		if v == core.Unknown {
			continue
		}
		out.stateMap[k] = v
	}
	return out
}
