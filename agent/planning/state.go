package planning

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

// State is the canonical immutable WorldState. Apply
// produces a new value; the receiver is never mutated. The key is
// computed eagerly at construction so concurrent readers (parallel-tick
// runners share a snapshot via [core.ProcessView.WorldState]) never
// race on lazy initialisation.
type State struct {
	conditions core.ConditionSet
	timestamp  time.Time
	key        string
}

// NewState seeds a state from a truth map. The
// map is copied defensively so subsequent mutations don't ripple in.
func NewState(conditions core.ConditionSet) *State {
	return newState(maps.Clone(conditions))
}

func newState(conditions core.ConditionSet) *State {
	state := &State{conditions: conditions, timestamp: time.Now()}
	state.key = state.deriveKey()
	return state
}

// Conditions returns a defensive copy of the underlying map — A* mutates
// result maps elsewhere, so the live map is never handed out.
func (s *State) Conditions() core.ConditionSet {
	return maps.Clone(s.conditions)
}

func (s *State) Timestamp() time.Time { return s.timestamp }

// Key returns the stable serialization of the sorted condition values.
// pairs separated by '|'. Computed eagerly at construction — see the
// type-level doc.
func (s *State) Key() string { return s.key }

// Apply layers effects on top, returning a new state. Effect map
// entries equal to Unknown are skipped — Unknown is "no information"
// and shouldn't override a definite value already in the state.
func (s *State) Apply(effects core.ConditionSet) core.WorldState {
	if len(effects) == 0 {
		return s
	}

	merged := make(core.ConditionSet, len(s.conditions)+len(effects))
	maps.Copy(merged, s.conditions)
	for key, truth := range effects {
		if truth == core.Unknown {
			continue
		}
		merged[key] = truth
	}
	return newState(merged)
}

// deriveKey produces a stable string identifier from the state
// map: sorted "key=det|" pairs, with Unknown entries elided so
// explicit Unknown and absent entries hash identically.
func (s *State) deriveKey() string {
	var builder strings.Builder
	for _, key := range slices.Sorted(maps.Keys(s.conditions)) {
		truth := s.conditions[key]
		if truth == core.Unknown {
			continue
		}
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(truth.String())
		builder.WriteByte('|')
	}
	return builder.String()
}
