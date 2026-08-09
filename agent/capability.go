package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// ErrInvalidCapability reports a malformed capability name or set.
var ErrInvalidCapability = errors.New("agent: invalid capability")

// Capability is one stable qualified authority name understood by a
// Deployment's dispatcher or another external boundary. The Framework only
// enforces possession and attenuation; it does not assign product meaning.
type Capability struct{ name string }

// ParseCapability validates a lowercase qualified capability name.
func ParseCapability(name string) (Capability, error) {
	if !validQualifiedName(name) {
		return Capability{}, ErrInvalidCapability
	}
	return Capability{name: name}, nil
}

// String returns the stable qualified name.
func (capability Capability) String() string { return capability.name }

// Valid reports whether the capability has a valid qualified name.
func (capability Capability) Valid() bool { return validQualifiedName(capability.name) }

// MarshalText returns the validated qualified capability name.
func (capability Capability) MarshalText() ([]byte, error) {
	if !capability.Valid() {
		return nil, ErrInvalidCapability
	}
	return []byte(capability.name), nil
}

// UnmarshalText replaces capability with a validated qualified name.
func (capability *Capability) UnmarshalText(text []byte) error {
	if capability == nil {
		return ErrInvalidCapability
	}
	value, err := ParseCapability(string(text))
	if err != nil {
		return err
	}
	*capability = value
	return nil
}

// CapabilitySet is an immutable, sorted set of authority names. Its zero value
// is the valid empty set.
type CapabilitySet struct{ values []Capability }

// NewCapabilitySet validates, deduplicates, and freezes capabilities.
func NewCapabilitySet(capabilities ...Capability) (CapabilitySet, error) {
	values := slices.Clone(capabilities)
	for _, capability := range values {
		if !capability.Valid() {
			return CapabilitySet{}, ErrInvalidCapability
		}
	}
	slices.SortFunc(values, func(left, right Capability) int {
		return bytes.Compare([]byte(left.name), []byte(right.name))
	})
	values = slices.Compact(values)
	return CapabilitySet{values: values}, nil
}

// Values returns an independently owned, sorted capability slice.
func (set CapabilitySet) Values() []Capability { return slices.Clone(set.values) }

// Contains reports whether capability belongs to the set.
func (set CapabilitySet) Contains(capability Capability) bool {
	if !capability.Valid() {
		return false
	}
	_, found := slices.BinarySearchFunc(set.values, capability, func(left, right Capability) int {
		return bytes.Compare([]byte(left.name), []byte(right.name))
	})
	return found
}

// Allows reports whether requested is a subset of set.
func (set CapabilitySet) Allows(requested CapabilitySet) bool {
	if !set.Valid() || !requested.Valid() {
		return false
	}
	for _, capability := range requested.values {
		if !set.Contains(capability) {
			return false
		}
	}
	return true
}

// Valid reports whether values are sorted, unique, and individually valid.
func (set CapabilitySet) Valid() bool {
	for index, capability := range set.values {
		if !capability.Valid() || index > 0 && set.values[index-1].name >= capability.name {
			return false
		}
	}
	return true
}

// MarshalJSON returns the canonical ordered capability set.
func (set CapabilitySet) MarshalJSON() ([]byte, error) {
	if !set.Valid() {
		return nil, ErrInvalidCapability
	}
	return json.Marshal(set.values)
}

// UnmarshalJSON replaces set with a validated canonical capability set.
func (set *CapabilitySet) UnmarshalJSON(data []byte) error {
	if set == nil {
		return ErrInvalidCapability
	}
	var values []Capability
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&values); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCapability, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCapability, err)
	}
	value, err := NewCapabilitySet(values...)
	if err != nil || len(value.values) != len(values) {
		return ErrInvalidCapability
	}
	*set = value
	return nil
}
