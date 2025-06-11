package slices

// EnsureIndex ensures that the slice can accommodate the specified index.
// If the index is within the current length, the original slice is returned unchanged.
// If the index exceeds the length but is within capacity, the slice is extended.
// If the index exceeds the capacity, a new slice is allocated with sufficient space.
//
// Parameters:
//   - s: The input slice to potentially extend
//   - i: The target index that must be accessible
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
	if i < len(s) {
		return s
	}

	if i < cap(s) {
		return s[:i+1]
	}

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
//   - size: The maximum size of each sub-slice. If size <= 0, returns a slice
//     containing the original slice as the only element
//
// Returns:
//   - A slice of sub-slices, where each sub-slice has at most 'size' elements
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
		return []S{s}
	}

	var (
		l  = len(s)
		rv = make([]S, 0, (l+size-1)/size)
	)

	for i := 0; i < l; i += size {
		end := min(i+size, l)
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

	if l <= 0 {
		return
	}

	if i < 0 {
		i = l + i
	}
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
