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
	// Message is a human-readable description of the failure.
	Message string

	// Token is the token at which parsing stopped.
	Token token.Token
}

// Error formats a [ParseError] including the token text and source
// position.
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

	// Atoms — anything that can start an expression.
	p.addPrefixHandler(token.IDENT, p.parseIdent)
	p.addPrefixHandler(token.NUMBER, p.parseLiteral)
	p.addPrefixHandler(token.STRING, p.parseLiteral)
	p.addPrefixHandler(token.TRUE, p.parseLiteral)
	p.addPrefixHandler(token.FALSE, p.parseLiteral)

	// Unary (prefix) — and NOT also sits in infix position for the
	// `<field> NOT IN (...)` form, reusing the IN handler + NOT wrapper.
	p.addPrefixHandler(token.NOT, p.parseUnaryExpr)
	p.addInfixHandler(token.NOT, p.parseNotInfix)

	// Binary.
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

	// Grouping / indexing.
	p.addPrefixHandler(token.LPAREN, p.parseParen)
	p.addInfixHandler(token.LBRACK, p.parseIndexExpr)

	return p, nil
}

// checkTokenError surfaces a lexer ERROR token as a [ParseError].
// Other token kinds are passed through unchanged.
func (p *Parser) checkTokenError() error {
	if p.currentToken.Kind.Is(token.ERROR) {
		return &ParseError{
			Message: fmt.Sprintf("lexical error: %s", p.currentToken.Literal),
			Token:   p.currentToken,
		}
	}
	return nil
}

// addPrefixHandler registers a handler for tokens that can start an
// expression.
func (p *Parser) addPrefixHandler(kind token.Kind, fn func() (ast.Expr, error)) {
	p.prefixHandlers[kind] = fn
}

// addInfixHandler registers a handler for tokens that can sit between
// two expressions.
func (p *Parser) addInfixHandler(kind token.Kind, fn func(ast.Expr) (ast.Expr, error)) {
	p.infixHandlers[kind] = fn
}

// consumeToken advances by one token. Lexer ERROR tokens surface as
// [ParseError].
func (p *Parser) consumeToken() error {
	p.currentToken = p.lexer.Scan()
	return p.checkTokenError()
}

// expectKind asserts the current token has the given kind, then
// advances. Returns the consumed token. Mismatch produces a
// [ParseError].
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

// Parse parses one complete expression and verifies the input is
// fully consumed.
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

// parseExpr is the Pratt-style precedence-climbing core. It picks a
// prefix handler for the current token, then loops over infix
// handlers as long as their precedence beats the binding level
// passed in.
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

// parseIdent consumes an IDENT token and produces an [*ast.Ident].
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

// parseLiteral consumes a literal token (NUMBER / STRING / TRUE /
// FALSE) and produces an [*ast.Literal].
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

// parseParen handles a leading `(`. It picks between grouping (one
// expression then `)`) and a list literal (multiple comma-separated
// literals) based on what follows the first sub-expression.
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

// parseListLiteral builds an [*ast.ListLiteral]. The opening paren
// and the first element have already been consumed by [parseParen].
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

// parseUnaryExpr handles prefix operators (NOT today). The operand
// must be a computed expression — a bare identifier or literal can't
// be negated.
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

// parseBinaryExpr handles infix operators. The left operand is the
// expression produced so far; the right operand is parsed at this
// operator's precedence so right-side operators of the same level
// are left-associative.
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

// parseNotInfix handles `<field> NOT IN (...)` in infix position. It
// reuses the two existing tokens (NOT + IN) rather than introducing a
// dedicated NIN: the IN handler builds `<field> IN (...)` and the result
// is wrapped in the existing NOT [ast.UnaryExpr], so every backend's NOT
// + IN handling renders NOT IN for free. Only NOT IN is accepted here;
// any other token after an infix NOT is a parse error.
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

// parseIsExpr handles the postfix null test `<field> IS NULL` and
// `<field> IS NOT NULL`. It deliberately reuses existing AST shapes:
// the test is an [ast.BinaryExpr] with the IS operator and a NULL
// literal on the right, and the negated form wraps that in the existing
// NOT [ast.UnaryExpr] — so every backend's NOT handling renders
// `NOT (x IS NULL)` as `x IS NOT NULL` without a dedicated node.
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

// parseIndexExpr handles `expr[index]`. The index must be a
// non-boolean literal (number for arrays, string for objects).
// Left-associativity falls out naturally from the Pratt loop, so
// `a[0][1]` parses as `Index(Index(a,0),1)`.
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

// Parse is the convenience entry point: build a parser from input
// and return the resulting [ast.Expr]. Equivalent to
// [NewParser] followed by [Parser.Parse].
func Parse(input string) (ast.Expr, error) {
	p, err := NewParser(input)
	if err != nil {
		return nil, err
	}
	return p.Parse()
}
