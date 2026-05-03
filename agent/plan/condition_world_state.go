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

// ConditionWorldState is the canonical immutable WorldState. Apply
// produces a new value; the receiver is never mutated. The hashKey is
// computed eagerly at construction so concurrent readers (parallel-tick
// runners share a snapshot via [core.Process.LastWorldState]) never
// race on lazy initialisation.
type ConditionWorldState struct {
	stateMap  map[string]core.Determination
	timestamp time.Time
	hashKey   string
}

// NewConditionWorldState seeds a state from a determination map. The
// map is copied defensively so subsequent mutations don't ripple in.
func NewConditionWorldState(state map[string]core.Determination) *ConditionWorldState {
	cloned := make(map[string]core.Determination, len(state))
	maps.Copy(cloned, state)
	return &ConditionWorldState{
		stateMap:  cloned,
		timestamp: core.Now(),
		hashKey:   computeHashKey(cloned),
	}
}

// EmptyWorldState is the all-Unknown starting point — convenience for
// tests and integrations that want a blank state without going through
// a map literal.
func EmptyWorldState() *ConditionWorldState { return NewConditionWorldState(nil) }

// State returns a defensive copy of the underlying map — A* mutates
// result maps elsewhere, so we never hand out the live one.
func (w *ConditionWorldState) State() map[string]core.Determination {
	out := make(map[string]core.Determination, len(w.stateMap))
	maps.Copy(out, w.stateMap)
	return out
}

func (w *ConditionWorldState) Timestamp() time.Time { return w.timestamp }

// HashKey returns the stable serialisation of the (sorted) (key, det)
// pairs separated by '|'. Computed eagerly at construction — see the
// type-level doc.
func (w *ConditionWorldState) HashKey() string { return w.hashKey }

// Apply layers effects on top, returning a new state. Effect map
// entries equal to Unknown are skipped — Unknown is "no information"
// and shouldn't override a definite value already in the state.
func (w *ConditionWorldState) Apply(effects core.EffectSpec) core.WorldState {
	if len(effects) == 0 {
		return w
	}

	merged := make(map[string]core.Determination, len(w.stateMap)+len(effects))
	maps.Copy(merged, w.stateMap)
	for k, v := range effects {
		if v == core.Unknown {
			continue
		}
		merged[k] = v
	}
	return &ConditionWorldState{
		stateMap:  merged,
		timestamp: core.Now(),
		hashKey:   computeHashKey(merged),
	}
}

// computeHashKey produces a stable string identifier from a state map:
// sorted "key=det|" pairs, with Unknown entries elided so explicit
// Unknown and absent entries hash identically.
func computeHashKey(state map[string]core.Determination) string {
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		v := state[k]
		if v == core.Unknown {
			continue
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(v.String())
		b.WriteByte('|')
	}
	return b.String()
}
