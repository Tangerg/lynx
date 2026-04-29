// Package ptr provides small helpers for working with pointer values.
//
// Use [To] to take the address of a literal or expression, [From] to
// safely dereference a possibly-nil pointer, and [Clone] to make an
// independent copy of the pointee.
//
// These helpers are most useful when interacting with APIs whose
// optional fields are modeled as pointers.
package ptr
