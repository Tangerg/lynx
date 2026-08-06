package agent2

import (
	"errors"
	"fmt"
)

const maxIdentityBytes = 256

// ErrInvalidIdentity reports a malformed Framework identity value.
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

// String returns the stable Process identity.
func (id ProcessID) String() string { return id.identity.String() }

// Valid reports whether id contains a valid Process identity.
func (id ProcessID) Valid() bool { return id.identity.Valid() }

// MarshalText returns the validated Process identity.
func (id ProcessID) MarshalText() ([]byte, error) { return id.identity.MarshalText() }

// UnmarshalText replaces id with a validated Process identity.
func (id *ProcessID) UnmarshalText(text []byte) error {
	if id == nil {
		return fmt.Errorf("%w: nil ProcessID receiver", ErrInvalidIdentity)
	}
	value, err := ParseProcessID(string(text))
	if err != nil {
		return err
	}
	*id = value
	return nil
}

// SignalID is the stable identity used to deduplicate one Signal delivery.
type SignalID struct{ identity }

// ParseSignalID validates an externally supplied Signal delivery identity.
// Parsing does not accept or deliver the Signal.
func ParseSignalID(value string) (SignalID, error) {
	id, err := parseIdentity("signal ID", value)
	return SignalID{id}, err
}

// String returns the stable Signal delivery identity.
func (id SignalID) String() string { return id.identity.String() }

// Valid reports whether id contains a valid Signal delivery identity.
func (id SignalID) Valid() bool { return id.identity.Valid() }

// MarshalText returns the validated Signal delivery identity.
func (id SignalID) MarshalText() ([]byte, error) { return id.identity.MarshalText() }

// UnmarshalText replaces id with a validated Signal delivery identity.
func (id *SignalID) UnmarshalText(text []byte) error {
	if id == nil {
		return fmt.Errorf("%w: nil SignalID receiver", ErrInvalidIdentity)
	}
	value, err := ParseSignalID(string(text))
	if err != nil {
		return err
	}
	*id = value
	return nil
}

// WaitID identifies one Engine-created external wait target. Parsing a WaitID
// does not create a wait; the Engine rejects identities it did not mint.
type WaitID struct{ identity }

// ParseWaitID validates the wire representation of a Wait identity.
func ParseWaitID(value string) (WaitID, error) {
	id, err := parseIdentity("wait ID", value)
	return WaitID{id}, err
}

// String returns the Engine-created wait identity.
func (id WaitID) String() string { return id.identity.String() }

// Valid reports whether id contains a valid wait identity.
func (id WaitID) Valid() bool { return id.identity.Valid() }

// MarshalText returns the validated wait identity.
func (id WaitID) MarshalText() ([]byte, error) { return id.identity.MarshalText() }

// UnmarshalText replaces id with a validated wait identity.
func (id *WaitID) UnmarshalText(text []byte) error {
	if id == nil {
		return fmt.Errorf("%w: nil WaitID receiver", ErrInvalidIdentity)
	}
	value, err := ParseWaitID(string(text))
	if err != nil {
		return err
	}
	*id = value
	return nil
}

// EffectID identifies one Effect at a stable Process, Step, and batch index.
type EffectID struct{ identity }

// ParseEffectID validates an externally encoded Effect identity.
func ParseEffectID(value string) (EffectID, error) {
	id, err := parseIdentity("effect ID", value)
	return EffectID{id}, err
}

// String returns the stable Effect identity.
func (id EffectID) String() string { return id.identity.String() }

// Valid reports whether id contains a valid Effect identity.
func (id EffectID) Valid() bool { return id.identity.Valid() }

// MarshalText returns the validated Effect identity.
func (id EffectID) MarshalText() ([]byte, error) { return id.identity.MarshalText() }

// UnmarshalText replaces id with a validated Effect identity.
func (id *EffectID) UnmarshalText(text []byte) error {
	if id == nil {
		return fmt.Errorf("%w: nil EffectID receiver", ErrInvalidIdentity)
	}
	value, err := ParseEffectID(string(text))
	if err != nil {
		return err
	}
	*id = value
	return nil
}

// WaitKey is an Execution-owned logical key used to associate a requested wait
// with the WaitID later minted by the Engine.
type WaitKey struct{ identity }

// ParseWaitKey validates an Execution-owned logical wait key.
func ParseWaitKey(value string) (WaitKey, error) {
	id, err := parseIdentity("wait key", value)
	return WaitKey{id}, err
}

// String returns the Execution-owned logical wait key.
func (id WaitKey) String() string { return id.identity.String() }

// Valid reports whether id contains a valid logical wait key.
func (id WaitKey) Valid() bool { return id.identity.Valid() }

// MarshalText returns the validated logical wait key.
func (id WaitKey) MarshalText() ([]byte, error) { return id.identity.MarshalText() }

// UnmarshalText replaces id with a validated logical wait key.
func (id *WaitKey) UnmarshalText(text []byte) error {
	if id == nil {
		return fmt.Errorf("%w: nil WaitKey receiver", ErrInvalidIdentity)
	}
	value, err := ParseWaitKey(string(text))
	if err != nil {
		return err
	}
	*id = value
	return nil
}

// ChildKey is an Execution-owned stable identity for one logical child start.
// The Engine combines it with the parent Process identity and prepared Effect
// identity to make retries and restoration idempotent.
type ChildKey struct{ identity }

// ParseChildKey validates an Execution-owned logical child identity.
func ParseChildKey(value string) (ChildKey, error) {
	id, err := parseIdentity("child key", value)
	return ChildKey{id}, err
}

// String returns the Execution-owned logical child key.
func (id ChildKey) String() string { return id.identity.String() }

// Valid reports whether id contains a valid logical child key.
func (id ChildKey) Valid() bool { return id.identity.Valid() }

// MarshalText returns the validated logical child key.
func (id ChildKey) MarshalText() ([]byte, error) { return id.identity.MarshalText() }

// UnmarshalText replaces id with a validated logical child key.
func (id *ChildKey) UnmarshalText(text []byte) error {
	if id == nil {
		return fmt.Errorf("%w: nil ChildKey receiver", ErrInvalidIdentity)
	}
	value, err := ParseChildKey(string(text))
	if err != nil {
		return err
	}
	*id = value
	return nil
}
