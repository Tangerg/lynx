package planning

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// WorldState is an immutable, canonical observation of known condition truths.
// Missing conditions read as Unknown. Its zero value is the valid empty state.
type WorldState struct {
	conditions []Condition
}

// NewWorldState validates, sorts, and freezes known conditions. A condition key
// may appear only once.
func NewWorldState(conditions ...Condition) (WorldState, error) {
	values := slices.Clone(conditions)
	for index, condition := range values {
		if !condition.Valid() {
			return WorldState{}, fmt.Errorf("%w: condition %d", ErrInvalidWorldState, index)
		}
	}
	slices.SortFunc(values, func(left, right Condition) int {
		return strings.Compare(left.key, right.key)
	})
	for index := 1; index < len(values); index++ {
		if values[index-1].key == values[index].key {
			return WorldState{}, fmt.Errorf("%w: duplicate condition %q", ErrInvalidWorldState, values[index].key)
		}
	}
	return WorldState{conditions: values}, nil
}

// Conditions returns an independently owned, key-sorted snapshot.
func (state WorldState) Conditions() []Condition { return slices.Clone(state.conditions) }

// Truth returns the observed truth for key, or Unknown when key is absent.
func (state WorldState) Truth(key string) Truth {
	index, found := slices.BinarySearchFunc(state.conditions, key, func(condition Condition, key string) int {
		return strings.Compare(condition.key, key)
	})
	if !found {
		return Unknown
	}
	return state.conditions[index].truth
}

// Satisfies reports whether state establishes every required condition.
func (state WorldState) Satisfies(requirements ...Condition) bool {
	for _, requirement := range requirements {
		if !requirement.Valid() || state.Truth(requirement.key) != requirement.truth {
			return false
		}
	}
	return true
}

// Apply returns a new state with predicted effects layered over this state.
// The receiver is never mutated.
func (state WorldState) Apply(effects ...Condition) (WorldState, error) {
	if !state.Valid() {
		return WorldState{}, ErrInvalidWorldState
	}
	values := make(map[string]Truth, len(state.conditions)+len(effects))
	for _, condition := range state.conditions {
		values[condition.key] = condition.truth
	}
	for index, effect := range effects {
		if !effect.Valid() {
			return WorldState{}, fmt.Errorf("%w: effect %d", ErrInvalidWorldState, index)
		}
		values[effect.key] = effect.truth
	}
	conditions := make([]Condition, 0, len(values))
	for key, truth := range values {
		conditions = append(conditions, Condition{key: key, truth: truth})
	}
	return NewWorldState(conditions...)
}

// Key returns a stable identity derived only from canonical known truths.
func (state WorldState) Key() string {
	var key strings.Builder
	for _, condition := range state.conditions {
		key.WriteString(condition.key)
		key.WriteByte('=')
		if condition.truth == True {
			key.WriteByte('1')
		} else {
			key.WriteByte('0')
		}
		key.WriteByte('|')
	}
	return key.String()
}

// Valid reports whether every condition is valid, sorted, and unique.
func (state WorldState) Valid() bool {
	for index, condition := range state.conditions {
		if !condition.Valid() || index > 0 && state.conditions[index-1].key >= condition.key {
			return false
		}
	}
	return true
}

func (state WorldState) MarshalJSON() ([]byte, error) {
	if !state.Valid() {
		return nil, ErrInvalidWorldState
	}
	conditions := slices.Clone(state.conditions)
	if conditions == nil {
		conditions = []Condition{}
	}
	return json.Marshal(worldStateWire{Conditions: conditions})
}

func (state *WorldState) UnmarshalJSON(data []byte) error {
	if state == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidWorldState)
	}
	var wire worldStateWire
	if err := decodeStrict(data, &wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidWorldState, err)
	}
	value, err := NewWorldState(wire.Conditions...)
	if err != nil {
		return err
	}
	*state = value
	return nil
}

type worldStateWire struct {
	Conditions []Condition `json:"conditions"`
}
