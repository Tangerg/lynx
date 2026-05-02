package core

import "reflect"

// DomainType describes a Go type that an agent treats as a planning artifact.
// It's used for documentation, schema export (e.g. JSON Schema for MCP), and
// the parent-interface walks the planner does when checking type compatibility
// between an action's outputs and a downstream action's inputs.
type DomainType struct {
	Name        string
	Description string
	ReflectType reflect.Type
	Parents     []string // Stable type names of parent interfaces (for sealed-style hierarchies).
	IsSealed    bool
}

// DomainTypeOf builds a DomainType from a generic parameter — convenient when
// declaring sealed-interface families up front so the planner has the parent
// information it needs.
func DomainTypeOf[T any](description string) DomainType {
	rt := reflect.TypeOf((*T)(nil)).Elem()
	return DomainType{
		Name:        TypeFullName(rt),
		Description: description,
		ReflectType: rt,
	}
}
