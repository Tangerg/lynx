package visitors

import (
	"errors"
	"strings"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
)

type SQLLikeVisitor struct {
	err    error
	buffer strings.Builder
}

func NewSQLLikeVisitor() *SQLLikeVisitor {
	return &SQLLikeVisitor{}
}

func (s *SQLLikeVisitor) Error() error {
	return s.err
}

func (s *SQLLikeVisitor) SQL() string {
	return s.buffer.String()
}

func (s *SQLLikeVisitor) Visit(expr ast.Expr) ast.Visitor {
	s.visit(expr)
	return nil
}

func (s *SQLLikeVisitor) visit(expr ast.Expr) {
	if s.err != nil {
		return
	}

	if expr == nil {
		s.err = errors.New("expression is nil")
		return
	}

	switch exprItem := expr.(type) {
	case *ast.UnaryExpr:
		s.visitUnaryExpr(exprItem)
	case *ast.BinaryExpr:
		s.visitBinaryExpr(exprItem)
	case *ast.IndexExpr:
		s.visitIndexExpr(exprItem)
	case *ast.Ident:
		s.visitIdent(exprItem)
	case *ast.Literal:
		s.visitLiteral(exprItem)
	case *ast.ListLiteral:
		s.visitListLiteral(exprItem)
	}
}

func (s *SQLLikeVisitor) visitUnaryExpr(expr *ast.UnaryExpr) {
	s.buffer.WriteString(expr.Op.Literal)
	s.buffer.WriteString(" ")

	// always add '(' and ')'
	s.buffer.WriteString("(")
	s.visit(expr.Right)
	s.buffer.WriteString(")")

}

func (s *SQLLikeVisitor) visitBinaryExpr(expr *ast.BinaryExpr) {

	isLeftLower := expr.IsLeftLower()
	if isLeftLower {
		s.buffer.WriteString("(")
	}
	s.visit(expr.Left)
	if isLeftLower {
		s.buffer.WriteString(")")
	}

	s.buffer.WriteString(" ")
	s.buffer.WriteString(expr.Op.Literal)
	s.buffer.WriteString(" ")

	isRightLower := expr.IsRightLower()
	if isRightLower {
		s.buffer.WriteString("(")
	}
	s.visit(expr.Right)
	if isRightLower {
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

func (s *SQLLikeVisitor) visitLiteral(expr *ast.Literal) {
	if expr.IsString() {
		s.buffer.WriteString("'")
		s.buffer.WriteString(expr.Value)
		s.buffer.WriteString("'")
	} else {
		s.buffer.WriteString(expr.Value)
	}
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
