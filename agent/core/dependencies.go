package core

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

var (
	// ErrInvalidDependencyKey reports an empty or otherwise malformed key.
	ErrInvalidDependencyKey = errors.New("agent: invalid dependency key")
	// ErrDependencyNotFound reports that no scope in the parent chain contains
	// the requested key.
	ErrDependencyNotFound = errors.New("agent: dependency not found")
	// ErrDependencyTypeMismatch reports that a nearer scope contains the requested
	// name under a different declared type. Resolution fails instead of silently
	// falling through to a parent value.
	ErrDependencyTypeMismatch = errors.New("agent: dependency type mismatch")
	// ErrDependencyExists reports a duplicate local registration.
	ErrDependencyExists = errors.New("agent: dependency already registered")
	// ErrDependenciesFrozen reports a registration attempted after a scope's
	// composition phase ended.
	ErrDependenciesFrozen = errors.New("agent: dependencies are frozen")
	// ErrNilDependency reports an untyped or typed nil registration. A nil
	// dependency is ambiguous with absence and is therefore rejected.
	ErrNilDependency = errors.New("agent: nil dependency")
)

// DependencyKey is a typed, named dependency slot. Define keys once (normally as
// package variables) and share the value with the host and actions that use the
// dependency. A key's name is also its shadowing identity across nested scopes;
// declaring the same name with a different T produces an explicit type error.
//
// The zero value is invalid. Use [NewDependencyKey] or [MustDependencyKey].
type DependencyKey[T any] struct {
	name string
	typ  reflect.Type
}

// NewDependencyKey validates name and returns a typed dependency key.
func NewDependencyKey[T any](name string) (DependencyKey[T], error) {
	key := DependencyKey[T]{name: name, typ: reflect.TypeFor[T]()}
	if err := key.validate(); err != nil {
		return DependencyKey[T]{}, fmt.Errorf("%w: name must be non-empty without surrounding whitespace", ErrInvalidDependencyKey)
	}
	return key, nil
}

// MustDependencyKey is the declaration-time companion to [NewDependencyKey]. It
// panics on an invalid programmer-authored key name.
func MustDependencyKey[T any](name string) DependencyKey[T] {
	key, err := NewDependencyKey[T](name)
	if err != nil {
		panic(err)
	}
	return key
}

// Name returns the stable diagnostic and shadowing name of the key.
func (k DependencyKey[T]) Name() string { return k.name }

type dependencyEntry struct {
	typ   reflect.Type
	value any
}

// Dependencies is a concurrency-safe hierarchy of typed runtime dependencies.
// Registrations are local and single-assignment. Lookup walks from the current
// scope toward its parent, so a process or action dependency shadows the same
// key registered on the engine.
//
// This is a narrow host-injection seam for dynamic and declarative agents, not
// a global DI container. Statically wired actions should still prefer constructor,
// struct-field, or closure injection.
type Dependencies struct {
	mu      sync.RWMutex
	parent  *Dependencies
	entries map[string]dependencyEntry
	frozen  bool
}

// NewDependencies returns an empty root scope.
func NewDependencies() *Dependencies {
	return &Dependencies{entries: make(map[string]dependencyEntry)}
}

// Child returns an empty child whose unresolved keys fall back to d. A child
// remains mutable even when its parent is frozen.
func (d *Dependencies) Child() *Dependencies {
	return &Dependencies{parent: d, entries: make(map[string]dependencyEntry)}
}

// Parent returns the immediate parent scope, or nil for a root.
func (d *Dependencies) Parent() *Dependencies {
	if d == nil {
		return nil
	}
	return d.parent
}

// Freeze makes the local scope immutable. It is idempotent and does not freeze
// parent or child scopes.
func (d *Dependencies) Freeze() {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.frozen = true
	d.mu.Unlock()
}

// Frozen reports whether the local composition phase has ended.
func (d *Dependencies) Frozen() bool {
	if d == nil {
		return true
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.frozen
}

// RegisterDependency registers value under key in the local scope. It never
// overwrites an existing registration.
func RegisterDependency[T any](dependencies *Dependencies, key DependencyKey[T], value T) error {
	if err := key.validate(); err != nil {
		return fmt.Errorf("core.RegisterDependency: %w", err)
	}
	if dependencies == nil {
		return fmt.Errorf("core.RegisterDependency %q: %w", key.name, ErrDependenciesFrozen)
	}
	if valueIsNil(value) {
		return fmt.Errorf("core.RegisterDependency %q: %w", key.name, ErrNilDependency)
	}

	dependencies.mu.Lock()
	defer dependencies.mu.Unlock()
	if dependencies.frozen {
		return fmt.Errorf("core.RegisterDependency %q: %w", key.name, ErrDependenciesFrozen)
	}
	if existing, ok := dependencies.entries[key.name]; ok {
		return fmt.Errorf(
			"core.RegisterDependency %q: %w (existing %s, new %s)",
			key.name,
			ErrDependencyExists,
			existing.typ,
			key.typ,
		)
	}
	dependencies.entries[key.name] = dependencyEntry{typ: key.typ, value: value}
	return nil
}

// LookupDependency returns the nearest value registered for key. Missing keys and
// same-name/different-type shadowing are distinct errors.
func LookupDependency[T any](dependencies *Dependencies, key DependencyKey[T]) (T, error) {
	var zero T
	if err := key.validate(); err != nil {
		return zero, fmt.Errorf("core.LookupDependency: %w", err)
	}
	for scope := dependencies; scope != nil; scope = scope.parent {
		scope.mu.RLock()
		entry, ok := scope.entries[key.name]
		scope.mu.RUnlock()
		if !ok {
			continue
		}
		if entry.typ != key.typ {
			return zero, fmt.Errorf(
				"core.LookupDependency %q: %w (registered %s, requested %s)",
				key.name,
				ErrDependencyTypeMismatch,
				entry.typ,
				key.typ,
			)
		}
		value, ok := entry.value.(T)
		if !ok {
			return zero, fmt.Errorf(
				"core.LookupDependency %q: %w (stored %T, requested %s)",
				key.name,
				ErrDependencyTypeMismatch,
				entry.value,
				key.typ,
			)
		}
		return value, nil
	}
	return zero, fmt.Errorf("core.LookupDependency %q: %w", key.name, ErrDependencyNotFound)
}

func (k DependencyKey[T]) validate() error {
	if k.name == "" || k.name != strings.TrimSpace(k.name) || k.typ == nil {
		return ErrInvalidDependencyKey
	}
	return nil
}

func valueIsNil(value any) bool {
	v := reflect.ValueOf(value)
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
