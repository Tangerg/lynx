package protocol

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/runtime/internal/contractshape"
)

// WireValidator is implemented by a DTO whose wire contract is stricter than its
// flat Go representation.
//
// Implementations are GENERATED (wire_constraints.generated.go) from the Contract
// Registry. Value constraints, closed-enum membership, union variants and
// conditional presence rules therefore have one author shared by Go, JSON Schema
// and TypeScript. A hand-written ValidateWire would be a second author.
//
// [ValidateWireTree] composes these node-local validators at delivery boundaries.
// Keeping the generated method local to one DTO avoids generated parent shapes
// restating child rules, while the tree walk makes it impossible for a response
// or event to skip a constrained nested DTO.
//
// ValidateWire stays a pure function of the value — no storage, dispatcher or
// executor (contract §11.2 / §14.6 gate 7). "Does this session exist" is not a
// shape constraint and remains a use-case decision.
type WireValidator interface {
	ValidateWire() error
}

// ValidateWireTree validates every constrained DTO reachable through the JSON
// representation of value. It is the delivery-boundary operation: requests,
// responses, errors and events call it once at their root, and it composes the
// generated node-local [WireValidator] implementations with precise JSON paths.
//
// Interface-valued payloads are intentionally opaque. They carry extension or
// provider data whose schema is owned outside the first-party wire contract, so
// recursively interpreting a concrete value hidden behind `any` would turn an
// implementation detail into protocol.
func ValidateWireTree(value any) error {
	root := reflect.ValueOf(value)
	if !root.IsValid() {
		return nil
	}
	rootType := root.Type()
	for rootType.Kind() == reflect.Pointer {
		rootType = rootType.Elem()
	}
	shape := rootType.Name()
	if shape == "" {
		shape = "wire value"
	}

	fields := validateWireValue(root, "", make(map[wirePointer]bool))
	if len(fields) == 0 {
		return nil
	}
	return &ConstraintError{Shape: shape, Fields: uniqueFieldErrors(fields)}
}

type wirePointer struct {
	typ reflect.Type
	ptr uintptr
}

func validateWireValue(value reflect.Value, path string, visiting map[wirePointer]bool) []FieldError {
	if !value.IsValid() {
		return nil
	}
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		pointer := wirePointer{typ: value.Type(), ptr: value.Pointer()}
		if visiting[pointer] {
			return nil
		}
		visiting[pointer] = true
		defer delete(visiting, pointer)
		value = value.Elem()
	}
	// An interface marks an intentionally opaque wire boundary. Do not unwrap it:
	// Result, payload and Arguments may contain arbitrary third-party JSON.
	if value.Kind() == reflect.Interface {
		return nil
	}

	var fields []FieldError
	if value.CanInterface() {
		if validator, ok := value.Interface().(WireValidator); ok {
			fields = append(fields, prefixedWireErrors(path, validator.ValidateWire())...)
		}
	}

	switch value.Kind() {
	case reflect.Struct:
		for _, field := range contractshape.Fields(value.Type()) {
			fields = append(fields, validateWireValue(
				value.FieldByName(field.GoName),
				joinWirePath(path, field.Name),
				visiting,
			)...)
		}
	case reflect.Slice, reflect.Array:
		for index := range value.Len() {
			fields = append(fields, validateWireValue(
				value.Index(index),
				fmt.Sprintf("%s[%d]", path, index),
				visiting,
			)...)
		}
	}
	return fields
}

func prefixedWireErrors(path string, err error) []FieldError {
	if err == nil {
		return nil
	}
	if constraint, ok := errors.AsType[*ConstraintError](err); ok {
		fields := make([]FieldError, 0, len(constraint.Fields))
		for _, field := range constraint.Fields {
			field.Field = joinWirePath(path, field.Field)
			fields = append(fields, field)
		}
		return fields
	}
	field := path
	if field == "" {
		field = "$"
	}
	return []FieldError{{Field: field, Detail: err.Error()}}
}

func joinWirePath(prefix, field string) string {
	switch {
	case prefix == "":
		return field
	case field == "":
		return prefix
	default:
		return prefix + "." + field
	}
}

func uniqueFieldErrors(fields []FieldError) []FieldError {
	seen := make(map[FieldError]bool, len(fields))
	out := make([]FieldError, 0, len(fields))
	for _, field := range fields {
		if seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
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

func optionalText[Text ~string](field string, value *Text) FieldError {
	if value != nil && *value == "" {
		return FieldError{Field: field, Detail: "must not be empty"}
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

// optionalPositiveScalarNumber treats zero as absence because encoding/json omits an
// optional scalar at its zero value. A negative value is still present and
// illegal. JSON Schema and the TypeScript validator apply minimum: 1 only when
// the property exists, which is the same serialized contract.
func optionalPositiveScalarNumber[Number wireNumber](field string, value Number) FieldError {
	if value < 0 {
		return FieldError{Field: field, Detail: "must be greater than zero when present"}
	}
	return FieldError{}
}

func optionalPositiveNumber[Number wireNumber](field string, value *Number) FieldError {
	if value == nil {
		return FieldError{}
	}
	return positiveNumber(field, *value)
}

type wireNumeric interface {
	wireNumber | ~float32 | ~float64
}

func nonNegativeNumber[Number wireNumeric](field string, value Number) FieldError {
	number := float64(value)
	if math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return FieldError{Field: field, Detail: "must be finite and non-negative"}
	}
	return FieldError{}
}

func optionalNonNegativeNumber[Number wireNumeric](field string, value *Number) FieldError {
	if value == nil {
		return FieldError{}
	}
	return nonNegativeNumber(field, *value)
}

func minimumNumber[Number wireNumeric](field string, value Number, minimum Number) FieldError {
	number := float64(value)
	if math.IsNaN(number) || math.IsInf(number, 0) || number < float64(minimum) {
		return FieldError{Field: field, Detail: fmt.Sprintf("must be at least %v", minimum)}
	}
	return FieldError{}
}

func maximumNumber[Number wireNumeric](field string, value Number, maximum Number) FieldError {
	number := float64(value)
	if math.IsNaN(number) || math.IsInf(number, 0) || number > float64(maximum) {
		return FieldError{Field: field, Detail: fmt.Sprintf("must be at most %v", maximum)}
	}
	return FieldError{}
}

func optionalMaximumNumber[Number wireNumeric](field string, value *Number, maximum Number) FieldError {
	if value == nil {
		return FieldError{}
	}
	return maximumNumber(field, *value, maximum)
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

func optionalMinItems[T any](field string, values []T, minimum int) FieldError {
	if values != nil && len(values) < minimum {
		return FieldError{Field: field, Detail: fmt.Sprintf("must contain at least %d items", minimum)}
	}
	return FieldError{}
}

func maxLength(field, value string, maximum int) FieldError {
	if utf8.RuneCountInString(value) > maximum {
		return FieldError{Field: field, Detail: fmt.Sprintf("must contain at most %d characters", maximum)}
	}
	return FieldError{}
}

func optionalMaxLength(field string, value *string, maximum int) FieldError {
	if value == nil {
		return FieldError{}
	}
	return maxLength(field, *value, maximum)
}

// nonEmptyProperties rejects an empty object map. nil remains a valid omission;
// a present empty map is rejected by the same length check after decoding.
func nonEmptyProperties[Value any](field string, values map[string]Value) FieldError {
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

func optionalUniqueItems[T comparable](field string, values *[]T) FieldError {
	if values == nil {
		return FieldError{}
	}
	return uniqueItems(field, *values)
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

// closedEnumItems applies the same membership rule to every element of an enum
// array. Schema and TypeScript validate at the item boundary; returning the exact
// index keeps Go diagnostics equally actionable.
func closedEnumItems[Enum ~string](field string, items []Enum, values []string) FieldError {
	for index, item := range items {
		if !slices.Contains(values, string(item)) {
			return FieldError{
				Field:  fmt.Sprintf("%s[%d]", field, index),
				Detail: "must be one of " + strings.Join(values, ", "),
			}
		}
	}
	return FieldError{}
}

func unionTag(field, value string, literals []string, pattern string) FieldError {
	if slices.Contains(literals, value) {
		return FieldError{}
	}
	if matched, err := regexp.MatchString(pattern, value); err == nil && matched {
		return FieldError{}
	}
	return FieldError{Field: field, Detail: "must be a known tag or match " + pattern}
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

func wireFieldMatches(value any, path, pattern string) bool {
	field, _, ok := lookupWireValue(reflect.ValueOf(value), path)
	if !ok || field.Kind() != reflect.String {
		return false
	}
	matched, err := regexp.MatchString(pattern, field.String())
	return err == nil && matched
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
		wireField, found := contractshape.LookupField(current.Type(), segment)
		if !found {
			return reflect.Value{}, false, false
		}
		current = current.FieldByName(wireField.GoName)
		optional = wireField.Optional
	}
	return current, optional, current.IsValid()
}
