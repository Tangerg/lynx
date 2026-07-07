package parser

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

func (p *Parser) parseExpr(precedence int) (ast.Expr, error) {
	if err := p.checkTokenError(); err != nil {
		return nil, err
	}

	prefix, ok := p.prefixHandlers[p.currentToken.Kind]
	if !ok {
		return nil, &ParseError{
			Message: fmt.Sprintf("unexpected token %q (expected an identifier, literal, or unary operator)",
				p.currentToken.Literal),
			Token: p.currentToken,
		}
	}

	left, err := prefix()
	if err != nil {
		return nil, fmt.Errorf("parser.parseExpr: prefix: %w", err)
	}

	for {
		if p.currentToken.Kind.Is(token.EOF) {
			break
		}
		if err = p.checkTokenError(); err != nil {
			return nil, err
		}
		if precedence >= p.currentToken.Kind.Precedence() {
			break
		}

		infix, found := p.infixHandlers[p.currentToken.Kind]
		if !found {
			break
		}

		left, err = infix(left)
		if err != nil {
			return nil, fmt.Errorf("parser.parseExpr: infix: %w", err)
		}
	}

	return left, nil
}
