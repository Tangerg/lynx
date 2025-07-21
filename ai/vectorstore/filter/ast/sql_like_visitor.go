package ast

import (
	"errors"
	"strings"
)

type SQLLikeVisitor struct {
	err    error
	buffer strings.Builder
}

func NewSQLLikeVisitor() *SQLLikeVisitor {
	return &SQLLikeVisitor{}
}

func (s *SQLLikeVisitor) Reset() {
	s.err = nil
	s.buffer.Reset()
}

func (s *SQLLikeVisitor) Error() error {
	return s.err
}

func (s *SQLLikeVisitor) SQL() string {
	return s.buffer.String()
}

func (s *SQLLikeVisitor) Visit(expr Expr) Visitor {
	s.visit(expr)
	return nil
}

func (s *SQLLikeVisitor) visit(expr Expr) {
	if s.err != nil {
		return
	}

	if expr == nil {
		s.err = errors.New("expression is nil")
		return
	}

	switch exprItem := expr.(type) {
	case *UnaryExpr:
		s.visitUnaryExpr(exprItem)
	case *BinaryExpr:
		s.visitBinaryExpr(exprItem)
	case *ParenExpr:
		s.visitParenExpr(exprItem)
	case *BrackExpr:
		s.visitBrackExpr(exprItem)
	case *Ident:
		s.visitIdent(exprItem)
	case *Literal:
		s.visitLiteral(exprItem)
	case *ListLiteral:
		s.visitListLiteral(exprItem)
	}
}

func (s *SQLLikeVisitor) visitUnaryExpr(expr *UnaryExpr) {
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

func (s *SQLLikeVisitor) visitBinaryExpr(expr *BinaryExpr) {

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

func (s *SQLLikeVisitor) visitParenExpr(expr *ParenExpr) {
	s.buffer.WriteString(expr.Lparen.Literal)
	s.visit(expr.Inner)
	s.buffer.WriteString(expr.Rparen.Literal)
}

func (s *SQLLikeVisitor) visitBrackExpr(expr *BrackExpr) {
	s.visit(expr.Left)
	s.buffer.WriteString(expr.LBrack.Literal)
	s.visit(expr.Literal)
	s.buffer.WriteString(expr.RBrack.Literal)
}

func (s *SQLLikeVisitor) visitIdent(expr *Ident) {
	s.buffer.WriteString(expr.Value)
}

func (s *SQLLikeVisitor) visitLiteral(expr *Literal) {
	if expr.IsString() {
		s.buffer.WriteString("'")
		s.buffer.WriteString(expr.Value)
		s.buffer.WriteString("'")
	} else {
		s.buffer.WriteString(expr.Value)
	}
}

func (s *SQLLikeVisitor) visitListLiteral(expr *ListLiteral) {
	s.buffer.WriteString(expr.Lparen.Literal)
	for i, value := range expr.Values {
		if i > 0 {
			s.buffer.WriteString(",")
		}
		s.visit(value)
	}
	s.buffer.WriteString(expr.Rparen.Literal)
}
