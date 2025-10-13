package ptr

// Pointer returns a pointer to the given value.
// This is useful when you need to pass a pointer to a literal value or
// when working with APIs that require pointer parameters.
func Pointer[V any](v V) *V {
	return &v
}

// Value safely dereferences a pointer and returns its value.
// If the pointer is nil, it returns the zero value of type T.
// This prevents nil pointer dereference panics.
func Value[T any](ptr *T) (v T) {
	if ptr != nil {
		return *ptr
	}
	return
}

// Clone creates a new pointer pointing to a copy of the value.
// If the pointer is nil, it returns nil.
func Clone[T any](ptr *T) *T {
	if ptr == nil {
		return nil
	}
	return &*ptr
}
