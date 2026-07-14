package parser

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/token"
)

func (p *Parser) parseIdent() (ast.Expr, error) {
	tok, err := p.expectKind(token.IDENT)
	if err != nil {
		return nil, fmt.Errorf("parser.parseIdent: %w", err)
	}

	return &ast.Ident{
		Token: tok,
		Value: tok.Literal,
	}, nil
}

func (p *Parser) parseLiteral() (ast.Expr, error) {
	if err := p.checkTokenError(); err != nil {
		return nil, err
	}

	tok := p.currentToken
	if !tok.Kind.IsLiteral() {
		return nil, &ParseError{
			Message: fmt.Sprintf("expected a literal (number, string, or boolean) but found %q",
				p.currentToken.Literal),
			Token: p.currentToken,
		}
	}

	if err := p.consumeToken(); err != nil {
		return nil, fmt.Errorf("parser.parseLiteral: %w", err)
	}

	return &ast.Literal{
		Token: tok,
		Value: tok.Literal,
	}, nil
}

func (p *Parser) parseParen() (ast.Expr, error) {
	lparen, err := p.expectKind(token.LPAREN)
	if err != nil {
		return nil, fmt.Errorf("parser.parseParen: opening paren: %w", err)
	}

	if p.currentToken.Kind.Is(token.RPAREN) {
		return nil, &ParseError{
			Message: "empty parentheses are not allowed",
			Token:   p.currentToken,
		}
	}

	first, err := p.parseExpr(token.PrecedenceLowest)
	if err != nil {
		return nil, fmt.Errorf("parser.parseParen: inner expression: %w", err)
	}

	if p.currentToken.Kind.Is(token.COMMA) {
		return p.parseListLiteral(lparen, first)
	}

	if p.currentToken.Kind.Is(token.RPAREN) {
		if _, err = p.expectKind(token.RPAREN); err != nil {
			return nil, fmt.Errorf("parser.parseParen: closing paren: %w", err)
		}
		return first, nil
	}

	return nil, &ParseError{
		Message: fmt.Sprintf("expected ',' (list) or ')' (grouping) but found %q",
			p.currentToken.Literal),
		Token: p.currentToken,
	}
}

// The opening paren and the first element have already been consumed by
// [Parser.parseParen]. Elements must be literals of one shared kind; trailing
// commas are rejected.
func (p *Parser) parseListLiteral(lparen token.Token, firstExpr ast.Expr) (ast.Expr, error) {
	first, ok := firstExpr.(*ast.Literal)
	if !ok {
		return nil, &ParseError{
			Message: "list elements must be literals (number, string, or boolean)",
			Token:   lparen,
		}
	}

	values := []*ast.Literal{first}

	for p.currentToken.Kind.Is(token.COMMA) {
		if _, err := p.expectKind(token.COMMA); err != nil {
			return nil, fmt.Errorf("parser.parseListLiteral: comma: %w", err)
		}

		if p.currentToken.Kind.Is(token.RPAREN) {
			return nil, &ParseError{
				Message: "trailing comma in list is not allowed",
				Token:   p.currentToken,
			}
		}

		next, err := p.parseLiteral()
		if err != nil {
			return nil, fmt.Errorf("parser.parseListLiteral: element: %w", err)
		}

		lit := next.(*ast.Literal)
		if !first.IsSameKind(lit) {
			return nil, &ParseError{
				Message: fmt.Sprintf("list type mismatch: first element is %s, found %s",
					first.Token.Kind.Name(), lit.Token.Kind.Name()),
				Token: lit.Token,
			}
		}

		values = append(values, lit)
	}

	rparen, err := p.expectKind(token.RPAREN)
	if err != nil {
		return nil, fmt.Errorf("parser.parseListLiteral: closing paren: %w", err)
	}

	return &ast.ListLiteral{
		Lparen: lparen,
		Rparen: rparen,
		Values: values,
	}, nil
}
