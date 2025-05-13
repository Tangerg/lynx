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

// Split divides a slice into smaller slices of a specified size.
//
// This is a generic function that works with slices of any type.
//
// Parameters:
//   - slice: The input slice to be divided.
//   - size: The maximum size of each sub-slice. If size <= 0, the function returns
//     a two-dimensional slice containing the original slice as the only element.
//
// Returns:
//   - [][]T: A two-dimensional slice where each sub-slice has a maximum length of `size`.
//     The last sub-slice may have fewer elements if the total number of elements
//     in the input slice is not evenly divisible by `size`.
//
// Examples:
//
//	slice := []int{1, 2, 3, 4, 5, 6}
//	result := Split(slice, 2)
//	// result: [[1 2] [3 4] [5 6]]
//
//	slice := []string{"a", "b", "c", "d", "e"}
//	result := Split(slice, 3)
//	// result: [["a" "b" "c"] ["d" "e"]]
func Split[T any](slice []T, size int) [][]T {
	if size <= 0 {
		return [][]T{slice}
	}

	var (
		rv     [][]T
		length = len(slice)
	)

	for i := 0; i < length; i += size {
		end := min(i+size, length)
		rv = append(rv, slice[i:end:end])
	}
	return rv
}
