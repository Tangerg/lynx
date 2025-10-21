package math

// NumericType encompasses all Go numeric types for literal creation.
type NumericType interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// ConvertSlice converts a slice from one numeric type to another.
// It preserves nil slices and performs element-wise type conversion.
//
// Warning: Conversion may cause precision loss (float to int) or
// overflow (larger to smaller integer types).
//
// Type parameters:
//   - T1: source numeric type
//   - T2: target numeric type
//
// Parameters:
//   - source: the input slice to convert
//
// Returns:
//   - A new slice of type T2, or nil if source is nil
//
// Example:
//
//	ints := []int{1, 2, 3}
//	floats := math.ConvertSlice[int, float64](ints)
//	// Result: []float64{1.0, 2.0, 3.0}
func ConvertSlice[T1, T2 NumericType](source []T1) []T2 {
	// Preserve nil slice semantics
	if source == nil {
		return nil
	}

	// Pre-allocate result slice with exact capacity
	result := make([]T2, len(source))

	// Convert each element
	for i, value := range source {
		result[i] = T2(value)
	}

	return result
}
