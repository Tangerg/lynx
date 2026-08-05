package agent2

import (
	"errors"
	"fmt"
)

const maxIdentityBytes = 256

var ErrInvalidIdentity = errors.New("agent: invalid identity")

type identity struct {
	value string
}

func parseIdentity(kind, value string) (identity, error) {
	if !validIdentity(value) {
		return identity{}, fmt.Errorf("%w: %s must contain 1 to %d URI-safe ASCII characters", ErrInvalidIdentity, kind, maxIdentityBytes)
	}
	return identity{value: value}, nil
}

func validIdentity(value string) bool {
	if len(value) == 0 || len(value) > maxIdentityBytes {
		return false
	}
	for index := range len(value) {
		character := value[index]
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' || character == '.' || character == ':' {
			continue
		}
		return false
	}
	return true
}

func (id identity) String() string { return id.value }

func (id identity) Valid() bool { return validIdentity(id.value) }

func (id identity) MarshalText() ([]byte, error) {
	if !id.Valid() {
		return nil, ErrInvalidIdentity
	}
	return []byte(id.value), nil
}

func (id *identity) UnmarshalText(text []byte) error {
	if id == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidIdentity)
	}
	value, err := parseIdentity("value", string(text))
	if err != nil {
		return err
	}
	*id = value
	return nil
}

// ProcessID is the stable identity of one Engine-owned Process.
type ProcessID struct{ identity }

// ParseProcessID validates an externally encoded Process identity.
func ParseProcessID(value string) (ProcessID, error) {
	id, err := parseIdentity("process ID", value)
	return ProcessID{id}, err
}

// SignalID is the stable identity used to deduplicate one Signal delivery.
type SignalID struct{ identity }

// ParseSignalID validates an externally supplied Signal delivery identity.
// Parsing does not accept or deliver the Signal.
func ParseSignalID(value string) (SignalID, error) {
	id, err := parseIdentity("signal ID", value)
	return SignalID{id}, err
}

// WaitID identifies one Engine-created external wait target. Parsing a WaitID
// does not create a wait; the Engine rejects identities it did not mint.
type WaitID struct{ identity }

// ParseWaitID validates the wire representation of a Wait identity.
func ParseWaitID(value string) (WaitID, error) {
	id, err := parseIdentity("wait ID", value)
	return WaitID{id}, err
}

// EffectID identifies one Effect at a stable Process, Step, and batch index.
type EffectID struct{ identity }

// ParseEffectID validates an externally encoded Effect identity.
func ParseEffectID(value string) (EffectID, error) {
	id, err := parseIdentity("effect ID", value)
	return EffectID{id}, err
}
