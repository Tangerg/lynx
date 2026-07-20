package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// TaggedValue is one durable blackboard value with the exact Go type needed
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

// EncodeBlackboard converts durable values into their strict tagged wire form.
// Every non-builtin concrete type must be declared by an action input/output
// on this Agent. Callers should store undeclared runtime objects through the
// Blackboard transient API instead.
func (a *Agent) EncodeBlackboard(bindings Bindings, objects []any) (map[string]TaggedValue, []TaggedValue, error) {
	if a == nil {
		return nil, nil, errors.New("agent.Agent.EncodeBlackboard: agent is nil")
	}
	table := a.durableTypes()
	var taggedNamed map[string]TaggedValue
	if bindings.Len() > 0 {
		taggedNamed = make(map[string]TaggedValue, bindings.Len())
		for key, value := range bindings.All() {
			tagged, err := tagSnapshotValue(value, table)
			if err != nil {
				return nil, nil, fmt.Errorf("blackboard[%q]: %w", key, err)
			}
			taggedNamed[key] = tagged
		}
	}
	taggedObjects := make([]TaggedValue, 0, len(objects))
	for i, value := range objects {
		tagged, err := tagSnapshotValue(value, table)
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

func tagSnapshotValue(value any, table map[string]reflect.Type) (TaggedValue, error) {
	if value == nil {
		return TaggedValue{Type: "any", Value: json.RawMessage("null")}, nil
	}
	typeName := typeFullName(reflect.TypeOf(value))
	if _, ok := table[typeName]; !ok {
		return TaggedValue{}, fmt.Errorf("type %q is not declared durable state", typeName)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return TaggedValue{}, fmt.Errorf("encode %q: %w", typeName, err)
	}
	return TaggedValue{Type: typeName, Value: data}, nil
}

// DecodeBlackboard reconstructs strict durable values. Unknown tags and decode
// failures are errors; restore never silently substitutes map[string]any.
func (a *Agent) DecodeBlackboard(named map[string]TaggedValue, objects []TaggedValue) (Bindings, []any, error) {
	if a == nil {
		return Bindings{}, nil, errors.New("agent.Agent.DecodeBlackboard: agent is nil")
	}
	table := a.durableTypes()
	var decodedNamed Bindings
	if len(named) > 0 {
		for key, tagged := range named {
			value, err := decodeSnapshotValue(tagged, table)
			if err != nil {
				return Bindings{}, nil, fmt.Errorf("blackboard[%q]: %w", key, err)
			}
			decodedNamed.Set(key, value)
		}
	}
	decodedObjects := make([]any, 0, len(objects))
	for i, tagged := range objects {
		value, err := decodeSnapshotValue(tagged, table)
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

func decodeSnapshotValue(tagged TaggedValue, table map[string]reflect.Type) (any, error) {
	if err := tagged.Validate(); err != nil {
		return nil, err
	}
	if tagged.Type == "any" {
		if !bytes.Equal(bytes.TrimSpace(tagged.Value), []byte("null")) {
			return nil, errors.New("type any is reserved for null")
		}
		return nil, nil
	}
	typeValue, ok := table[tagged.Type]
	if !ok || typeValue == nil {
		return nil, fmt.Errorf("unknown durable type %q", tagged.Type)
	}
	pointer := reflect.New(typeValue)
	if err := json.Unmarshal(tagged.Value, pointer.Interface()); err != nil {
		return nil, fmt.Errorf("decode %q: %w", tagged.Type, err)
	}
	return pointer.Elem().Interface(), nil
}

// durableTypes maps tagged state names to concrete action I/O and builtin
// values accepted by this agent's durable blackboard.
func (a *Agent) durableTypes() map[string]reflect.Type {
	table := map[string]reflect.Type{}
	for _, value := range []reflect.Type{
		reflect.TypeFor[bool](),
		reflect.TypeFor[string](),
		reflect.TypeFor[int](), reflect.TypeFor[int8](), reflect.TypeFor[int16](), reflect.TypeFor[int32](), reflect.TypeFor[int64](),
		reflect.TypeFor[uint](), reflect.TypeFor[uint8](), reflect.TypeFor[uint16](), reflect.TypeFor[uint32](), reflect.TypeFor[uint64](),
		reflect.TypeFor[float32](), reflect.TypeFor[float64](),
	} {
		table[typeFullName(value)] = value
	}
	for _, action := range a.Actions() {
		if action == nil {
			continue
		}
		metadata := action.Metadata()
		for _, bindings := range [][]Binding{metadata.Inputs, metadata.Outputs} {
			for _, binding := range bindings {
				if binding.goType != nil {
					table[binding.Type] = binding.goType
				}
			}
		}
	}
	return table
}
