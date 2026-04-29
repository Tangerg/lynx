package assert

// Must returns value if err is nil; otherwise it panics with err.
//
// It is intended to wrap calls whose error indicates a programmer
// mistake rather than a runtime condition, typically during package
// initialization.
//
// Example:
//
//	var re = assert.Must(regexp.Compile(`^\d+$`))
func Must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// Ensure panics with err if condition is false. It is meant for
// invariant and precondition checks whose violation represents a bug.
//
// Example:
//
//	assert.Ensure(len(items) > 0, errors.New("items must not be empty"))
func Ensure(condition bool, err error) {
	if !condition {
		panic(err)
	}
}
