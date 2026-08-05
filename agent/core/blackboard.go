package core

import (
	"errors"
	"reflect"
	"strings"
	"unicode"

	"github.com/Tangerg/lynx/agent/internal/nilvalue"
)

// ErrUnportableValue reports a write of a value whose Go state cannot survive
// the blackboard's portable form. Implementations wrap it so callers can tell a
// contract violation from a transport or storage failure.
var ErrUnportableValue = errors.New("core: value has no portable form")

// ErrUndeclaredState reports a value whose exact Go type the owning Agent never
// declared, so no snapshot of it could be restored. See [SnapshotCodec].
var ErrUndeclaredState = errors.New("core: type is not declared snapshot state")

// BlackboardReader is the read-only slice of [Blackboard] — passed to
// contexts that observe state but should not mutate it (e.g. condition
// evaluation, world-state determination, planner introspection).
type BlackboardReader interface {
	// Load returns an ownership-isolated copy of the value stored at key.
	// Mutating the returned value never changes blackboard state; callers commit
	// changes by storing a replacement.
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

	// Objects returns ownership-isolated values in insertion order.
	Objects() []any

	// Condition reads boolean state set via [BlackboardWriter.StoreCondition].
	Condition(key string) (bool, bool)
}

// BlackboardWriter is the mutation slice of [Blackboard].
type BlackboardWriter interface {
	// Store saves by name and appends to the ordered objects list, making the
	// value reachable both by name and by latest-of-type lookup. The blackboard
	// takes ownership of a serialized copy and returns [ErrUnportableValue] when
	// value's Go state would not survive that form.
	Store(key string, value any) error

	// Add appends without binding to a name. Used when an action wants
	// to record an artifact without claiming the canonical "it" slot.
	Add(value any) error

	// Bind stores under "it" AND derives a second key from the value's type
	// (e.g. UserInput → "user_input"). Dual-binding so YAML/prompt actions
	// can reference inputs by type-derived names without coupling to the
	// actual variable name.
	Bind(value any) error

	// StoreAll stores each binding — convenience for seeding.
	StoreAll(bindings Bindings) error

	// Hide marks an object as not-discoverable via Lookup, without removing
	// it from the historical record (Objects() still returns it).
	Hide(target any) error

	// StoreCondition records boolean state that is NOT derived from object
	// presence (e.g. "user_authenticated"). The planner consults these alongside
	// type bindings.
	StoreCondition(key string, value bool) error
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
// Ownership is taken of a value's portable form on write, and a read returns a
// copy rebuilt from it, so a stored type's Go state survives only where that
// form carries it. Runtime handles and other non-portable capabilities belong in
// [Dependencies], not in planner state. A write whose state would be
// dropped — unexported fields, an interface-typed field whose concrete type
// cannot be recovered — is rejected with [ErrUnportableValue] rather than
// silently truncated. A type that needs unexported state on the blackboard owns
// its own portable form by implementing both marshal and unmarshal for the
// implementation's encoding.
//
// Implementations MUST be safe for concurrent use by host code. Framework
// workflow fan-out does not share values or writes: every branch receives
// Clone state and its mutations are discarded before deterministic result join.
type Blackboard interface {
	Extension
	BlackboardReader
	BlackboardWriter

	// Clone returns an ownership-isolated copy of the current state.
	// Used by sub-agents, parallel action branches, and engine prototypes.
	Clone() (Blackboard, error)

	// ClearWorkingState removes bindings, objects, conditions, and hidden
	// markers. It reports a failure to commit that removal, so a caller that
	// clears before binding an output does not write the output over state it
	// only assumes is gone.
	ClearWorkingState() error
}

// Get is the typed form of [BlackboardReader.Lookup]. It is a top-level
// function because Go does not permit method type parameters.
func Get[T any](blackboard BlackboardReader, name string) (T, bool) {
	var zero T
	if nilvalue.Is(blackboard) {
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
// assignable to T, preserving insertion order.
func Objects[T any](blackboard BlackboardReader) []T {
	if nilvalue.Is(blackboard) {
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
	return snakeCase(name)
}

func snakeCase(value string) string {
	if value == "" {
		return ""
	}

	runes := []rune(value)
	words := make([]string, 0, len(runes))
	var word strings.Builder
	for i, current := range runes {
		word.WriteRune(current)
		if i == len(runes)-1 {
			break
		}

		next := runes[i+1]
		switch {
		case unicode.IsLetter(current) != unicode.IsLetter(next):
		case unicode.IsLower(current) && unicode.IsUpper(next):
		case unicode.IsUpper(current) && unicode.IsUpper(next) &&
			i+2 < len(runes) && unicode.IsLower(runes[i+2]):
		default:
			continue
		}
		words = append(words, word.String())
		word.Reset()
	}
	if word.Len() > 0 {
		words = append(words, word.String())
	}

	normalized := words[:0]
	for _, current := range words {
		if current == "" || current == "_" {
			continue
		}
		normalized = append(normalized, strings.ToLower(current))
	}
	return strings.Join(normalized, "_")
}
