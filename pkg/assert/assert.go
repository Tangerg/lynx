package assert

// ErrorIsNil checks if the error from a function call is nil and returns the value.
// If the error is not nil, it panics with the error.
//
// It's a generic function that accepts any type T as the return value,
// making it usable with any function that returns a value and an error.
//
// Example usage:
//
//	result := assert.ErrorNil(someFunction())
//
// Parameters:
//
//	v   - The value returned by the function
//	err - The error returned by the function
//
// Returns:
//
//	The value v if err is nil
//
// Panics:
//
//	If err is not nil
func ErrorIsNil[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func Assert(cond bool, message string) {
	if !cond {
		panic(message)
	}
}
