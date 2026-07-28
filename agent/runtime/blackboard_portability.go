package runtime

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
)

// The in-memory blackboard owns each value as JSON, so a stored type's Go state
// survives a store/load only where the portable form carries it. These rules
// reject at the write the types whose state does not survive, because silent
// truncation surfaces far from its cause: the write succeeds, and the loss
// appears later as a zeroed field or a missing binding — often only after a
// restore, with nothing left to point at the type that could not be carried.
//
// A type implementing both [json.Marshaler] and [json.Unmarshaler] owns its own
// portable form, so the walk trusts it and stops. That is the supported way to
// put unexported state on the blackboard, and the reason [time.Time] and
// [encoding/json.RawMessage] are accepted.
var (
	jsonMarshalerType   = reflect.TypeFor[json.Marshaler]()
	jsonUnmarshalerType = reflect.TypeFor[json.Unmarshaler]()

	// portableVerdicts caches one verdict per Go type. The walk is pure and the
	// same handful of types are stored over and over.
	portableVerdicts sync.Map // reflect.Type -> error, nil when portable
)

// requirePortableType reports why typ's Go state cannot survive the
// blackboard's portable form, or nil when it can. A nil type is the untyped nil
// value, which round-trips as JSON null.
func requirePortableType(typ reflect.Type) error {
	if typ == nil {
		return nil
	}
	if cached, loaded := portableVerdicts.Load(typ); loaded {
		verdict, _ := cached.(error)
		return verdict
	}
	verdict := portabilityOf(typ, map[reflect.Type]bool{})
	portableVerdicts.Store(typ, verdict)
	return verdict
}

// portabilityOf walks typ's shape. visiting carries the types already on the
// walk so a recursive type is decided by the rest of its shape instead of
// looping.
func portabilityOf(typ reflect.Type, visiting map[reflect.Type]bool) error {
	if ownsPortableForm(typ) {
		return nil
	}
	if visiting[typ] {
		return nil
	}
	visiting[typ] = true
	defer delete(visiting, typ)

	switch typ.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return nil
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return portabilityOf(typ.Elem(), visiting)
	case reflect.Map:
		if err := portabilityOf(typ.Key(), visiting); err != nil {
			return fmt.Errorf("map key: %w", err)
		}
		return portabilityOf(typ.Elem(), visiting)
	case reflect.Struct:
		return portableStruct(typ, visiting)
	case reflect.Interface:
		return fmt.Errorf(
			"%w: %s is an interface, and a JSON round trip cannot recover the concrete type behind it",
			core.ErrUnportableValue, typ,
		)
	default:
		return fmt.Errorf("%w: %s has no portable form", core.ErrUnportableValue, typ)
	}
}

func portableStruct(typ reflect.Type, visiting map[reflect.Type]bool) error {
	for index := range typ.NumField() {
		field := typ.Field(index)
		if field.PkgPath != "" && !promotesExportedFields(field) {
			return fmt.Errorf(
				"%w: %s.%s is unexported, so its state is dropped; implement json.Marshaler and json.Unmarshaler on %s to own its portable form",
				core.ErrUnportableValue, typ, field.Name, typ,
			)
		}
		if field.Tag.Get("json") == "-" {
			return fmt.Errorf(
				"%w: %s.%s is excluded from JSON, so its state is dropped",
				core.ErrUnportableValue, typ, field.Name,
			)
		}
		if err := portabilityOf(field.Type, visiting); err != nil {
			return fmt.Errorf("%s.%s: %w", typ, field.Name, err)
		}
	}
	return nil
}

// promotesExportedFields reports whether field is an embedded struct whose own
// exported fields encoding/json promotes into the outer object. Such a field is
// unexported yet carries its state, so the unexported-field rule must not
// reject it.
func promotesExportedFields(field reflect.StructField) bool {
	if !field.Anonymous {
		return false
	}
	typ := field.Type
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ.Kind() == reflect.Struct
}

// ownsPortableForm reports whether typ carries its own JSON representation in
// both directions. Encoding sees the stored value, so a marshaler reachable
// only through a pointer receiver would not run for a non-pointer type;
// decoding always allocates, so a pointer type's own unmarshaler is enough.
func ownsPortableForm(typ reflect.Type) bool {
	if !typ.Implements(jsonMarshalerType) {
		return false
	}
	if typ.Kind() == reflect.Pointer {
		return typ.Implements(jsonUnmarshalerType)
	}
	return reflect.PointerTo(typ).Implements(jsonUnmarshalerType)
}
