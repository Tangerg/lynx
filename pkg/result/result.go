// Package result provides a generic Result type for elegant error handling.
// Result type encapsulates a value and an error, avoiding repetitive patterns
// in traditional Go error handling.
package result

import (
	"fmt"
)

// Result represents a value that can either contain a successful result of type T
// or an error. It provides a functional programming approach to error handling.
type Result[T any] struct {
	v   T
	err error
}

// New creates a new Result with both value and error.
// This is useful when you want to wrap existing functions that return (T, error).
func New[T any](v T, err error) Result[T] {
	return Result[T]{
		v:   v,
		err: err,
	}
}

// Value creates a new Result containing a successful value with no error.
func Value[T any](v T) Result[T] {
	return Result[T]{
		v: v,
	}
}

// Error creates a new Result containing only an error with zero value of type T.
func Error[T any](err error) Result[T] {
	return Result[T]{
		err: err,
	}
}

// Get returns both the value and error contained in the Result.
// This method provides compatibility with traditional Go error handling patterns.
func (r *Result[T]) Get() (T, error) {
	return r.v, r.err
}

// Error returns the error contained in the Result, or nil if successful.
func (r *Result[T]) Error() error {
	return r.err
}

// Value returns the value contained in the Result.
// Note: This returns the zero value of T if the Result contains an error.
// Use Get() or check Error() first if you need to handle errors properly.
func (r *Result[T]) Value() T {
	return r.v
}

// String returns a string representation of the Result.
// If the Result contains an error, it returns "error: <error message>".
// If the Result contains a successful value, it returns "value: <value string>".
// For values that implement fmt.Stringer, it uses the String() method,
// otherwise it falls back to fmt.Sprintf with %+v format.
func (r *Result[T]) String() string {
	if r.err != nil {
		return "error: " + r.err.Error()
	}

	stringer, ok := any(r.v).(fmt.Stringer)
	if ok {
		return "value: " + stringer.String()
	}
	return fmt.Sprintf("value: %+v", r.v)
}

// Map applies a function to the value inside the Result if it's successful,
// returning a new Result with the transformed value.
// If the original Result contains an error, the error is propagated to the new Result.
//
// Example:
//
//	result := Value(10)
//	doubled := Map(result, func(x int) int { return x * 2 })
//	// doubled contains Value(20)
func Map[T, U any](res Result[T], fn func(T) U) Result[U] {
	if res.err != nil {
		return Error[U](res.err)
	}
	return Value(fn(res.v))
}
