package core

import (
	"reflect"
	"strings"
)

const (
	// DefaultBindingName is the implicit variable name when callers
	// don't supply one; the planner falls back to "the most recent
	// value of this type" when it sees this name.
	DefaultBindingName = "it"

	// LastResultBindingName addresses the single most-recently-added
	// object regardless of type.
	LastResultBindingName = "last_result"
)

// IOBinding identifies a typed slot on the blackboard: a variable name plus
// a stable string describing its Go type. The string form ("name:Type") is
// stable across processes so it can act as a planner condition key.
type IOBinding struct {
	Name string
	Type string
}

// String renders the canonical "name:Type" form. An empty Name normalizes to
// DefaultBindingName so equivalent bindings always serialize identically.
func (b IOBinding) String() string {
	name := b.Name
	if name == "" {
		name = DefaultBindingName
	}
	return name + ":" + b.Type
}

// IsDefault reports whether the binding uses the conventional "it" name.
func (b IOBinding) IsDefault() bool {
	return b.Name == "" || b.Name == DefaultBindingName
}

// NewIOBinding constructs an IOBinding for type T using reflection to derive a
// stable, fully-qualified type name. Pointer types unwrap to their element
// type so "Foo" and "*Foo" share the same binding key.
func NewIOBinding[T any](name string) IOBinding {
	if name == "" {
		name = DefaultBindingName
	}

	return IOBinding{
		Name: name,
		Type: typeFullName(reflect.TypeFor[T]()),
	}
}

// ParseIOBinding restores an IOBinding from its canonical "name:Type" form.
// An input without a colon is treated as type-only and uses the default name.
func ParseIOBinding(s string) IOBinding {
	name, typ, ok := strings.Cut(s, ":")
	if !ok {
		return IOBinding{Name: DefaultBindingName, Type: s}
	}
	return IOBinding{Name: name, Type: typ}
}

// typeFullName produces a stable identifier for a Go type. Pointers unwrap;
// named types include their package path so different packages with same-
// named types don't collide on the planner's condition keys. Built-ins and
// unnamed types (slices, maps, anon structs) fall back to reflect's String()
// representation.
func typeFullName(rt reflect.Type) string {
	if rt == nil {
		return "any"
	}

	for rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}

	if rt.PkgPath() == "" {
		return rt.String()
	}
	return rt.PkgPath() + "." + rt.Name()
}

// TypeFullName exposes the same type-naming rule used internally so
// callers building IOBindings outside the [NewIOBinding]/[TypeName]
// generics path produce identifiers that match [IOBinding.Type] exactly.
func TypeFullName(rt reflect.Type) string { return typeFullName(rt) }

// TypeName returns the stable type name for the generic parameter T.
func TypeName[T any]() string {
	return typeFullName(reflect.TypeFor[T]())
}
