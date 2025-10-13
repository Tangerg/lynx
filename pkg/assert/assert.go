package assert

// Must panics if err is not nil, otherwise returns the value.
//
// Example:
//
//	result := assert.Must(someFunction())
func Must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}

	return value
}

// Ensure panics with message if condition is false.
func Ensure(condition bool, message string) {
	if !condition {
		panic(message)
	}
}
