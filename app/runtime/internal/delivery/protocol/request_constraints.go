package protocol

import (
	"slices"
	"strings"
)

// Validator is implemented by a request DTO that carries a constraint its JSON
// shape alone cannot express: an id that must be present, a revision that must
// be positive, an enum that must be one of a closed set.
//
// The implementations are GENERATED (request_constraints.generated.go) from the
// declared value constraints, so the Go check, the TypeScript check and the
// schema's minLength / minimum all come from one statement. Only the interface,
// the error type and these helpers are hand-written; a hand-written Validate()
// would be a second author of a rule the schema also states.
//
// Delivery calls Validate immediately after decoding and before the request
// reaches any use case, mapping a failure to invalid_params (API.md §8.2). A
// constraint therefore belongs HERE and not in a handler: it is a property of
// the request shape, so every transport and every generated client reads one
// statement of it. A handler that re-checks the same field is a second author.
//
// Validate must stay a pure function of the value — no storage, no dispatcher,
// no executor (contract §11.2 / §14.6 gate 7). "Does this session exist" is not
// a shape constraint and is answered by the use case.
type Validator interface {
	Validate() error
}

// ConstraintError reports which fields of a request violated their constraints.
// It is what makes [ProblemData.Errors] answerable: a validation failure knows
// the offending params key, so delivery can hand the client a per-field list to
// flag instead of one prose sentence it would have to parse (API.md §8.3).
//
// Detail strings are programmer diagnostics, not UI copy — a client renders its
// own localized message keyed by field + type (the §8.2 lookup), exactly as it
// does for a ProblemData.type.
type ConstraintError struct {
	Fields []FieldError
}

func (e *ConstraintError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Field+": "+f.Detail)
	}
	return strings.Join(parts, "; ")
}

// violate returns nil when there is nothing to report, so a Validate built from
// several checks composes without the caller testing each one.
func violate(fields ...FieldError) error {
	present := make([]FieldError, 0, len(fields))
	for _, f := range fields {
		if f.Field != "" {
			present = append(present, f)
		}
	}
	if len(present) == 0 {
		return nil
	}
	return &ConstraintError{Fields: present}
}

func required(field, value string) FieldError {
	if value == "" {
		return FieldError{Field: field, Detail: "is required"}
	}
	return FieldError{}
}

func positive(field string, value uint64) FieldError {
	if value == 0 {
		return FieldError{Field: field, Detail: "must be greater than zero"}
	}
	return FieldError{}
}

// nonEmptyItems rejects an array that was SENT with nothing in it. An absent
// optional array is untouched — a nil slice is the field's absence, which is what
// distinguishes "no filter" from "a filter that matches nothing", and it is the
// same distinction the schema's minItems draws by applying only when the property
// is present.
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

// oneOf rejects a value outside a closed set. Go's decoder puts any string into a
// named string type, so without this an unknown tag would reach a use case instead
// of failing as invalid_params. optional allows the empty string: an absent optional
// enum is the caller declining to choose, not a bad value.
func oneOf(field, value string, values []string, optional bool) FieldError {
	if value == "" && optional {
		return FieldError{}
	}
	if slices.Contains(values, value) {
		return FieldError{}
	}
	return FieldError{Field: field, Detail: "must be one of " + strings.Join(values, ", ")}
}
