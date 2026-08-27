package filter

import (
	"fmt"

	"github.com/samber/lo"
)

// Visitor processes a complete expression tree. Implementations own traversal
// order and any target-specific state so they can validate, evaluate, or
// compile the tree while returning the first error encountered. Callers should
// pass a predicate accepted by [Predicate.Validate]; [Parse] and vector-store request
// validation already enforce that boundary.
type Visitor interface {
	Visit(Predicate) error
}

func accept(predicate Predicate, visitor Visitor) error {
	if err := predicate.Validate(); err != nil {
		return err
	}
	if lo.IsNil(visitor) {
		return fmt.Errorf("filter.Predicate.Accept: visitor is nil")
	}
	return visitor.Visit(predicate)
}
