package planning

import (
	"encoding/json"
	"fmt"
)

// Truth is the three-valued truth of one observed condition. Unknown is not a
// synonym for False: it means the current WorldState does not establish either
// known value. The zero value is Unknown.
type Truth uint8

const (
	// Unknown means the current observation does not establish the condition.
	Unknown Truth = iota
	// False means the current observation establishes that the condition is false.
	False
	// True means the current observation establishes that the condition is true.
	True
)

// Valid reports whether truth is one of the three supported values.
func (truth Truth) Valid() bool { return truth <= True }

func (truth Truth) known() bool { return truth == False || truth == True }

// String returns the stable lower-case truth name.
func (truth Truth) String() string {
	switch truth {
	case Unknown:
		return "unknown"
	case False:
		return "false"
	case True:
		return "true"
	default:
		return "invalid"
	}
}

// MarshalJSON encodes truth as its stable string name.
func (truth Truth) MarshalJSON() ([]byte, error) {
	if !truth.Valid() {
		return nil, fmt.Errorf("%w: truth value %d", ErrInvalidCondition, truth)
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
	switch encoded {
	case "unknown":
		*truth = Unknown
	case "false":
		*truth = False
	case "true":
		*truth = True
	default:
		return fmt.Errorf("%w: unsupported truth %q", ErrInvalidCondition, encoded)
	}
	return nil
}
