package filter

import (
	"fmt"
	"strings"
)

type parser struct {
	scanner *scanner
	current lexeme
}

func newParser(input string) (*parser, error) {
	if strings.TrimSpace(input) == "" {
		return nil, newSyntaxError(Position{Line: 1, Column: 1}, "", "expression is empty")
	}
	p := &parser{scanner: newScanner(input)}
	if err := p.advance(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *parser) parse() (Predicate, error) {
	expr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.current.kind != tokenEOF {
		return nil, p.unexpected("end of expression")
	}
	return expr, nil
}

func (p *parser) parseOr() (Predicate, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.current.kind == tokenOr {
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{
			left: left, operator: OpOr, right: right,
			start: left.Start(), end: right.End(),
		}
	}
	return left, nil
}

func (p *parser) parseAnd() (Predicate, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.current.kind == tokenAnd {
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{
			left: left, operator: OpAnd, right: right,
			start: left.Start(), end: right.End(),
		}
	}
	return left, nil
}

func (p *parser) parseNot() (Predicate, error) {
	if p.current.kind == tokenNot {
		op := p.current
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{
			operator: OpNot, right: right,
			start: op.start, end: right.End(),
		}, nil
	}

	if p.current.kind == tokenLeftParen {
		if err := p.advance(); err != nil {
			return nil, err
		}
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenRightParen, "closing ')'"); err != nil {
			return nil, err
		}
		return expr, nil
	}
	return p.parsePredicate()
}

func (p *parser) parsePredicate() (Predicate, error) {
	left, err := p.parseSelector()
	if err != nil {
		return nil, err
	}

	if p.current.kind == tokenNot {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenIn, "IN after NOT"); err != nil {
			return nil, err
		}
		list, err := p.parseList()
		if err != nil {
			return nil, err
		}
		membership := &BinaryExpr{
			left: left, operator: OpIn, right: list,
			start: left.Start(), end: list.End(),
		}
		return &UnaryExpr{
			operator: OpNot, right: membership,
			start: left.Start(), end: list.End(),
		}, nil
	}

	switch p.current.kind {
	case tokenEqual, tokenNotEqual, tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		op := operatorForToken(p.current.kind)
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseLiteral()
		if err != nil {
			return nil, err
		}
		return &BinaryExpr{
			left: left, operator: op, right: right,
			start: left.Start(), end: right.End(),
		}, nil

	case tokenIn:
		if err := p.advance(); err != nil {
			return nil, err
		}
		list, err := p.parseList()
		if err != nil {
			return nil, err
		}
		return &BinaryExpr{
			left: left, operator: OpIn, right: list,
			start: left.Start(), end: list.End(),
		}, nil

	case tokenHas:
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseLiteral()
		if err != nil {
			return nil, err
		}
		return &BinaryExpr{
			left: left, operator: OpHas, right: right,
			start: left.Start(), end: right.End(),
		}, nil

	case tokenLike:
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseLiteral()
		if err != nil {
			return nil, err
		}
		return &BinaryExpr{
			left: left, operator: OpLike, right: right,
			start: left.Start(), end: right.End(),
		}, nil

	case tokenIs:
		if err := p.advance(); err != nil {
			return nil, err
		}
		negated := p.current.kind == tokenNot
		if negated {
			if err := p.advance(); err != nil {
				return nil, err
			}
		}
		nullToken, err := p.expect(tokenNull, "NULL after IS")
		if err != nil {
			return nil, err
		}
		null := &Literal{
			kind: LiteralNull, text: "null",
			start: nullToken.start, end: nullToken.end,
		}
		test := &BinaryExpr{
			left: left, operator: OpIs, right: null,
			start: left.Start(), end: null.End(),
		}
		if !negated {
			return test, nil
		}
		return &UnaryExpr{
			operator: OpNot, right: test,
			start: left.Start(), end: null.End(),
		}, nil

	default:
		return nil, p.unexpected("comparison, IN, HAS, LIKE, or IS")
	}
}

func (p *parser) parseSelector() (Selector, error) {
	identToken, err := p.expect(tokenIdent, "identifier")
	if err != nil {
		return nil, err
	}

	var left Selector = &Ident{
		name:  identToken.literal,
		start: identToken.start, end: identToken.end,
	}
	for p.current.kind == tokenLeftBracket {
		if err := p.advance(); err != nil {
			return nil, err
		}
		index, err := p.parseLiteral()
		if err != nil {
			return nil, err
		}
		if !index.IsString() && !index.IsNumber() {
			return nil, newSyntaxError(index.Start(), index.text, "index must be a string or number")
		}
		closing, err := p.expect(tokenRightBracket, "closing ']'")
		if err != nil {
			return nil, err
		}
		left = &IndexExpr{
			left: left, index: index,
			start: left.Start(), end: closing.end,
		}
	}
	return left, nil
}

func (p *parser) parseList() (*ListLiteral, error) {
	opening, err := p.expect(tokenLeftParen, "opening '('")
	if err != nil {
		return nil, err
	}
	if p.current.kind == tokenRightParen {
		return nil, newSyntaxError(p.current.start, p.current.literal, "list cannot be empty")
	}

	first, err := p.parseLiteral()
	if err != nil {
		return nil, err
	}
	values := []*Literal{first}
	for p.current.kind == tokenComma {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.current.kind == tokenRightParen {
			return nil, newSyntaxError(p.current.start, p.current.literal, "trailing comma in list")
		}
		value, err := p.parseLiteral()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}

	closing, err := p.expect(tokenRightParen, "closing ')'")
	if err != nil {
		return nil, err
	}
	return &ListLiteral{
		values: values,
		start:  opening.start, end: closing.end,
	}, nil
}

func (p *parser) parseLiteral() (*Literal, error) {
	kind, ok := literalKindForToken(p.current.kind)
	if !ok {
		return nil, p.unexpected("string, number, or boolean literal")
	}
	current := p.current
	if err := p.advance(); err != nil {
		return nil, err
	}
	return &Literal{
		kind: kind, text: current.literal,
		start: current.start, end: current.end,
	}, nil
}

func (p *parser) expect(kind tokenKind, expected string) (lexeme, error) {
	if p.current.kind != kind {
		return lexeme{}, p.unexpected(expected)
	}
	current := p.current
	if err := p.advance(); err != nil {
		return lexeme{}, err
	}
	return current, nil
}

func (p *parser) advance() error {
	next, err := p.scanner.next()
	if err != nil {
		return err
	}
	p.current = next
	return nil
}

func (p *parser) unexpected(expected string) error {
	literal := p.current.literal
	if p.current.kind == tokenEOF {
		literal = tokenEOF.string()
	}
	return newSyntaxError(
		p.current.start,
		literal,
		fmt.Sprintf("expected %s, got %s", expected, literal),
	)
}

func operatorForToken(kind tokenKind) Operator {
	switch kind {
	case tokenEqual:
		return OpEqual
	case tokenNotEqual:
		return OpNotEqual
	case tokenLess:
		return OpLess
	case tokenLessEqual:
		return OpLessEqual
	case tokenGreater:
		return OpGreater
	case tokenGreaterEqual:
		return OpGreaterEqual
	default:
		return ""
	}
}

func literalKindForToken(kind tokenKind) (LiteralKind, bool) {
	switch kind {
	case tokenString:
		return LiteralString, true
	case tokenNumber:
		return LiteralNumber, true
	case tokenTrue, tokenFalse:
		return LiteralBool, true
	default:
		return "", false
	}
}
