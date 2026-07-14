package parser

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/lexer"
	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/token"
)

// ParseError is returned when parsing fails. It carries the offending
// token so callers can produce position-aware diagnostics.
type ParseError struct {
	Message string
	Token   token.Token
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parser: %s (at %s, token %q)",
		e.Message, e.Token.Start.String(), e.Token.Literal)
}

// Parser is a Pratt parser over a [lexer.Lexer]. The prefix and infix
// handler maps form the operator table; precedence climbing drives
// the expression loop.
type Parser struct {
	lexer          *lexer.Lexer
	currentToken   token.Token
	prefixHandlers map[token.Kind]func() (ast.Expr, error)
	infixHandlers  map[token.Kind]func(ast.Expr) (ast.Expr, error)
}

// NewParser builds a [Parser] from either a source string (a fresh
// lexer is created) or an existing [*lexer.Lexer] (which is reset
// before use). The first token is consumed eagerly so the parser is
// ready to call [Parser.Parse].
func NewParser[I string | *lexer.Lexer](input I) (*Parser, error) {
	var (
		lex *lexer.Lexer
		err error
	)

	switch typed := any(input).(type) {
	case string:
		lex, err = lexer.NewLexer(typed)
		if err != nil {
			return nil, fmt.Errorf("parser.NewParser: build lexer: %w", err)
		}
	case *lexer.Lexer:
		lex = typed
		lex.Reset()
	}

	p := &Parser{
		lexer:          lex,
		prefixHandlers: make(map[token.Kind]func() (ast.Expr, error)),
		infixHandlers:  make(map[token.Kind]func(ast.Expr) (ast.Expr, error)),
	}

	p.currentToken = lex.Scan()
	if err = p.checkTokenError(); err != nil {
		return nil, err
	}

	p.addPrefixHandler(token.IDENT, p.parseIdent)
	p.addPrefixHandler(token.NUMBER, p.parseLiteral)
	p.addPrefixHandler(token.STRING, p.parseLiteral)
	p.addPrefixHandler(token.TRUE, p.parseLiteral)
	p.addPrefixHandler(token.FALSE, p.parseLiteral)

	// Unary (prefix) — and NOT also sits in infix position for the
	// `<field> NOT IN (...)` form, reusing the IN handler + NOT wrapper.
	p.addPrefixHandler(token.NOT, p.parseUnaryExpr)
	p.addInfixHandler(token.NOT, p.parseNotInfix)

	p.addInfixHandler(token.AND, p.parseBinaryExpr)
	p.addInfixHandler(token.OR, p.parseBinaryExpr)
	p.addInfixHandler(token.EQ, p.parseBinaryExpr)
	p.addInfixHandler(token.NE, p.parseBinaryExpr)
	p.addInfixHandler(token.LT, p.parseBinaryExpr)
	p.addInfixHandler(token.LE, p.parseBinaryExpr)
	p.addInfixHandler(token.GT, p.parseBinaryExpr)
	p.addInfixHandler(token.GE, p.parseBinaryExpr)
	p.addInfixHandler(token.IN, p.parseBinaryExpr)
	p.addInfixHandler(token.LIKE, p.parseBinaryExpr)
	p.addInfixHandler(token.IS, p.parseIsExpr)

	p.addPrefixHandler(token.LPAREN, p.parseParen)
	p.addInfixHandler(token.LBRACK, p.parseIndexExpr)

	return p, nil
}

func (p *Parser) Parse() (ast.Expr, error) {
	if err := p.checkTokenError(); err != nil {
		return nil, err
	}

	expr, err := p.parseExpr(token.PrecedenceLowest)
	if err != nil {
		return nil, fmt.Errorf("parser.Parse: %w", err)
	}

	if !p.currentToken.Kind.Is(token.EOF) {
		if err = p.checkTokenError(); err != nil {
			return nil, err
		}
		return nil, &ParseError{
			Message: fmt.Sprintf("unexpected trailing token %q after expression",
				p.currentToken.Literal),
			Token: p.currentToken,
		}
	}

	return expr, nil
}

func Parse(input string) (ast.Expr, error) {
	p, err := NewParser(input)
	if err != nil {
		return nil, err
	}
	return p.Parse()
}
