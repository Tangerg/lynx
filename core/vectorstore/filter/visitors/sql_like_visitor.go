package visitors

import (
	"errors"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
)

// SQLLikeVisitor renders an [ast.Expr] back into SQL-flavored text.
// Useful for logging the parsed filter, generating debug output, or
// adapting the filter language to a SQL-shaped backend.
//
// Example:
//
//	expr, _ := filter.Parse(`category == 'tech' AND year >= 2020`)
//	v := visitors.NewSQLLikeVisitor()
//	v.Visit(expr)
//	if err := v.Error(); err != nil {
//	    return err
//	}
//	fmt.Println(v.SQL())
type SQLLikeVisitor struct {
	err    error
	buffer strings.Builder
}

// NewSQLLikeVisitor returns an empty [SQLLikeVisitor] ready to walk an
// AST.
func NewSQLLikeVisitor() *SQLLikeVisitor { return &SQLLikeVisitor{} }

// Error returns the first error encountered during traversal, or nil.
func (s *SQLLikeVisitor) Error() error { return s.err }

// SQL returns the rendered output. Call it after [SQLLikeVisitor.Visit]
// finishes the traversal.
func (s *SQLLikeVisitor) SQL() string { return s.buffer.String() }

// Visit dispatches expr to the matching internal handler and stops
// further descent — the visitor walks the tree itself rather than
// returning a sub-visitor.
func (s *SQLLikeVisitor) Visit(expr ast.Expr) ast.Visitor {
	s.visit(expr)
	return nil
}

// visit is the internal dispatch used recursively by handlers. Halts on
// the first error and treats nil expressions as a programmer mistake.
func (s *SQLLikeVisitor) visit(expr ast.Expr) {
	if s.err != nil {
		return
	}
	if expr == nil {
		s.err = errors.New("visitors.SQLLikeVisitor: expression is nil")
		return
	}

	switch node := expr.(type) {
	case *ast.UnaryExpr:
		s.visitUnaryExpr(node)
	case *ast.BinaryExpr:
		s.visitBinaryExpr(node)
	case *ast.IndexExpr:
		s.visitIndexExpr(node)
	case *ast.Ident:
		s.visitIdent(node)
	case *ast.Literal:
		s.visitLiteral(node)
	case *ast.ListLiteral:
		s.visitListLiteral(node)
	}
}

// visitUnaryExpr renders "<op> (<operand>)" — the operand is always
// parenthesized so precedence ambiguities never sneak in.
func (s *SQLLikeVisitor) visitUnaryExpr(expr *ast.UnaryExpr) {
	s.buffer.WriteString(expr.Op.Literal)
	s.buffer.WriteString(" (")
	s.visit(expr.Right)
	s.buffer.WriteString(")")
}

// visitBinaryExpr renders "left op right", parenthesizing each operand
// only when its precedence is lower than the parent — produces clean
// output without redundant parens.
func (s *SQLLikeVisitor) visitBinaryExpr(expr *ast.BinaryExpr) {
	leftNeedsParens := expr.IsLeftLower()
	if leftNeedsParens {
		s.buffer.WriteString("(")
	}
	s.visit(expr.Left)
	if leftNeedsParens {
		s.buffer.WriteString(")")
	}

	s.buffer.WriteString(" ")
	s.buffer.WriteString(expr.Op.Literal)
	s.buffer.WriteString(" ")

	rightNeedsParens := expr.IsRightLower()
	if rightNeedsParens {
		s.buffer.WriteString("(")
	}
	s.visit(expr.Right)
	if rightNeedsParens {
		s.buffer.WriteString(")")
	}
}

// visitIndexExpr renders "left[index]".
func (s *SQLLikeVisitor) visitIndexExpr(expr *ast.IndexExpr) {
	s.visit(expr.Left)
	s.buffer.WriteString(expr.LBrack.Literal)
	s.visit(expr.Index)
	s.buffer.WriteString(expr.RBrack.Literal)
}

// visitIdent renders an identifier verbatim.
func (s *SQLLikeVisitor) visitIdent(expr *ast.Ident) {
	s.buffer.WriteString(expr.Value)
}

// visitLiteral renders a literal — strings get single quotes, other
// kinds are emitted as-is.
func (s *SQLLikeVisitor) visitLiteral(expr *ast.Literal) {
	if expr.IsString() {
		s.buffer.WriteString("'")
		s.buffer.WriteString(expr.Value)
		s.buffer.WriteString("'")
		return
	}
	s.buffer.WriteString(expr.Value)
}

// visitListLiteral renders "(v1,v2,v3)" — comma-separated, no spaces
// (matches the most-restrictive SQL flavor that still parses).
func (s *SQLLikeVisitor) visitListLiteral(expr *ast.ListLiteral) {
	s.buffer.WriteString(expr.Lparen.Literal)
	for i, value := range expr.Values {
		if i > 0 {
			s.buffer.WriteString(",")
		}
		s.visit(value)
	}
	s.buffer.WriteString(expr.Rparen.Literal)
}
