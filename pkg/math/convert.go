package math

// ConvertSlice converts each element of src from numeric type T1 to
// T2. It returns nil if src is nil and an empty slice if src is empty.
//
// Conversion is element-wise type conversion (T2(v)); precision loss
// (float→int) and overflow (e.g. int64→int8) follow Go's standard
// conversion rules and are not detected.
//
// Example:
//
//	v64 := []float64{1.5, 2.5, 3.5}
//	v32 := math.ConvertSlice[float64, float32](v64)
func ConvertSlice[T1, T2 NumericType](src []T1) []T2 {
	if src == nil {
		return nil
	}
	out := make([]T2, len(src))
	for i, v := range src {
		out[i] = T2(v)
	}
	return out
}
