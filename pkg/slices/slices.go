package slices

// ExpandToFit ensures that the slice is expanded to accommodate the specified index.
// If the index is already within the current length of the slice, the original slice is returned.
// If the index is beyond the length but within the capacity, the slice is resized.
// If the index exceeds the capacity, a new slice is allocated with enough space.
func ExpandToFit[T any](slice []T, index int) []T {
	if index < len(slice) {
		return slice
	}
	if index < cap(slice) {
		return slice[:index+1]
	}
	newSlice := make([]T, index+1)
	copy(newSlice, slice)
	return newSlice
}
