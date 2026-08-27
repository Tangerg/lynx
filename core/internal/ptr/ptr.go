// Package ptr holds pointer helpers shared across Core's protocol packages.
package ptr

func Clone[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}
