// Package nilvalue centralizes the reflection needed to recognize interface
// values that hold a typed nil. Public boundaries use it wherever nil means
// "capability absent" or "invalid input".
package nilvalue

import "reflect"

// Is reports whether value is nil or holds a nil value of a nil-capable kind.
func Is(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
