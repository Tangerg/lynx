package core

import (
	"encoding/json"
	"reflect"
)

// TaggedValue is one blackboard value persisted with its concrete Go type
// name. A [ProcessSnapshot] stores blackboard state as TaggedValues so a
// round-trip through JSON (the persistence boundary) can reconstruct the
// original Go type on restore instead of decoding every value into the
// generic map[string]any JSON yields. Without this a restored typed-action
// input fails its type assertion and the resumed action errors.
//
// Value holds the value's own JSON; Type is its [TypeFullName] (empty for a
// nil value). Decoding pairs Type against a type table derived from the
// restoring agent's action I/O bindings (see [Agent] snapshotTypeTable).
type TaggedValue struct {
	Type  string          `json:"t,omitempty"`
	Value json.RawMessage `json:"v,omitempty"`
}

// tagValue captures v with its concrete type name. A marshal failure yields
// an empty tag (decodes to nil) rather than aborting the whole snapshot —
// dropping one unserializable value beats losing the entire process state.
func tagValue(v any) TaggedValue {
	if v == nil {
		return TaggedValue{}
	}
	data, err := json.Marshal(v)
	if err != nil {
		return TaggedValue{}
	}
	return TaggedValue{Type: typeFullName(reflect.TypeOf(v)), Value: data}
}

// TagBlackboard wraps the named bindings + ordered objects a
// [BlackboardSnapshotter] produced into type-tagged form for persistence.
// Conditions need no tagging — bools round-trip through JSON losslessly.
func TagBlackboard(named map[string]any, objects []any) (map[string]TaggedValue, []TaggedValue) {
	var taggedNamed map[string]TaggedValue
	if len(named) > 0 {
		taggedNamed = make(map[string]TaggedValue, len(named))
		for k, v := range named {
			taggedNamed[k] = tagValue(v)
		}
	}
	var taggedObjects []TaggedValue
	if len(objects) > 0 {
		taggedObjects = make([]TaggedValue, len(objects))
		for i, v := range objects {
			taggedObjects[i] = tagValue(v)
		}
	}
	return taggedNamed, taggedObjects
}

// decode reconstructs the value's concrete Go type using table (type name →
// reflect.Type). A type absent from the table — a primitive, or one no
// deployed action declares — decodes into a generic any (the same
// lossy-but-usable form the pre-tagging snapshot produced); an empty value
// decodes to nil.
func (tv TaggedValue) decode(table map[string]reflect.Type) any {
	if len(tv.Value) == 0 {
		return nil
	}
	if rt, ok := table[tv.Type]; ok && rt != nil {
		ptr := reflect.New(rt)
		if err := json.Unmarshal(tv.Value, ptr.Interface()); err == nil {
			return ptr.Elem().Interface()
		}
	}
	var v any
	_ = json.Unmarshal(tv.Value, &v)
	return v
}

// UntagBlackboard reverses [TagBlackboard], reconstructing concrete types via
// the type table derived from agentDef's declared action I/O bindings.
func UntagBlackboard(named map[string]TaggedValue, objects []TaggedValue, agentDef *Agent) (map[string]any, []any) {
	table := agentDef.snapshotTypeTable()
	var outNamed map[string]any
	if len(named) > 0 {
		outNamed = make(map[string]any, len(named))
		for k, tv := range named {
			outNamed[k] = tv.decode(table)
		}
	}
	var outObjects []any
	if len(objects) > 0 {
		outObjects = make([]any, len(objects))
		for i, tv := range objects {
			outObjects[i] = tv.decode(table)
		}
	}
	return outNamed, outObjects
}

// snapshotTypeTable maps type name → concrete reflect.Type from every action's
// declared input/output bindings, so a snapshot's tagged values decode back to
// their original Go types on restore. Only bindings built via [NewIOBinding]
// (which captures the type) contribute; the rest fall back to generic decode.
func (a *Agent) snapshotTypeTable() map[string]reflect.Type {
	table := map[string]reflect.Type{}
	if a == nil {
		return table
	}
	for _, act := range a.Actions {
		if act == nil {
			continue
		}
		meta := act.Metadata()
		for _, b := range meta.Inputs {
			if b.goType != nil {
				table[b.Type] = b.goType
			}
		}
		for _, b := range meta.Outputs {
			if b.goType != nil {
				table[b.Type] = b.goType
			}
		}
	}
	return table
}
