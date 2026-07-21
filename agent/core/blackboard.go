package core

import (
	"fmt"
	"reflect"
	"strings"

	pkgstrings "github.com/Tangerg/lynx/pkg/strings"
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

	// Load returns whatever is stored at key (by name only).
	Load(key string) (any, bool)

	// Lookup returns the value bound to (variable, typeName). When
	// variable is DefaultBindingName ("it"), implementations search the
	// objects list from newest to oldest for a type match. When variable
	// is LastResultBindingName ("last_result"), it returns the most-recent
	// object regardless of type.
	Lookup(variable, typeName string) (any, bool)

	// HasValue is the planner's cheap precondition probe; equivalent to
	// Lookup returning ok.
	HasValue(variable, typeName string) bool

	// Objects returns a snapshot in insertion order.
	Objects() []any

	// Condition reads boolean state set via [BlackboardWriter.StoreCondition].
	Condition(key string) (bool, bool)

	// Inspect is for human consumption — verbose=true dumps everything,
	// false produces a compact summary.
	Inspect(verbose bool) string
}

// BlackboardWriter is the mutation slice of [Blackboard].
type BlackboardWriter interface {
	// Store saves by name and appends to the ordered objects list, making the
	// value reachable both by name and by latest-of-type lookup.
	Store(key string, value any)

	// StoreTransient stores a runtime-only named value. It participates in live
	// lookups but is excluded from durable snapshots and restored processes.
	StoreTransient(key string, value any)

	// Add appends without binding to a name. Used when an action wants
	// to record an artifact without claiming the canonical "it" slot.
	Add(value any)

	// AddTransient appends a runtime-only artifact.
	AddTransient(value any)

	// Bind stores under "it" AND derives a second key from the value's type
	// (e.g. UserInput → "user_input"). Dual-binding so YAML/prompt actions
	// can reference inputs by type-derived names without coupling to the
	// actual variable name.
	Bind(value any)

	// BindTransient applies Bind's lookup semantics without making the value
	// durable. Use it for handles, clients, channels, and other runtime state.
	BindTransient(value any)

	// StoreAll stores each binding — convenience for seeding.
	StoreAll(bindings Bindings)

	// StoreProtected marks a key so Clone() preserves it on child blackboards
	// even when the rest of the state is forked. Useful for session tokens
	// and other ambient context.
	StoreProtected(key string, value any)

	// Hide marks an object as not-discoverable via Lookup, without removing
	// it from the historical record (Objects() still returns it).
	Hide(target any)

	// StoreCondition records boolean state that is NOT derived from object
	// presence (e.g. "user_authenticated"). The planner consults these
	// alongside type bindings.
	StoreCondition(key string, value bool)
}

// Blackboard is the shared, typed memory all actions read from and write
// to. It uses named keys for explicit lookups, an ordered tail for
// "give me the latest thing of type T" semantics, plus a separate set of
// explicit conditions.
//
// A Blackboard is also an engine [Extension]: register one and the
// runtime uses [Blackboard.Clone] to produce a fresh, isolated
// instance for every new process. The registered value itself is the
// prototype — it is never read from or written to directly. Blackboard is
// engine-scoped only; [ProcessOptions.Blackboard] is the explicit per-process
// override.
//
// Implementations MUST be safe for concurrent use by host code. Framework
// workflow fan-out does not share writes: every branch receives Clone() state
// and its mutations are discarded before deterministic result join.
type Blackboard interface {
	Extension
	BlackboardReader
	BlackboardWriter

	// Clone returns an independent copy of the current state.
	// Mutations on the child do not propagate back. Used by sub-agents
	// and (since the prototype pattern replaced BlackboardFactory) to
	// produce the per-process Blackboard at process start.
	Clone() Blackboard

	// ClearWorkingState removes ordinary bindings, objects, conditions, and
	// hidden markers while retaining values stored with StoreProtected.
	ClearWorkingState()
}

// Get is the typed form of [BlackboardReader.Lookup]. It is a top-level
// function because Go does not permit method type parameters.
func Get[T any](blackboard BlackboardReader, name string) (T, bool) {
	var zero T
	if blackboard == nil {
		return zero, false
	}

	value, ok := blackboard.Lookup(name, TypeName[T]())
	if !ok {
		return zero, false
	}

	typed, ok := value.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

// Objects filters the blackboard's object list to entries
// assignable to T, preserving insertion order. Useful when an action
// collects "all citations" or "all decisions made so far".
func Objects[T any](blackboard BlackboardReader) []T {
	if blackboard == nil {
		return nil
	}

	var out []T
	for _, object := range blackboard.Objects() {
		if typed, ok := object.(T); ok {
			out = append(out, typed)
		}
	}
	return out
}

// Last returns the most-recent object of type T or the zero value if absent.
func Last[T any](blackboard BlackboardReader) (T, bool) {
	matches := Objects[T](blackboard)
	if len(matches) == 0 {
		var zero T
		return zero, false
	}
	return matches[len(matches)-1], true
}

// TypeKey converts a Go reflect type into the variable name used
// by Bind() for dual-binding. UserInput → "user_input",
// *Quote → "quote", HTTPResponse → "http_response". Empty names
// (anonymous types) yield the empty string so callers can skip.
func TypeKey(value any) string {
	if value == nil {
		return ""
	}

	typ := reflect.TypeOf(value)
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil {
		return ""
	}

	name := typ.Name()
	if name == "" {
		return ""
	}
	return string(pkgstrings.AsCamelCase(name).ToSnakeCase())
}

// FormatBlackboard helps custom Blackboard implementations format consistent
// debug strings; the in-memory blackboard's Inspect delegates to it.
func FormatBlackboard(blackboard BlackboardReader, verbose bool) string {
	if blackboard == nil {
		return "<nil blackboard>"
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Blackboard{id=%s objects=%d}", blackboard.ID(), len(blackboard.Objects()))
	if !verbose {
		return out.String()
	}

	for i, object := range blackboard.Objects() {
		fmt.Fprintf(&out, "\n  [%d] %T = %+v", i, object, object)
	}
	return out.String()
}
