package core

import (
	"fmt"
	"reflect"
	"strings"
)

// BlackboardReader is the read-only slice of [Blackboard] — passed to
// contexts that observe state but should not mutate it (e.g. condition
// evaluation, world-state determination, planner introspection).
//
// Splitting reader from writer is structural, not nominal: a custom
// Blackboard automatically satisfies both halves by satisfying the full
// interface.
type BlackboardReader interface {
	ID() string

	// Get returns whatever is stored at key (by name only).
	Get(key string) (any, bool)

	// GetValue returns the value bound to (variable, typeName). When
	// variable is DefaultBindingName ("it"), implementations search the
	// objects list from newest to oldest for a type match.
	// LastResultBindingName returns the most-recent object regardless
	// of type.
	GetValue(variable, typeName string) (any, bool)

	// HasValue is the planner's cheap precondition probe; equivalent to
	// GetValue returning ok.
	HasValue(variable, typeName string) bool

	// Objects returns a snapshot in insertion order.
	Objects() []any

	// GetCondition reads boolean state set via [BlackboardWriter.SetCondition].
	GetCondition(key string) (bool, bool)

	// InfoString is for human consumption — verbose=true dumps everything,
	// false produces a compact summary.
	InfoString(verbose bool) string
}

// BlackboardWriter is the mutation slice of [Blackboard].
type BlackboardWriter interface {
	// Set stores by name AND appends to the ordered objects list — so a
	// single Set both makes the value reachable by name and by latest-of-type.
	Set(key string, value any)

	// AddObject appends without binding to a name. Used when an action wants
	// to record an artifact without claiming the canonical "it" slot.
	AddObject(value any)

	// Bind stores under "it" AND derives a second key from the value's type
	// (e.g. UserInput → "userInput"). Implements embabel 0.4's autonomy
	// dual-binding so YAML/prompt actions can reference inputs by
	// type-derived names without coupling to the actual variable name.
	Bind(value any)

	// BindAll runs Set for each entry — convenience for seeding.
	BindAll(m map[string]any)

	// BindProtected marks a key so Spawn() preserves it on child blackboards
	// even when the rest of the state is forked. Useful for session tokens
	// and other ambient context.
	BindProtected(key string, value any)

	// Hide marks an object as not-discoverable via GetValue, without removing
	// it from the historical record (Objects() still returns it).
	Hide(target any)

	// SetCondition records boolean state that is NOT derived from object
	// presence (e.g. "user_authenticated"). The planner consults these
	// alongside type bindings.
	SetCondition(key string, value bool)
}

// Blackboard is the shared, typed memory all actions read from and write
// to. embabel uses a flat map plus an ordered "objects" list — Lynx
// mirrors that: named keys for explicit lookups, an ordered tail for
// "give me the latest thing of type T" semantics, plus a separate set of
// explicit conditions.
type Blackboard interface {
	BlackboardReader
	BlackboardWriter

	// Spawn creates a child that starts with a copy of the parent's state.
	// Mutations on the child do not propagate back. Used by sub-agents.
	Spawn() Blackboard

	Clear()
}

// Get is a typed shortcut for [BlackboardReader.GetValue]. It's a
// top-level function because Go doesn't permit method-level type
// parameters; callers write core.Get[Foo](bb, "it") instead of
// bb.Get<Foo>("it").
func Get[T any](bb BlackboardReader, name string) (T, bool) {
	var zero T
	if bb == nil {
		return zero, false
	}

	value, ok := bb.GetValue(name, TypeFullNameOf[T]())
	if !ok {
		return zero, false
	}

	typed, ok := value.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

// ObjectsOfType filters the blackboard's object list to entries
// assignable to T, preserving insertion order. Useful when an action
// collects "all citations" or "all decisions made so far".
func ObjectsOfType[T any](bb BlackboardReader) []T {
	if bb == nil {
		return nil
	}

	var out []T
	for _, obj := range bb.Objects() {
		if typed, ok := obj.(T); ok {
			out = append(out, typed)
		}
	}
	return out
}

// Last returns the most-recent object of type T or the zero value if absent.
func Last[T any](bb BlackboardReader) (T, bool) {
	matches := ObjectsOfType[T](bb)
	if len(matches) == 0 {
		var zero T
		return zero, false
	}
	return matches[len(matches)-1], true
}

// Count reports how many T-typed objects are on the blackboard.
func Count[T any](bb BlackboardReader) int { return len(ObjectsOfType[T](bb)) }

// DerivedTypeKey converts a Go reflect type into the variable name used by
// Bind() for dual-binding. UserInput → "userInput", *Quote → "quote".
// Empty names (anonymous types) yield the empty string so callers can skip.
func DerivedTypeKey(v any) string {
	if v == nil {
		return ""
	}

	rt := reflect.TypeOf(v)
	for rt != nil && rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if rt == nil {
		return ""
	}

	name := rt.Name()
	if name == "" {
		return ""
	}
	return strings.ToLower(name[:1]) + name[1:]
}

// InspectInfoString helps custom Blackboard implementations format consistent
// debug strings. Exposed as a helper because the runtime uses it for tests.
func InspectInfoString(bb BlackboardReader, verbose bool) string {
	if bb == nil {
		return "<nil blackboard>"
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Blackboard{id=%s objects=%d}", bb.ID(), len(bb.Objects()))
	if !verbose {
		return out.String()
	}

	for i, obj := range bb.Objects() {
		fmt.Fprintf(&out, "\n  [%d] %T = %+v", i, obj, obj)
	}
	return out.String()
}
