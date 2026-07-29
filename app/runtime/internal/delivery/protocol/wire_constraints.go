package protocol

import (
	"reflect"
	"slices"
	"strings"
)

// WireValidator is implemented by a DTO whose wire contract is stricter than its
// flat Go representation.
//
// Implementations are GENERATED (wire_constraints.generated.go) from the Contract
// Registry. Value constraints, closed-enum membership, union variants and
// conditional presence rules therefore have one author shared by Go, JSON Schema
// and TypeScript. A hand-written ValidateWire would be a second author.
//
// Delivery calls ValidateWire immediately after decoding a request and before it
// reaches any use case, mapping a failure to invalid_params (API.md §8.2). Output
// boundaries call the same method before publishing a value. Shape constraints
// therefore live here rather than in handlers or presenters.
//
// ValidateWire stays a pure function of the value — no storage, dispatcher or
// executor (contract §11.2 / §14.6 gate 7). "Does this session exist" is not a
// shape constraint and remains a use-case decision.
type WireValidator interface {
	ValidateWire() error
}

// ConstraintError reports which fields of a wire shape violated their contract.
// Shape qualifies diagnostics without polluting [FieldError.Field]: request errors
// still carry client-addressable paths such as "scope.type", while logs say
// "ListItemsRequest.scope.type".
//
// Detail strings are programmer diagnostics, not UI copy — a client renders its
// own localized message keyed by field + type (the §8.2 lookup), exactly as it
// does for a ProblemData.type.
type ConstraintError struct {
	Shape  string
	Fields []FieldError
}

func (e *ConstraintError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		path := f.Field
		if e.Shape != "" {
			path = e.Shape + "." + path
		}
		parts = append(parts, path+": "+f.Detail)
	}
	return strings.Join(parts, "; ")
}

// Enrich preserves the exact offending fields when a nested decoder returns the
// error through the normal dispatcher error path.
func (e *ConstraintError) Enrich(data *ProblemData) {
	data.Errors = append(data.Errors, e.Fields...)
}

// collectWireViolations returns nil when there is nothing to report, so a
// generated validator can compose independent rules without branching around
// every check.
func collectWireViolations(shape string, fields ...FieldError) error {
	present := make([]FieldError, 0, len(fields))
	for _, f := range fields {
		if f.Field != "" {
			present = append(present, f)
		}
	}
	if len(present) == 0 {
		return nil
	}
	return &ConstraintError{Shape: shape, Fields: present}
}

func requiredText(field, value string) FieldError {
	if value == "" {
		return FieldError{Field: field, Detail: "is required"}
	}
	return FieldError{}
}

type wireNumber interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

func positiveNumber[Number wireNumber](field string, value Number) FieldError {
	if value <= 0 {
		return FieldError{Field: field, Detail: "must be greater than zero"}
	}
	return FieldError{}
}

// requiredItems rejects an absent or empty required array. Requiredness comes
// from the DTO's JSON tag; the generator selects this helper for a field without
// omitempty so the runtime enforces the same required + minItems contract as the
// schema and generated client.
func requiredItems[T any](field string, values []T) FieldError {
	if values == nil {
		return FieldError{Field: field, Detail: "is required"}
	}
	if len(values) == 0 {
		return FieldError{Field: field, Detail: "must not be empty"}
	}
	return FieldError{}
}

// nonEmptyItems rejects an optional array that was sent with nothing in it. A
// nil slice is the field's absence, which remains valid for an optional field.
func nonEmptyItems[T any](field string, values []T) FieldError {
	if values != nil && len(values) == 0 {
		return FieldError{Field: field, Detail: "must not be empty"}
	}
	return FieldError{}
}

// uniqueItems rejects a repeated element, so a filter that is a set is checked as
// one. Comparing values directly keeps the rule independent of order: the caller
// may list them however it likes.
func uniqueItems[T comparable](field string, values []T) FieldError {
	seen := make(map[T]bool, len(values))
	for _, value := range values {
		if seen[value] {
			return FieldError{Field: field, Detail: "must not repeat a value"}
		}
		seen[value] = true
	}
	return FieldError{}
}

// closedEnum rejects a value outside a closed set. Go's decoder puts any string
// into a named string type, so typing alone cannot enforce membership.
func closedEnum(field, value string, values []string, optional bool) FieldError {
	if value == "" && optional {
		return FieldError{}
	}
	if slices.Contains(values, value) {
		return FieldError{}
	}
	return FieldError{Field: field, Detail: "must be one of " + strings.Join(values, ", ")}
}

func requiredWhen(applies bool, field string, value any) FieldError {
	if applies && !wireFieldPresent(value, field) {
		return FieldError{Field: field, Detail: "is required"}
	}
	return FieldError{}
}

func forbiddenWhen(applies bool, field string, value any) FieldError {
	if applies && wireFieldPresent(value, field) {
		return FieldError{Field: field, Detail: "must not be present here"}
	}
	return FieldError{}
}

func wireFieldEquals(value any, path, expected string) bool {
	field, _, ok := lookupWireValue(reflect.ValueOf(value), path)
	return ok && field.Kind() == reflect.String && field.String() == expected
}

// wireFieldPresent answers whether encoding/json would put a registered field on
// the wire. The registry has already proved every path exists; this reflection is
// only the shared runtime interpretation of presence, including nested pointers
// and omitempty collections.
func wireFieldPresent(value any, path string) bool {
	field, optional, ok := lookupWireValue(reflect.ValueOf(value), path)
	if !ok {
		return false
	}
	if !optional {
		return true
	}
	switch field.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return field.Len() > 0
	case reflect.Bool:
		return field.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return field.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return field.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return field.Float() != 0
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Pointer:
		return !field.IsNil()
	default:
		return !field.IsZero()
	}
}

func lookupWireValue(value reflect.Value, path string) (reflect.Value, bool, bool) {
	current := value
	var optional bool
	for segment := range strings.SplitSeq(path, ".") {
		for current.IsValid() && (current.Kind() == reflect.Interface || current.Kind() == reflect.Pointer) {
			if current.IsNil() {
				return reflect.Value{}, false, false
			}
			current = current.Elem()
		}
		if !current.IsValid() || current.Kind() != reflect.Struct {
			return reflect.Value{}, false, false
		}
		wireField, found := LookupWireField(current.Type(), segment)
		if !found {
			return reflect.Value{}, false, false
		}
		current = current.FieldByName(wireField.GoName)
		optional = wireField.Optional
	}
	return current, optional, current.IsValid()
}
