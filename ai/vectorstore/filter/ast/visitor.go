package ast

// Visitor defines the interface for implementing the visitor pattern on AST nodes.
// The visitor pattern allows you to perform operations on AST nodes without modifying
// the node structures themselves. This is useful for tasks like validation, transformation,
// code generation, or analysis of filter expressions.
type Visitor interface {
	// Visit is called for each AST node during traversal.
	// It receives the current expression node and returns a Visitor for continuing traversal:
	//   - Return the same visitor to continue traversal with the same visitor
	//   - Return a different visitor to continue traversal with a new visitor
	//   - Return nil to stop traversal of the current subtree
	// Parameters:
	//   - expr: the current AST node being visited
	// Returns:
	//   - a Visitor for continuing traversal, or nil to stop traversal
	Visit(expr Expr) Visitor
}

// Walk performs a depth-first traversal of the AST starting from the given expression.
// It applies the visitor pattern by calling the visitor's Visit method on each node.
// The traversal order follows the logical structure of expressions:
//   - For unary expressions: visits the operator, then the operand
//   - For binary expressions: visits the operator, then left operand, then right operand
//   - For parenthesized expressions: visits the parentheses, then the inner expression
//   - For bracket expressions: visits the left expression, then the index/key literal
//   - For list literals: visits each literal value in order
//   - For identifiers and literals: visits the node itself (leaf nodes)
//
// Parameters:
//   - v: the visitor to apply to each AST node
//   - expr: the root expression node to start traversal from
func Walk(v Visitor, expr Expr) {
	v = v.Visit(expr)
	if v == nil {
		return
	}

	switch exprItem := expr.(type) {
	case *UnaryExpr:
		Walk(v, exprItem.Right)
	case *BinaryExpr:
		Walk(v, exprItem.Left)
		Walk(v, exprItem.Right)
	case *ParenExpr:
		Walk(v, exprItem.Inner)
	case *BrackExpr:
		Walk(v, exprItem.Left)
		Walk(v, exprItem.Literal)
	case *Ident:
		Walk(v, exprItem)
	case *Literal:
		Walk(v, exprItem)
	case *ListLiteral:
		for _, literal := range exprItem.Values {
			Walk(v, literal)
		}
	}
}
