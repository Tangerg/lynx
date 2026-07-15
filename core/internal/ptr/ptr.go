// Package ptr holds pointer helpers shared across Core's protocol packages.
package ptr

// Clone returns an independent copy of an optional value, preserving nil. It
// backs the deep-copy guarantee of the modality Options.Clone methods and the
// chat response accumulator's copy-on-write snapshots: mutating a cloned
// field's pointee must never alias the original.
func Clone[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}
