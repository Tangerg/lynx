package result

import "fmt"

// Result holds either a successful value of type T or an error.
// The zero Result is a successful zero T with nil error.
type Result[T any] struct {
	v   T
	err error
}

// New wraps the standard (T, error) return into a Result. It is the
// usual way to lift an existing function call.
//
// Example:
//
//	r := result.New(strconv.Atoi("42"))
func New[T any](v T, err error) Result[T] {
	return Result[T]{v: v, err: err}
}

// Value returns a successful Result containing v.
func Value[T any](v T) Result[T] {
	return Result[T]{v: v}
}

// Error returns a failed Result with the given error and a zero value
// of T.
func Error[T any](err error) Result[T] {
	return Result[T]{err: err}
}

// Get returns both the value and error in a single call, mirroring the
// idiomatic Go return pair.
func (r *Result[T]) Get() (T, error) {
	return r.v, r.err
}

// Error returns the contained error, or nil if r is successful.
func (r *Result[T]) Error() error {
	return r.err
}

// Value returns the contained value. If r holds an error, the zero
// value of T is returned; check [Result.Error] first when that matters.
func (r *Result[T]) Value() T {
	return r.v
}

// String returns "error: <msg>" for a failed result, or "value: <v>"
// for a successful one. Values implementing fmt.Stringer use their
// String method; otherwise %+v formatting is used.
func (r *Result[T]) String() string {
	if r.err != nil {
		return "error: " + r.err.Error()
	}
	if s, ok := any(r.v).(fmt.Stringer); ok {
		return "value: " + s.String()
	}
	return fmt.Sprintf("value: %+v", r.v)
}

// Map applies fn to the value of r if r is successful, returning a new
// Result of the mapped type. If r holds an error, the error is
// propagated unchanged.
//
// Example:
//
//	r := result.New(strconv.Atoi(s))
//	doubled := result.Map(r, func(n int) int { return n * 2 })
func Map[T, U any](r Result[T], fn func(T) U) Result[U] {
	if r.err != nil {
		return Error[U](r.err)
	}
	return Value(fn(r.v))
}
