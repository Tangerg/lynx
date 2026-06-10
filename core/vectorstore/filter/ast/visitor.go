package ast

// Visitor translates or validates an [Expr] tree. Visit receives the
// root expression and walks the tree itself, returning the first error
// encountered (semantic violation, unsupported shape, backend
// capability gap, ...) or nil when the whole tree was accepted.
//
// Every vector-store backend implements Visitor to compile the filter
// AST into its native query dialect; accumulated output (SQL text,
// filter structs, ...) is exposed through backend-specific accessors
// such as Result().
type Visitor interface {
	// Visit processes the whole tree rooted at expr.
	Visit(expr Expr) error
}
