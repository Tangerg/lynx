package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/Tangerg/lynx/agent/internal/nilvalue"
)

const nullJSON = "null"

// TaggedValue is one snapshot blackboard value with the exact Go type needed
// to reconstruct it after a JSON round trip.
type TaggedValue struct {
	Type  string          `json:"t"`
	Value json.RawMessage `json:"v"`
}

// Validate checks the tagged wire value without decoding its concrete type.
func (tv TaggedValue) Validate() error {
	if tv.Type == "" {
		return errors.New("tagged value type is empty")
	}
	if len(tv.Value) == 0 || !json.Valid(tv.Value) {
		return errors.New("tagged value JSON is invalid")
	}
	return nil
}

// EncodeBlackboard converts snapshot values into their strict tagged wire form.
// Every non-builtin concrete type must be declared by an action input/output or
// by [AgentConfig.SnapshotBindings] on this Agent: the declaration is what lets
// [Agent.DecodeBlackboard] recover the exact Go type from a tag, so an
// undeclared type is a value this Agent could encode but never restore.
//
// Undeclared state has no home on the blackboard. Runtime handles and other
// values that must not be carried across a restore belong in [Dependencies] or
// on the request context, not in planner state.
func (a *Agent) EncodeBlackboard(bindings Bindings, objects []any) (map[string]TaggedValue, []TaggedValue, error) {
	if a == nil {
		return nil, nil, errors.New("agent.Agent.EncodeBlackboard: agent is nil")
	}
	codec := a.SnapshotCodec()
	var taggedNamed map[string]TaggedValue
	if bindings.Len() > 0 {
		taggedNamed = make(map[string]TaggedValue, bindings.Len())
		for key, value := range bindings.All() {
			tagged, err := codec.Encode(value)
			if err != nil {
				return nil, nil, fmt.Errorf("blackboard[%q]: %w", key, err)
			}
			taggedNamed[key] = tagged
		}
	}
	taggedObjects := make([]TaggedValue, 0, len(objects))
	for i, value := range objects {
		tagged, err := codec.Encode(value)
		if err != nil {
			return nil, nil, fmt.Errorf("objects[%d]: %w", i, err)
		}
		taggedObjects = append(taggedObjects, tagged)
	}
	if len(taggedObjects) == 0 {
		taggedObjects = nil
	}
	return taggedNamed, taggedObjects, nil
}

// SnapshotCodec is the single authority on which values can cross a process
// snapshot. A value survives only if its exact Go type is declared by this
// Agent — an action input or output, or [AgentConfig.SnapshotBindings] — because
// the declaration is the only thing that turns a tag back into a
// [reflect.Type]. An undeclared type is one the Agent could encode and never
// restore.
//
// Obtain one with [Agent.SnapshotCodec]. The zero value declares nothing and
// rejects every non-nil value, which is the safe reading of "no agent".
type SnapshotCodec struct {
	types map[string]reflect.Type
}

// SnapshotCodec returns the codec for this Agent's declared state. The result
// is a snapshot of the declarations; later mutation of the Agent cannot widen
// what an in-flight process is allowed to carry.
func (a *Agent) SnapshotCodec() SnapshotCodec {
	if a == nil {
		return SnapshotCodec{}
	}
	return SnapshotCodec{types: a.snapshotTypes()}
}

// ValidateDeclaredType reports why value's type cannot cross a snapshot, or nil when it
// can. It answers from the declaration table alone, so it costs nothing to ask
// and cannot catch a declared type whose value fails to encode —
// [SnapshotCodec.Encode] is the complete check, and what the runtime gates
// writes on.
func (c SnapshotCodec) ValidateDeclaredType(value any) error {
	if value == nil {
		return nil
	}
	typeName := snapshotTypeName(reflect.TypeOf(value))
	if _, ok := c.types[typeName]; !ok {
		return fmt.Errorf("%w: %q", ErrUndeclaredSnapshotType, typeName)
	}
	return nil
}

// Encode tags value with the exact type needed to rebuild it.
func (c SnapshotCodec) Encode(value any) (TaggedValue, error) {
	if value == nil {
		return TaggedValue{Type: anyTypeName, Value: json.RawMessage(nullJSON)}, nil
	}
	if err := c.ValidateDeclaredType(value); err != nil {
		return TaggedValue{}, err
	}
	typeName := snapshotTypeName(reflect.TypeOf(value))
	data, err := json.Marshal(value)
	if err != nil {
		return TaggedValue{}, fmt.Errorf("encode %q: %w", typeName, err)
	}
	return TaggedValue{Type: typeName, Value: data}, nil
}

// Decode rebuilds the exact Go value behind a tag. Unknown tags and decode
// failures are errors; restore never silently substitutes a generic JSON value.
func (c SnapshotCodec) Decode(tagged TaggedValue) (any, error) {
	if err := tagged.Validate(); err != nil {
		return nil, err
	}
	if tagged.Type == anyTypeName {
		if !bytes.Equal(bytes.TrimSpace(tagged.Value), []byte(nullJSON)) {
			return nil, errors.New("type any is reserved for null")
		}
		return nil, nil
	}
	typeValue, ok := c.types[tagged.Type]
	if !ok || typeValue == nil {
		return nil, fmt.Errorf("%w: %q", ErrUndeclaredSnapshotType, tagged.Type)
	}
	pointer := reflect.New(typeValue)
	if err := json.Unmarshal(tagged.Value, pointer.Interface()); err != nil {
		return nil, fmt.Errorf("decode %q: %w", tagged.Type, err)
	}
	return pointer.Elem().Interface(), nil
}

// DecodeBlackboard reconstructs strict snapshot values. Unknown tags and decode
// failures are errors; restore never silently substitutes generic JSON objects.
func (a *Agent) DecodeBlackboard(named map[string]TaggedValue, objects []TaggedValue) (Bindings, []any, error) {
	if a == nil {
		return Bindings{}, nil, errors.New("agent.Agent.DecodeBlackboard: agent is nil")
	}
	codec := a.SnapshotCodec()
	var decodedNamed Bindings
	if len(named) > 0 {
		for key, tagged := range named {
			value, err := codec.Decode(tagged)
			if err != nil {
				return Bindings{}, nil, fmt.Errorf("blackboard[%q]: %w", key, err)
			}
			decodedNamed.Set(key, value)
		}
	}
	decodedObjects := make([]any, 0, len(objects))
	for i, tagged := range objects {
		value, err := codec.Decode(tagged)
		if err != nil {
			return Bindings{}, nil, fmt.Errorf("objects[%d]: %w", i, err)
		}
		decodedObjects = append(decodedObjects, value)
	}
	if len(decodedObjects) == 0 {
		decodedObjects = nil
	}
	return decodedNamed, decodedObjects, nil
}

// snapshotTypes maps tagged state names to concrete action I/O and builtin
// values accepted by this agent's process snapshots.
func (a *Agent) snapshotTypes() map[string]reflect.Type {
	table := map[string]reflect.Type{}
	for _, value := range []reflect.Type{
		reflect.TypeFor[bool](),
		reflect.TypeFor[string](),
		reflect.TypeFor[int](), reflect.TypeFor[int8](), reflect.TypeFor[int16](), reflect.TypeFor[int32](), reflect.TypeFor[int64](),
		reflect.TypeFor[uint](), reflect.TypeFor[uint8](), reflect.TypeFor[uint16](), reflect.TypeFor[uint32](), reflect.TypeFor[uint64](),
		reflect.TypeFor[float32](), reflect.TypeFor[float64](),
	} {
		table[snapshotTypeName(value)] = value
	}
	register := func(bindings []Binding) {
		for _, binding := range bindings {
			if binding.goType != nil {
				table[snapshotTypeName(binding.goType)] = binding.goType
			}
		}
	}
	register(a.SnapshotBindings())
	for _, action := range a.Actions() {
		if nilvalue.Is(action) {
			continue
		}
		metadata, err := inspectActionMetadata(action)
		if err != nil {
			continue
		}
		register(metadata.Inputs)
		register(metadata.Outputs)
	}
	return table
}

// snapshotTypeName is the snapshot identity of an exact Go type. Planner
// bindings intentionally normalize pointers, while snapshots must distinguish
// T from *T to restore values accepted by typed actions.
func snapshotTypeName(typ reflect.Type) string {
	if typ == nil {
		return anyTypeName
	}
	switch typ.Kind() {
	case reflect.Pointer:
		return "*" + snapshotTypeName(typ.Elem())
	case reflect.Slice:
		return "[]" + snapshotTypeName(typ.Elem())
	case reflect.Array:
		return "[" + strconv.Itoa(typ.Len()) + "]" + snapshotTypeName(typ.Elem())
	case reflect.Map:
		return "map[" + snapshotTypeName(typ.Key()) + "]" + snapshotTypeName(typ.Elem())
	}
	if typ.PkgPath() != "" && typ.Name() != "" {
		return typ.PkgPath() + "." + typ.Name()
	}
	return typ.String()
}
