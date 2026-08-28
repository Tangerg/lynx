package filter

import (
	"errors"

	"github.com/samber/lo"
)

// Visitor processes a complete expression tree. Implementations own traversal
// order and any target-specific state so they can validate, evaluate, or
// compile the tree while returning the first error encountered. Callers should
// pass a predicate accepted by [Predicate.Validate]; [Parse] and vector-store request
// validation already enforce that boundary.
type Visitor interface {
	// Visit consumes one complete, already validated predicate. Implementations
	// own traversal and may stop at the first target-specific error; they must not
	// mutate the immutable expression tree.
	Visit(predicate Predicate) error
}

func accept(predicate Predicate, visitor Visitor) error {
	if err := predicate.Validate(); err != nil {
		return err
	}
	if lo.IsNil(visitor) {
		return errors.New("filter: accept visitor: visitor is nil")
	}
	return visitor.Visit(predicate)
}
