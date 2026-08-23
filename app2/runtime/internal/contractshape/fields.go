// Package contractshape describes the JSON shape of Runtime protocol values for
// private validation and artifact generation. It is deliberately internal: the
// public protocol surface consists of values, not reflection machinery.
package contractshape

import (
	"fmt"
	"reflect"
	"strings"
)

// How a wire struct becomes JSON is a fact about this package's types, so it is
// answered here — once. The registration-time shape checks, the schema walker and
// the TypeScript emitter all need the same answer, and three private copies of
// encoding/json's naming and inlining rules would be three chances to disagree
// about what the wire actually looks like.

// Field is one JSON field a struct marshals, with embedded structs inlined
// the way encoding/json inlines them.
type Field struct {
	Name string
	Type reflect.Type

	// GoName is the struct field's Go name, which a code generator needs to emit a
	// selector for it. An embedded struct's fields are promoted, so the name alone
	// addresses them — there is no prefix to carry.
	GoName string

	// Optional reports that the encoder may omit the field entirely
	// (`omitempty` / `omitzero`). It is the only honest source for "may this be
	// absent": a required-looking field with omitempty is absent on the wire
	// whenever it is zero, whatever the doc says.
	Optional bool
}

// Fields lists the JSON fields a struct marshals, in declaration order.
// Fields the encoder never emits — unexported, or tagged "-" — are absent: they
// are not on the wire, so nothing about the contract may mention them.
func Fields(owner reflect.Type) []Field {
	if owner.Kind() != reflect.Struct {
		return nil
	}
	var out []Field
	for field := range owner.Fields() {
		name, options := wireNameOf(field)
		if options.embedded {
			out = append(out, Fields(Deref(field.Type))...)
			continue
		}
		if name == "" {
			continue
		}
		out = append(out, Field{Name: name, Type: field.Type, GoName: field.Name, Optional: options.optional})
	}
	return out
}

// Embeds lists the wire shapes owner composes by embedding, outermost first.
// Their fields are inlined into owner's frame, so anything true of those fields
// is true of owner — which is what lets a rule declared for the embedded shape
// follow it rather than being restated.
func Embeds(owner reflect.Type) []reflect.Type {
	owner = Deref(owner)
	if owner.Kind() != reflect.Struct {
		return nil
	}
	var out []reflect.Type
	for field := range owner.Fields() {
		if _, options := wireNameOf(field); !options.embedded {
			continue
		}
		embedded := Deref(field.Type)
		out = append(out, embedded)
		out = append(out, Embeds(embedded)...)
	}
	return out
}

// FieldNames lists just the names, for a caller checking coverage.
func FieldNames(owner reflect.Type) []string {
	fields := Fields(owner)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		out = append(out, field.Name)
	}
	return out
}

// LookupField finds one JSON field by wire name, following embedded structs.
func LookupField(owner reflect.Type, name string) (Field, bool) {
	for _, field := range Fields(owner) {
		if field.Name == name {
			return field, true
		}
	}
	return Field{}, false
}

// HasWirePath reports whether a dotted JSON path (`payload.tool`) addresses a
// real field, returning an error naming the segment that does not exist.
func HasPath(root reflect.Type, path string) error {
	current := root
	for segment := range strings.SplitSeq(path, ".") {
		field, ok := LookupField(current, segment)
		if !ok {
			return fmt.Errorf("no JSON field %q on %s", segment, current.Name())
		}
		current = Deref(field.Type)
	}
	return nil
}

// GoPath resolves a dotted JSON path to the Go selector that reads it, so a
// generator can emit `r.Artifact.Session.ID` for `artifact.session.id`.
func GoPath(root reflect.Type, path string) (selector string, leaf Field, ok bool) {
	current := root
	var parts []string
	for segment := range strings.SplitSeq(path, ".") {
		field, found := LookupField(current, segment)
		if !found {
			return "", Field{}, false
		}
		parts = append(parts, field.GoName)
		leaf = field
		current = Deref(field.Type)
	}
	return strings.Join(parts, "."), leaf, true
}

// Deref unwraps the pointers and slices around a value type, so a path segment
// can address a field of `*T` and `[]T` alike.
func Deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	return t
}

type wireTagOptions struct {
	embedded bool
	optional bool
}

// wireNameOf returns a field's wire name and how the encoder treats it. An
// anonymous struct field with no json name is embedded: its fields are inlined
// onto the same JSON object.
func wireNameOf(field reflect.StructField) (string, wireTagOptions) {
	tag, tagged := field.Tag.Lookup("json")
	if field.Anonymous && (!tagged || tag == "") {
		return "", wireTagOptions{embedded: true}
	}
	if !field.IsExported() {
		return "", wireTagOptions{}
	}
	name, rest, hasOptions := strings.Cut(tag, ",")
	options := wireTagOptions{}
	for option := range strings.SplitSeq(rest, ",") {
		if option == "omitempty" || option == "omitzero" {
			options.optional = true
		}
	}
	switch {
	case !tagged, name == "":
		return field.Name, options
	case name == "-" && !hasOptions:
		// A bare "-" skips the field; `json:"-,"` names it "-".
		return "", wireTagOptions{}
	default:
		return name, options
	}
}
