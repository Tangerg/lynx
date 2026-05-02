package ast

// Visitor walks an [Expr] tree, optionally swapping visitors as it
// descends. The contract is identical to [go/ast.Visitor]:
//
//   - return the same Visitor: keep walking with the current visitor;
//   - return a different Visitor: switch to it for the subtree;
//   - return nil: stop descending into the current subtree.
type Visitor interface {
	// Visit is called once per AST node.
	Visit(expr Expr) Visitor
}
