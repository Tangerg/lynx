package parser

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

func (p *Parser) checkTokenError() error {
	if p.currentToken.Kind.Is(token.ERROR) {
		return &ParseError{
			Message: fmt.Sprintf("lexical error: %s", p.currentToken.Literal),
			Token:   p.currentToken,
		}
	}
	return nil
}

func (p *Parser) addPrefixHandler(kind token.Kind, fn func() (ast.Expr, error)) {
	p.prefixHandlers[kind] = fn
}

func (p *Parser) addInfixHandler(kind token.Kind, fn func(ast.Expr) (ast.Expr, error)) {
	p.infixHandlers[kind] = fn
}

func (p *Parser) consumeToken() error {
	p.currentToken = p.lexer.Scan()
	return p.checkTokenError()
}

func (p *Parser) expectKind(kind token.Kind) (token.Token, error) {
	if err := p.checkTokenError(); err != nil {
		return p.currentToken, err
	}

	if !p.currentToken.Kind.Is(kind) {
		return p.currentToken, &ParseError{
			Message: fmt.Sprintf("expected %q but found %q",
				kind.Literal(), p.currentToken.Kind.Literal()),
			Token: p.currentToken,
		}
	}

	consumed := p.currentToken
	if err := p.consumeToken(); err != nil {
		return consumed, err
	}
	return consumed, nil
}
