package ast

// Visitor defines the visitor pattern interface for AST node operations.
// Allows performing operations like validation, transformation, or analysis
// without modifying the node structures.
type Visitor interface {
	// Visit is called for each AST node during traversal.
	// Returns:
	//   - same visitor: continue with current visitor
	//   - different visitor: continue with new visitor
	//   - nil: stop traversal of current subtree
	Visit(expr Expr) Visitor
}
