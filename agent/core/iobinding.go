package core

import (
	"reflect"
	"strings"
)

const (
	// DefaultBindingName is the implicit variable name when callers don't
	// supply one; the planner falls back to "the most recent value of this
	// type" when it sees this name.
	DefaultBindingName = "it"

	// LastResultBindingName addresses the single most-recently-added object
	// regardless of type — mirrors embabel's @Trigger(lastResult=true) idiom.
	LastResultBindingName = "lastResult"

	// DefaultBinding is the legacy alias kept for backward compatibility with
	// the design docs and earlier example code.
	DefaultBinding = DefaultBindingName

	// LastResultBinding is the legacy alias for LastResultBindingName.
	LastResultBinding = LastResultBindingName
)

// IoBinding identifies a typed slot on the blackboard: a variable name plus
// a stable string describing its Go type. The string form ("name:Type") is
// stable across processes so it can act as a planner condition key.
type IoBinding struct {
	Name string
	Type string
}

// String renders the canonical "name:Type" form. An empty Name normalizes to
// DefaultBindingName so equivalent bindings always serialize identically.
func (b IoBinding) String() string {
	name := b.Name
	if name == "" {
		name = DefaultBindingName
	}
	return name + ":" + b.Type
}

// IsDefault reports whether the binding uses the conventional "it" name.
func (b IoBinding) IsDefault() bool {
	return b.Name == "" || b.Name == DefaultBindingName
}

// NewIoBinding constructs an IoBinding for type T using reflection to derive a
// stable, fully-qualified type name. Pointer types unwrap to their element
// type so "Foo" and "*Foo" share the same binding key.
func NewIoBinding[T any](name string) IoBinding {
	if name == "" {
		name = DefaultBindingName
	}

	return IoBinding{
		Name: name,
		Type: typeFullName(reflect.TypeFor[T]()),
	}
}

// ParseIoBinding restores an IoBinding from its canonical "name:Type" form.
// An input without a colon is treated as type-only and uses the default name.
func ParseIoBinding(s string) IoBinding {
	name, typ, ok := strings.Cut(s, ":")
	if !ok {
		return IoBinding{Name: DefaultBindingName, Type: s}
	}
	return IoBinding{Name: name, Type: typ}
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

// TypeFullName exposes the same type-naming rule used internally so callers
// (DSL, reflection layer, codegen) produce identifiers that match
// IoBinding.Type exactly.
func TypeFullName(rt reflect.Type) string { return typeFullName(rt) }

// TypeFullNameOf returns the stable type name for the generic parameter T.
func TypeFullNameOf[T any]() string {
	return typeFullName(reflect.TypeFor[T]())
}
