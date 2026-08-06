package planning

import (
	"encoding/json"
	"fmt"
)

// Condition is one immutable known truth requirement or prediction. Unknown is
// represented by absence from a WorldState and therefore cannot be stored in a
// Condition.
type Condition struct {
	key   string
	truth Truth
}

// NewCondition constructs a known condition. Key must be a stable lower-case
// qualified name and truth must be False or True.
func NewCondition(key string, truth Truth) (Condition, error) {
	condition := Condition{key: key, truth: truth}
	if !condition.Valid() {
		return Condition{}, fmt.Errorf("%w: key %q and truth %s", ErrInvalidCondition, key, truth)
	}
	return condition, nil
}

// Key returns the stable condition identity.
func (condition Condition) Key() string { return condition.key }

// Truth returns the known truth asserted by the condition.
func (condition Condition) Truth() Truth { return condition.truth }

// Valid reports whether the condition has a valid key and known truth.
func (condition Condition) Valid() bool {
	return validName(condition.key) && condition.truth.known()
}

// MarshalJSON returns the validated immutable condition.
func (condition Condition) MarshalJSON() ([]byte, error) {
	if !condition.Valid() {
		return nil, ErrInvalidCondition
	}
	return json.Marshal(conditionWire{Key: condition.key, Truth: condition.truth})
}

// UnmarshalJSON replaces condition with a strictly decoded known truth.
func (condition *Condition) UnmarshalJSON(data []byte) error {
	if condition == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidCondition)
	}
	var wire conditionWire
	if err := decodeStrict(data, &wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidCondition, err)
	}
	value, err := NewCondition(wire.Key, wire.Truth)
	if err != nil {
		return err
	}
	*condition = value
	return nil
}

type conditionWire struct {
	Key   string `json:"key"`
	Truth Truth  `json:"truth"`
}
