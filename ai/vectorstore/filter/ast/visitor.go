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

// Walk performs depth-first traversal of the AST using the visitor pattern.
// Traversal order:
//   - UnaryExpr: operator → operand
//   - BinaryExpr: operator → left → right
//   - IndexExpr: left expression → index literal
//   - ListLiteral: each literal in order
//   - Leaf nodes: the node itself
func Walk(v Visitor, expr Expr) {
	v = v.Visit(expr)
	if v == nil {
		return
	}

	switch typedExpr := expr.(type) {
	case *UnaryExpr:
		Walk(v, typedExpr.Right)
	case *BinaryExpr:
		Walk(v, typedExpr.Left)
		Walk(v, typedExpr.Right)
	case *IndexExpr:
		Walk(v, typedExpr.Left)
		Walk(v, typedExpr.Index)
	case *Ident:
		Walk(v, typedExpr)
	case *Literal:
		Walk(v, typedExpr)
	case *ListLiteral:
		for _, literal := range typedExpr.Values {
			Walk(v, literal)
		}
	}
}
