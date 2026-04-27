// Package assert provides minimal panic-based assertion helpers for
// situations where a failure indicates a programmer error rather than a
// recoverable condition.
//
// Use [Must] to unwrap (value, error) results from functions whose error
// must never occur in normal operation, such as compiling a regular
// expression with a constant pattern. Use [Ensure] to enforce invariants
// and preconditions.
//
// These helpers are intended for initialization code and internal
// invariants. Do not use them in place of normal error handling on
// runtime input.
package assert
