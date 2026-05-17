package slices

// Map applies f to each element of s and returns a new slice with the
// results. The returned slice is always non-nil and has len(s) entries
// even when s is empty.
//
// Example:
//
//	slices.Map([]float32{1, 2, 3}, func(v float32) float64 {
//	    return float64(v)
//	}) // [1 2 3]
func Map[S ~[]E, E any, R any](s S, f func(E) R) []R {
	out := make([]R, len(s))
	for i, v := range s {
		out[i] = f(v)
	}
	return out
}

// EnsureIndex returns a slice whose length is at least i+1, so that
// s[i] is safe to read or assign. If i is already addressable, s is
// returned unchanged. If it fits within cap(s), the length is extended
// in place; otherwise a new backing array is allocated and existing
// elements are copied.
//
// EnsureIndex panics if i is negative.
//
// Example:
//
//	s := []int{1, 2, 3}
//	s = slices.EnsureIndex(s, 5) // len 6, [1 2 3 0 0 0]
//	s[5] = 42
func EnsureIndex[S ~[]E, E any](s S, i int) S {
	if i < 0 {
		panic("slices: index must be non-negative")
	}
	if i < len(s) {
		return s
	}
	if i < cap(s) {
		return s[:i+1]
	}
	out := make(S, i+1)
	copy(out, s)
	return out
}

// Chunk splits s into contiguous sub-slices of length size. The final
// chunk may be shorter if len(s) is not a multiple of size. Each
// returned chunk shares storage with s but has its capacity bounded to
// its length, so appending to a chunk does not overwrite the next.
//
// Chunk panics if size <= 0.
//
// Example:
//
//	slices.Chunk([]int{1, 2, 3, 4, 5}, 2) // [[1 2] [3 4] [5]]
func Chunk[S ~[]E, E any](s S, size int) []S {
	if size <= 0 {
		panic("slices: chunk size must be positive")
	}
	n := len(s)
	out := make([]S, 0, (n+size-1)/size)
	for i := 0; i < n; i += size {
		end := min(i+size, n)
		out = append(out, s[i:end:end])
	}
	return out
}

// At returns the element at index i and reports whether the index was
// valid. Negative indices count from the end: -1 is the last element.
// On out-of-range access, At returns the zero value and false.
//
// Example:
//
//	v, ok := slices.At([]int{10, 20, 30}, -1) // 30, true
func At[S ~[]E, E any](s S, i int) (e E, ok bool) {
	n := len(s)
	if n == 0 {
		return
	}
	if i < 0 {
		i += n
	}
	if i < 0 || i >= n {
		return
	}
	return s[i], true
}

// AtOr is like [At] but returns or when the index is out of range.
func AtOr[S ~[]E, E any](s S, i int, or E) E {
	if e, ok := At(s, i); ok {
		return e
	}
	return or
}

// First returns the first element of s and whether s is non-empty.
func First[S ~[]E, E any](s S) (E, bool) {
	return At(s, 0)
}

// FirstOr returns the first element of s, or or if s is empty.
func FirstOr[S ~[]E, E any](s S, or E) E {
	return AtOr(s, 0, or)
}

// Last returns the last element of s and whether s is non-empty.
func Last[S ~[]E, E any](s S) (E, bool) {
	return At(s, -1)
}

// LastOr returns the last element of s, or or if s is empty.
func LastOr[S ~[]E, E any](s S, or E) E {
	return AtOr(s, -1, or)
}
