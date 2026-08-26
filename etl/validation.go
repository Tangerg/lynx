package etl

import (
	"errors"
	"reflect"
)

// ErrNilDocument reports a nil document at an ETL boundary.
var ErrNilDocument = errors.New("etl: document must not be nil")

func isNil(value any) bool {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return true
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
