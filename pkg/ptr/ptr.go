package ptr

// To returns a pointer to v. It is convenient for taking the address of
// a literal or function result, which is not directly addressable in Go.
//
// Example:
//
//	req := &Request{Timeout: ptr.To(30 * time.Second)}
func To[T any](v T) *T {
	return &v
}

// From returns the value pointed to by p, or the zero value of T if p
// is nil. It avoids manual nil checks at call sites.
//
// Example:
//
//	timeout := ptr.From(req.Timeout) // 0 if req.Timeout is nil
func From[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// Clone returns a new pointer to a copy of *p, or nil if p is nil.
// The copy is shallow: pointers, slices, and maps inside *p still
// reference the original backing data.
//
// Example:
//
//	dup := ptr.Clone(cfg.Retries) // independent *int with the same value
func Clone[T any](p *T) *T {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}
