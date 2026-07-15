package filter

import (
	"fmt"
	"reflect"
)

// Visitor processes a complete expression tree. Implementations own traversal
// order and any target-specific state so they can validate, evaluate, or
// compile the tree while returning the first error encountered. Callers should
// pass a predicate accepted by [Validate]; [Parse] and vector-store request
// validation already enforce that boundary.
type Visitor interface {
	Visit(Predicate) error
}

// Visit validates predicate, then passes it to each visitor in order. It
// returns the first visitor error unchanged and does not call later visitors.
// Effects from earlier visitors are not rolled back. A nil visitor, including
// a typed nil, is rejected before any visitor runs.
func Visit(predicate Predicate, visitors ...Visitor) error {
	if err := Validate(predicate); err != nil {
		return err
	}
	for i, visitor := range visitors {
		if isNilVisitor(visitor) {
			return fmt.Errorf("filter.Visit: visitor %d is nil", i)
		}
	}
	for _, visitor := range visitors {
		if err := visitor.Visit(predicate); err != nil {
			return err
		}
	}
	return nil
}

func isNilVisitor(visitor Visitor) bool {
	if visitor == nil {
		return true
	}
	value := reflect.ValueOf(visitor)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
