package filter

// Visitor processes a complete expression tree. Implementations own traversal
// order and any target-specific state so they can validate, evaluate, or
// compile the tree while returning the first error encountered. Callers should
// pass a predicate accepted by [Validate]; [Parse] and vector-store request
// validation already enforce that boundary.
type Visitor interface {
	Visit(Predicate) error
}
