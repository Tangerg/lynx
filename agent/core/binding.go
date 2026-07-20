package core

import (
	"iter"
	"maps"
	"reflect"
	"strings"
)

const anyTypeName = "any"

const (
	// DefaultBindingName is the implicit variable name when callers
	// don't supply one; the planner falls back to "the most recent
	// value of this type" when it sees this name.
	DefaultBindingName = "it"

	// LastResultBindingName addresses the single most-recently-added
	// object regardless of type.
	LastResultBindingName = "last_result"
)

// Bindings is an ownership-aware set of initial blackboard values. Its zero
// value is an empty set ready for use. Clone copies the container so runtime
// processes never retain a caller-owned map; stored values themselves are not
// deep-copied because blackboard values may be arbitrary Go objects.
type Bindings struct {
	values map[string]any
}

// Input returns bindings containing value under [DefaultBindingName].
func Input(value any) Bindings {
	var bindings Bindings
	bindings.Set(DefaultBindingName, value)
	return bindings
}

// Set associates value with name.
func (b *Bindings) Set(name string, value any) {
	if b.values == nil {
		b.values = make(map[string]any)
	}
	b.values[name] = value
}

// Get returns the value associated with name.
func (b Bindings) Get(name string) (any, bool) {
	value, ok := b.values[name]
	return value, ok
}

// Delete removes name from b.
func (b *Bindings) Delete(name string) { delete(b.values, name) }

// Len returns the number of bindings.
func (b Bindings) Len() int { return len(b.values) }

// All returns an iterator over the bindings. Iteration order is unspecified.
func (b Bindings) All() iter.Seq2[string, any] { return maps.All(b.values) }

// Clone returns an independent copy of the binding container.
func (b Bindings) Clone() Bindings { return Bindings{values: maps.Clone(b.values)} }

// Binding identifies a typed slot on the blackboard: a variable name plus
// a stable string describing its Go type. The string form ("name:Type") is
// stable across processes so it can act as a planner condition key.
type Binding struct {
	Name string
	Type string

	// goType is the concrete reflect.Type the binding was declared with,
	// retained so a snapshot round-trip can reconstruct the original Go type
	// rather than the generic map JSON decodes into (see snapshotTypeTable).
	// Set only by NewBinding[T]; nil for bindings parsed from their string
	// form (ParseBinding) — those carry no recoverable type information.
	goType reflect.Type
}

// String renders the canonical "name:Type" form. An empty Name normalizes to
// DefaultBindingName so equivalent bindings always serialize identically.
func (b Binding) String() string {
	b = b.Canonical()
	return b.Name + ":" + b.Type
}

// Canonical returns b with the conventional default name made explicit.
func (b Binding) Canonical() Binding {
	if b.Name == "" {
		b.Name = DefaultBindingName
	}
	return b
}

// IsDefault reports whether the binding uses the conventional "it" name.
func (b Binding) IsDefault() bool {
	return b.Name == "" || b.Name == DefaultBindingName
}

// NewBinding constructs a Binding for type T using reflection to derive a
// stable, fully-qualified type name. Pointer types unwrap to their element
// type so "Foo" and "*Foo" share the same binding key.
func NewBinding[T any](name string) Binding {
	if name == "" {
		name = DefaultBindingName
	}

	typ := reflect.TypeFor[T]()
	element := typ
	for element.Kind() == reflect.Pointer {
		element = element.Elem()
	}
	return Binding{
		Name:   name,
		Type:   typeFullName(typ),
		goType: element, // unwrapped to match Type's pointer-normalized name
	}
}

// ParseBinding restores a Binding from its canonical "name:Type" form.
// An input without a colon is treated as type-only and uses the default name.
func ParseBinding(text string) Binding {
	name, typ, ok := strings.Cut(text, ":")
	if !ok {
		return Binding{Name: DefaultBindingName, Type: text}
	}
	return Binding{Name: name, Type: typ}
}

// typeFullName produces a stable identifier for a Go type. Pointers unwrap;
// named types include their package path so different packages with same-
// named types don't collide on the planner's condition keys. Built-ins and
// unnamed types (slices, maps, anon structs) fall back to reflect's String()
// representation.
func typeFullName(typ reflect.Type) string {
	if typ == nil {
		return anyTypeName
	}

	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	if typ.PkgPath() == "" {
		return typ.String()
	}
	return typ.PkgPath() + "." + typ.Name()
}

// TypeNameOf exposes the same type-naming rule used internally so
// callers building bindings outside the [NewBinding]/[TypeName]
// generics path produce identifiers that match [Binding.Type] exactly.
func TypeNameOf(typ reflect.Type) string { return typeFullName(typ) }

// TypeName returns the stable type name for the generic parameter T.
func TypeName[T any]() string {
	return typeFullName(reflect.TypeFor[T]())
}
