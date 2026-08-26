package planning

import (
	"encoding/json"
	"fmt"
)

// Truth is the three-valued truth of one observed condition. Unknown is not a
// synonym for False: it means the current WorldState does not establish either
// known value. The zero value is invalid; callers must choose explicitly.
type Truth string

const (
	// Unknown means the current observation does not establish the condition.
	Unknown Truth = "unknown"
	// False means the current observation establishes that the condition is false.
	False Truth = "false"
	// True means the current observation establishes that the condition is true.
	True Truth = "true"
)

// Valid reports whether truth is one of the three supported values.
func (truth Truth) Valid() bool { return truth == Unknown || truth == False || truth == True }

func (truth Truth) known() bool { return truth == False || truth == True }

// String returns the stable lower-case truth name.
func (truth Truth) String() string {
	if !truth.Valid() {
		return "invalid"
	}
	return string(truth)
}

// MarshalJSON encodes truth as its stable string name.
func (truth Truth) MarshalJSON() ([]byte, error) {
	if !truth.Valid() {
		return nil, fmt.Errorf("%w: truth value %q", ErrInvalidCondition, truth)
	}
	return json.Marshal(truth.String())
}

// UnmarshalJSON decodes one strict truth string.
func (truth *Truth) UnmarshalJSON(data []byte) error {
	if truth == nil {
		return fmt.Errorf("%w: nil Truth receiver", ErrInvalidCondition)
	}
	var encoded string
	if err := decodeStrict(data, &encoded); err != nil {
		return fmt.Errorf("%w: decode Truth: %w", ErrInvalidCondition, err)
	}
	decoded := Truth(encoded)
	if !decoded.Valid() {
		return fmt.Errorf("%w: unsupported truth %q", ErrInvalidCondition, encoded)
	}
	*truth = decoded
	return nil
}
