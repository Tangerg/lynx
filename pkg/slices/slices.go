package slices

// EnsureIndex ensures that the slice can accommodate the specified index.
// If the index is within the current length, the original slice is returned unchanged.
// If the index exceeds the length but is within capacity, the slice is extended.
// If the index exceeds the capacity, a new slice is allocated with sufficient space.
//
// Parameters:
//   - s: The input slice to potentially extend
//   - i: The target index that must be accessible. Must be positive, otherwise panics
//
// Returns:
//   - A slice that can safely access index i
//
// Example:
//
//	s := []int{1, 2, 3}
//	s = EnsureIndex(s, 5)  // s is now length 6: [1, 2, 3, 0, 0, 0]
//	s[5] = 42              // Safe to access
func EnsureIndex[S ~[]E, E any](s S, i int) S {
	if i < 0 {
		panic("index must be positive")
	}
	// Return original slice if index is already accessible
	if i < len(s) {
		return s
	}

	// Extend slice length if index is within capacity
	if i < cap(s) {
		return s[:i+1]
	}

	// Allocate new slice and copy existing elements
	newS := make(S, i+1)
	copy(newS, s)

	return newS
}

// Chunk divides a slice into smaller sub-slices of the specified size.
// All sub-slices except possibly the last one will have exactly 'size' elements.
// The last sub-slice may contain fewer elements if the input length is not evenly divisible by size.
//
// This is a generic function that works with slices of any type.
//
// Parameters:
//   - s: The input slice to be divided
//   - size: The maximum size of each sub-slice. Must be positive, otherwise panics
//
// Returns:
//   - A slice of sub-slices, where each sub-slice has at most 'size' elements
//
// Panics:
//   - If size <= 0
//
// Examples:
//
//	numbers := []int{1, 2, 3, 4, 5, 6}
//	chunks := Chunk(numbers, 2)
//	// Result: [[1, 2], [3, 4], [5, 6]]
//
//	letters := []string{"a", "b", "c", "d", "e"}
//	chunks := Chunk(letters, 3)
//	// Result: [["a", "b", "c"], ["d", "e"]]
func Chunk[S ~[]E, E any](s S, size int) []S {
	if size <= 0 {
		panic("chunk size must be positive")
	}

	var (
		l = len(s)
		// Pre-calculate capacity to avoid slice reallocations
		rv = make([]S, 0, (l+size-1)/size)
	)

	for i := 0; i < l; i += size {
		end := min(i+size, l)
		// Use three-index slicing to set capacity and prevent accidental modification
		rv = append(rv, s[i:end:end])
	}

	return rv
}

// At retrieves an element from the slice at the specified index.
// Supports negative indexing where -1 refers to the last element, -2 to the second-to-last, etc.
// Returns the element and true if the index is valid, or the zero value and false otherwise.
//
// Parameters:
//   - s: The input slice
//   - i: The index to access (supports negative values for reverse indexing)
//
// Returns:
//   - The element at the specified index
//   - A boolean indicating whether the index was valid
//
// Examples:
//
//	slice := []int{10, 20, 30, 40}
//	val, ok := At(slice, 1)    // val = 20, ok = true
//	val, ok := At(slice, -1)   // val = 40, ok = true (last element)
//	val, ok := At(slice, 10)   // val = 0, ok = false (out of bounds)
func At[S ~[]E, E any](s S, i int) (e E, ok bool) {
	l := len(s)

	// Return zero value for empty slice
	if l <= 0 {
		return
	}

	// Convert negative index to positive
	if i < 0 {
		i = l + i
	}
	// Check bounds after conversion
	if i < 0 || i > l-1 {
		return
	}

	return s[i], true
}

// AtOr retrieves an element from the slice at the specified index, or returns a default value.
// This is a convenience function that combines At with a fallback value.
// Supports negative indexing like the At function.
//
// Parameters:
//   - s: The input slice
//   - i: The index to access (supports negative values for reverse indexing)
//   - or: The default value to return if the index is invalid
//
// Returns:
//   - The element at the specified index, or the default value if index is invalid
//
// Examples:
//
//	slice := []int{10, 20, 30}
//	val := AtOr(slice, 1, -1)    // val = 20
//	val := AtOr(slice, -1, -1)   // val = 30 (last element)
//	val := AtOr(slice, 10, -1)   // val = -1 (default value)
func AtOr[S ~[]E, E any](s S, i int, or E) E {
	e, ok := At(s, i)
	if ok {
		return e
	}
	return or
}

// First retrieves the first element from the slice.
// Returns the first element and true if the slice is not empty,
// or the zero value and false if the slice is empty.
//
// Parameters:
//   - s: The input slice
//
// Returns:
//   - The first element of the slice
//   - A boolean indicating whether the slice was non-empty
//
// Example:
//
//	slice := []int{10, 20, 30}
//	val, ok := First(slice)  // val = 10, ok = true
//
//	empty := []int{}
//	val, ok := First(empty)  // val = 0, ok = false
func First[S ~[]E, E any](s S) (E, bool) {
	return At(s, 0)
}

// FirstOr retrieves the first element from the slice, or returns a default value.
// This is a convenience function that combines First with a fallback value.
//
// Parameters:
//   - s: The input slice
//   - or: The default value to return if the slice is empty
//
// Returns:
//   - The first element of the slice, or the default value if slice is empty
//
// Example:
//
//	slice := []int{10, 20, 30}
//	val := FirstOr(slice, -1)  // val = 10
//
//	empty := []int{}
//	val := FirstOr(empty, -1)  // val = -1 (default value)
func FirstOr[S ~[]E, E any](s S, or E) E {
	return AtOr(s, 0, or)
}

// Last retrieves the last element from the slice.
// Returns the last element and true if the slice is not empty,
// or the zero value and false if the slice is empty.
//
// Parameters:
//   - s: The input slice
//
// Returns:
//   - The last element of the slice
//   - A boolean indicating whether the slice was non-empty
//
// Example:
//
//	slice := []int{10, 20, 30}
//	val, ok := Last(slice)   // val = 30, ok = true
//
//	empty := []int{}
//	val, ok := Last(empty)   // val = 0, ok = false
func Last[S ~[]E, E any](s S) (E, bool) {
	return At(s, -1)
}

// LastOr retrieves the last element from the slice, or returns a default value.
// This is a convenience function that combines Last with a fallback value.
//
// Parameters:
//   - s: The input slice
//   - or: The default value to return if the slice is empty
//
// Returns:
//   - The last element of the slice, or the default value if slice is empty
//
// Example:
//
//	slice := []int{10, 20, 30}
//	val := LastOr(slice, -1)   // val = 30
//
//	empty := []int{}
//	val := LastOr(empty, -1)   // val = -1 (default value)
func LastOr[S ~[]E, E any](s S, or E) E {
	return AtOr(s, -1, or)
}
