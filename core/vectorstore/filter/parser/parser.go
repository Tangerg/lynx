package parser

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/lexer"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
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

// The opening paren and the first element have already been consumed
// by [parseParen].
// Elements must be literals of one shared kind; trailing commas are
// rejected.
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

func Parse(input string) (ast.Expr, error) {
	p, err := NewParser(input)
	if err != nil {
		return nil, err
	}
	return p.Parse()
}
