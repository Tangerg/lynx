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
//	if err := v.Visit(expr); err != nil {
//	    return err
//	}
//	fmt.Println(v.SQL())
type SQLLikeVisitor struct {
	err    error
	buffer strings.Builder
}

func NewSQLLikeVisitor() *SQLLikeVisitor { return &SQLLikeVisitor{} }

func (s *SQLLikeVisitor) SQL() string { return s.buffer.String() }

func (s *SQLLikeVisitor) Visit(expr ast.Expr) error {
	s.visit(expr)
	return s.err
}

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

func (s *SQLLikeVisitor) visitUnaryExpr(expr *ast.UnaryExpr) {
	s.buffer.WriteString(expr.Op.Literal)
	s.buffer.WriteString(" (")
	s.visit(expr.Right)
	s.buffer.WriteString(")")
}

type precedenced interface {
	Precedence() int
}

func needsParens(parent precedenced, child ast.Expr) bool {
	c, ok := child.(precedenced)
	if !ok {
		return false
	}
	return c.Precedence() < parent.Precedence()
}

func (s *SQLLikeVisitor) visitBinaryExpr(expr *ast.BinaryExpr) {
	leftWraps := needsParens(expr, expr.Left)
	if leftWraps {
		s.buffer.WriteString("(")
	}
	s.visit(expr.Left)
	if leftWraps {
		s.buffer.WriteString(")")
	}

	s.buffer.WriteString(" ")
	s.buffer.WriteString(expr.Op.Literal)
	s.buffer.WriteString(" ")

	rightWraps := needsParens(expr, expr.Right)
	if rightWraps {
		s.buffer.WriteString("(")
	}
	s.visit(expr.Right)
	if rightWraps {
		s.buffer.WriteString(")")
	}
}

func (s *SQLLikeVisitor) visitIndexExpr(expr *ast.IndexExpr) {
	s.visit(expr.Left)
	s.buffer.WriteString(expr.LBrack.Literal)
	s.visit(expr.Index)
	s.buffer.WriteString(expr.RBrack.Literal)
}

func (s *SQLLikeVisitor) visitIdent(expr *ast.Ident) {
	s.buffer.WriteString(expr.Value)
}

// stringEscaper re-applies the filter language's string escapes — the
// inverse of the lexer's resolveEscape — so a rendered literal
// round-trips through the parser and an embedded quote can't break
// out of the quoted form. Backslash comes first so the escapes the
// later pairs insert are never themselves re-escaped.
var stringEscaper = strings.NewReplacer(
	`\`, `\\`,
	`'`, `\'`,
	"\n", `\n`,
	"\t", `\t`,
	"\r", `\r`,
)

func (s *SQLLikeVisitor) visitLiteral(expr *ast.Literal) {
	if expr.IsString() {
		s.buffer.WriteString("'")
		s.buffer.WriteString(stringEscaper.Replace(expr.Value))
		s.buffer.WriteString("'")
		return
	}
	s.buffer.WriteString(expr.Value)
}

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
