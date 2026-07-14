package parser

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/token"
)

func (p *Parser) parseUnaryExpr() (ast.Expr, error) {
	op := p.currentToken
	if !op.Kind.IsUnaryOperator() {
		return nil, &ParseError{
			Message: fmt.Sprintf("%q is not a valid unary operator", op.Literal),
			Token:   op,
		}
	}

	if err := p.consumeToken(); err != nil {
		return nil, fmt.Errorf("parser.parseUnaryExpr: advance past %q: %w", op.Literal, err)
	}

	right, err := p.parseExpr(op.Kind.Precedence())
	if err != nil {
		return nil, fmt.Errorf("parser.parseUnaryExpr: operand: %w", err)
	}

	computed, ok := right.(ast.ComputedExpr)
	if !ok {
		return nil, &ParseError{
			Message: fmt.Sprintf("unary %q cannot apply to a non-computed expression",
				op.Literal),
			Token: op,
		}
	}

	return &ast.UnaryExpr{
		Op:    op,
		Right: computed,
	}, nil
}

func (p *Parser) parseBinaryExpr(left ast.Expr) (ast.Expr, error) {
	op := p.currentToken
	if !op.Kind.IsBinaryOperator() {
		return nil, &ParseError{
			Message: fmt.Sprintf("%q is not a valid binary operator", op.Literal),
			Token:   op,
		}
	}

	if err := p.consumeToken(); err != nil {
		return nil, fmt.Errorf("parser.parseBinaryExpr: advance past %q: %w", op.Literal, err)
	}

	right, err := p.parseExpr(op.Kind.Precedence())
	if err != nil {
		return nil, fmt.Errorf("parser.parseBinaryExpr: right operand for %q: %w", op.Literal, err)
	}

	return &ast.BinaryExpr{
		Left:  left,
		Op:    op,
		Right: right,
	}, nil
}

func (p *Parser) parseNotInfix(left ast.Expr) (ast.Expr, error) {
	notTok, err := p.expectKind(token.NOT)
	if err != nil {
		return nil, fmt.Errorf("parser.parseNotInfix: %w", err)
	}

	if !p.currentToken.Kind.Is(token.IN) {
		return nil, &ParseError{
			Message: fmt.Sprintf("expected IN after NOT but found %q", p.currentToken.Kind.Literal()),
			Token:   p.currentToken,
		}
	}

	// currentToken is IN — let the binary handler consume `IN (...)`.
	inExpr, err := p.parseBinaryExpr(left)
	if err != nil {
		return nil, fmt.Errorf("parser.parseNotInfix: %w", err)
	}

	computed, ok := inExpr.(ast.ComputedExpr)
	if !ok {
		return nil, &ParseError{
			Message: "NOT IN operand must be a computed expression",
			Token:   notTok,
		}
	}
	return &ast.UnaryExpr{Op: notTok, Right: computed}, nil
}

func (p *Parser) parseIsExpr(left ast.Expr) (ast.Expr, error) {
	isTok, err := p.expectKind(token.IS)
	if err != nil {
		return nil, fmt.Errorf("parser.parseIsExpr: %w", err)
	}

	var notTok token.Token
	negated := false
	if p.currentToken.Kind.Is(token.NOT) {
		notTok, err = p.expectKind(token.NOT)
		if err != nil {
			return nil, fmt.Errorf("parser.parseIsExpr: %w", err)
		}
		negated = true
	}

	nullTok, err := p.expectKind(token.NULL)
	if err != nil {
		return nil, fmt.Errorf("parser.parseIsExpr: expected NULL after IS: %w", err)
	}

	test := &ast.BinaryExpr{
		Left:  left,
		Op:    isTok,
		Right: &ast.Literal{Token: nullTok, Value: nullTok.Literal},
	}
	if !negated {
		return test, nil
	}
	return &ast.UnaryExpr{Op: notTok, Right: test}, nil
}

func (p *Parser) parseIndexExpr(left ast.Expr) (ast.Expr, error) {
	lbrack, err := p.expectKind(token.LBRACK)
	if err != nil {
		return nil, fmt.Errorf("parser.parseIndexExpr: opening bracket: %w", err)
	}

	indexExpr, err := p.parseLiteral()
	if err != nil {
		return nil, fmt.Errorf("parser.parseIndexExpr: index value: %w", err)
	}

	index := indexExpr.(*ast.Literal)
	if index.IsBool() {
		return nil, &ParseError{
			Message: "index must be a number or string, not a boolean",
			Token:   index.Token,
		}
	}

	rbrack, err := p.expectKind(token.RBRACK)
	if err != nil {
		return nil, fmt.Errorf("parser.parseIndexExpr: closing bracket: %w", err)
	}

	return &ast.IndexExpr{
		LBrack: lbrack,
		RBrack: rbrack,
		Left:   left,
		Index:  index,
	}, nil
}
