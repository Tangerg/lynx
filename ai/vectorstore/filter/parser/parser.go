package parser

import (
	"fmt"

	"strconv"
)

// parseError represents a parsing error with position information
type parseError struct {
	message string
	token   Token
}

func (e *parseError) Error() string {
	return fmt.Sprintf("parse error: %s (token: %s)",
		e.message, e.token.Value())
}

// Parser represents the parser state
type Parser struct {
	lexer        *Lexer
	currentToken Token
}

// NewParser creates a new parser with the given tokens
func NewParser[I string | *Lexer](input I) (*Parser, error) {
	var (
		lexer    *Lexer
		err      error
		anyInput = any(input)
	)

	switch anyInput.(type) {
	case string:
		lexer, err = NewLexer(anyInput.(string))
		if err != nil {
			return nil, err
		}
	case *Lexer:
		lexer = anyInput.(*Lexer)
		lexer.Reset()
	}
	return &Parser{
		lexer:        lexer,
		currentToken: lexer.NextToken(),
	}, nil
}

// advance moves to the nextToken token
func (p *Parser) nextToken() {
	p.currentToken = p.lexer.NextToken()
}

// consume expects and consumes a token of the given kind
func (p *Parser) consume(kind TokenKind) (Token, error) {
	if p.currentToken.Kind().Is(kind) {
		token := p.currentToken
		p.nextToken()
		return token, nil
	}

	err := &parseError{
		message: fmt.Sprintf("expected %s, but got %s", kind.String(), p.currentToken.Kind().String()),
		token:   p.currentToken,
	}
	return p.currentToken, err
}

// Parse is the main entry point for parsing
func (p *Parser) Parse() (Node, error) {
	node, err := p.parseOrExpression()
	if err != nil {
		return nil, err
	}

	// Check for unexpected tokens at the end
	if !p.currentToken.Kind().Is(EOF) {
		return nil, &parseError{
			message: fmt.Sprintf("unexpected token after complete expression: %s", p.currentToken.Kind().String()),
			token:   p.currentToken,
		}
	}

	return node, nil
}

// parseOrExpression parses OR expressions (lowest precedence)
func (p *Parser) parseOrExpression() (Node, error) {
	var (
		left  Node
		right Node
		err   error
	)
	left, err = p.parseAndExpression()
	if err != nil {
		return nil, err
	}

	for p.currentToken.Kind().Is(OR) {
		_, err = p.consume(OR)
		if err != nil {
			return nil, err
		}

		right, err = p.parseAndExpression()
		if err != nil {
			return nil, err
		}

		left = &BinaryOpNode{
			left:  left,
			op:    OR,
			right: right,
		}
	}

	return left, nil
}

// parseAndExpression parses AND expressions (middle precedence)
func (p *Parser) parseAndExpression() (Node, error) {
	var (
		left  Node
		right Node
		err   error
	)

	left, err = p.parseNotExpression()
	if err != nil {
		return nil, err
	}

	for p.currentToken.Kind().Is(AND) {
		_, err = p.consume(AND)
		if err != nil {
			return nil, err
		}

		right, err = p.parseNotExpression()
		if err != nil {
			return nil, err
		}

		left = &BinaryOpNode{
			left:  left,
			op:    AND,
			right: right,
		}
	}

	return left, nil
}

// parseNotExpression parses NOT expressions (higher precedence)
func (p *Parser) parseNotExpression() (Node, error) {
	if p.currentToken.Kind().Is(NOT) {
		_, err := p.consume(NOT)
		if err != nil {
			return nil, err
		}

		operand, err := p.parsePrimaryExpression()
		if err != nil {
			return nil, err
		}

		return &UnaryOpNode{
			op:      NOT,
			operand: operand,
		}, nil
	}
	return p.parsePrimaryExpression()
}

// parsePrimaryExpression parses primary expressions (highest precedence)
func (p *Parser) parsePrimaryExpression() (Node, error) {
	switch p.currentToken.Kind() {
	case LPAREN:
		_, err := p.consume(LPAREN)
		if err != nil {
			return nil, err
		}

		expr, err := p.parseOrExpression()
		if err != nil {
			return nil, err
		}

		_, err = p.consume(RPAREN)
		if err != nil {
			return nil, err
		}

		return expr, nil

	case IDENTIFIER:
		return p.parseFieldExpression()

	default:
		return nil, &parseError{
			message: fmt.Sprintf("unexpected token: %s", p.currentToken.Kind().String()),
			token:   p.currentToken,
		}
	}
}

// parseFieldExpression parses field-related expressions
func (p *Parser) parseFieldExpression() (Node, error) {
	field, err := p.consume(IDENTIFIER)
	if err != nil {
		return nil, err
	}

	switch p.currentToken.Kind() {
	case EQ, NE, GT, GE, LT, LE:
		return p.parseComparison(field)
	case LIKE:
		return p.parseLikeExpression(field)
	case IN:
		return p.parseInExpression(field)
	default:
		return nil, &parseError{
			message: fmt.Sprintf("expected comparison operator, got %s", p.currentToken.Value()),
			token:   p.currentToken,
		}
	}
}

// parseComparison parses comparison expressions
func (p *Parser) parseComparison(left Token) (Node, error) {
	op := p.currentToken.Kind()

	p.nextToken()

	right, err := p.parseOperand()
	if err != nil {
		return nil, err
	}

	return &ComparisonNode{
		left:  &IdentifierNode{value: left.Value()},
		op:    op,
		right: right,
	}, nil
}

// parseLikeExpression parses LIKE expressions
func (p *Parser) parseLikeExpression(field Token) (Node, error) {
	_, err := p.consume(LIKE)
	if err != nil {
		return nil, err
	}

	pattern, err := p.parseOperand()
	if err != nil {
		return nil, err
	}

	return &LikeNode{
		field:   &IdentifierNode{value: field.Value()},
		pattern: pattern,
	}, nil
}

// parseInExpression parses IN expressions
func (p *Parser) parseInExpression(field Token) (Node, error) {
	_, err := p.consume(IN)
	if err != nil {
		return nil, err
	}

	_, err = p.consume(LPAREN)
	if err != nil {
		return nil, err
	}

	var values []Node

	// Handle empty list case
	if p.currentToken.Kind().Is(RPAREN) {
		_, err = p.consume(RPAREN)
		if err != nil {
			return nil, err
		}
		return &InNode{
			field:  &IdentifierNode{value: field.Value()},
			values: values,
		}, nil
	}

	// Parse first operand
	operand, err := p.parseOperand()
	if err != nil {
		return nil, err
	}
	values = append(values, operand)

	// Parse remaining operands
	for p.currentToken.Kind().Is(COMMA) {
		_, err = p.consume(COMMA)
		if err != nil {
			return nil, err
		}

		operand, err = p.parseOperand()
		if err != nil {
			return nil, err
		}
		values = append(values, operand)
	}

	_, err = p.consume(RPAREN)
	if err != nil {
		return nil, err
	}

	return &InNode{
		field:  &IdentifierNode{value: field.Value()},
		values: values,
	}, nil
}

// parseOperand parses operands (literals and identifiers)
func (p *Parser) parseOperand() (Node, error) {
	token := p.currentToken

	if token.Kind().Is(ERROR) {
		return nil, &parseError{
			message: token.Value(),
			token:   p.currentToken,
		}
	}

	if token.Kind().Is(EOF) {
		return nil, &parseError{
			message: "unexpected end of input",
			token:   token,
		}
	}

	p.nextToken()

	switch token.Kind() {
	case IDENTIFIER:
		return &IdentifierNode{value: token.Value()}, nil

	case STRING:
		return &StringNode{value: token.Value()}, nil

	case NUMBER:
		// Convert string to float64
		value, err := strconv.ParseFloat(token.Value(), 64)
		if err != nil {
			return nil, &parseError{
				message: fmt.Sprintf("invalid number format: %s", token.Value()),
				token:   token,
			}
		}
		return &NumberNode{value: value}, nil

	case BOOLEAN:
		// Convert string to bool
		value, err := strconv.ParseBool(token.Value())
		if err != nil {
			return nil, &parseError{
				message: fmt.Sprintf("invalid boolean format: %s", token.Value()),
				token:   token,
			}
		}
		return &BooleanNode{value: value}, nil

	default:
		return nil, &parseError{
			message: fmt.Sprintf("unexpected operand: %s", token.Value()),
			token:   token,
		}
	}
}
