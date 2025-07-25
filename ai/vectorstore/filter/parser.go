package filter

import (
	"fmt"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// ParseError represents a parsing error with currentPosition information.
// Provides human-readable error messages for debugging and user feedback.
type ParseError struct {
	Message string      // Human-readable error description
	Token   token.Token // The problematic Token that caused the error
}

// Error implements the error interface with comprehensive error formatting.
// Includes error description and currentPosition information.
func (e *ParseError) Error() string {
	return fmt.Sprintf(
		"Parsing failed: %s (at currentPosition %s, Token: '%s')",
		e.Message,
		e.Token.Start.String(),
		e.Token.Literal,
	)
}

// Parser represents a recursive descent parser with Pratt parsing capabilities.
// Uses the Pratt algorithm for efficient operator precedence handling.
type Parser struct {
	lexer          *Lexer                                          // Token provider from input
	currentToken   token.Token                                     // Current Token being processed
	prefixHandlers map[token.Kind]func() (ast.Expr, error)         // Handles expression-starting tokens
	infixHandlers  map[token.Kind]func(ast.Expr) (ast.Expr, error) // Handles binary operators
}

// NewParser creates a new parser with proper initialization.
// Accepts either string input or existing *Lexer for flexibility.
func NewParser[I string | *Lexer](input I) (*Parser, error) {
	var (
		lexer *Lexer
		err   error
	)

	switch typedInput := any(input).(type) {
	case string:
		lexer, err = NewLexer(typedInput)
		if err != nil {
			return nil, fmt.Errorf("failed to create lexer from input string: %w", err)
		}
	case *Lexer:
		lexer = typedInput
		lexer.Reset()
	}

	parser := &Parser{
		lexer:          lexer,
		currentToken:   lexer.Scan(),
		prefixHandlers: make(map[token.Kind]func() (ast.Expr, error)),
		infixHandlers:  make(map[token.Kind]func(ast.Expr) (ast.Expr, error)),
	}

	// Register prefix parse functions for expression starters
	parser.addPrefixHandler(token.IDENT, parser.parseIdent)
	parser.addPrefixHandler(token.NUMBER, parser.parseLiteral)
	parser.addPrefixHandler(token.STRING, parser.parseLiteral)
	parser.addPrefixHandler(token.TRUE, parser.parseLiteral)
	parser.addPrefixHandler(token.FALSE, parser.parseLiteral)

	// Register unary operators
	parser.addPrefixHandler(token.NOT, parser.parseUnaryExpr)

	// Register binary operators
	parser.addInfixHandler(token.AND, parser.parseBinaryExpr)
	parser.addInfixHandler(token.OR, parser.parseBinaryExpr)
	parser.addInfixHandler(token.EQ, parser.parseBinaryExpr)
	parser.addInfixHandler(token.NE, parser.parseBinaryExpr)
	parser.addInfixHandler(token.LT, parser.parseBinaryExpr)
	parser.addInfixHandler(token.LE, parser.parseBinaryExpr)
	parser.addInfixHandler(token.GT, parser.parseBinaryExpr)
	parser.addInfixHandler(token.GE, parser.parseBinaryExpr)
	parser.addInfixHandler(token.IN, parser.parseBinaryExpr)
	parser.addInfixHandler(token.LIKE, parser.parseBinaryExpr)

	// Register grouping and indexing
	parser.addPrefixHandler(token.LPAREN, parser.parseParen)
	parser.addInfixHandler(token.LBRACK, parser.parseIndexExpr)

	return parser, nil
}

// addPrefixHandler associates a prefix parsing function with a Token kind.
// Handles tokens that can appear at the beginning of expressions.
func (p *Parser) addPrefixHandler(kind token.Kind, fn func() (ast.Expr, error)) {
	p.prefixHandlers[kind] = fn
}

// addInfixHandler associates an infix parsing function with a Token kind.
// Handles binary operators that appear between expressions.
func (p *Parser) addInfixHandler(kind token.Kind, fn func(ast.Expr) (ast.Expr, error)) {
	p.infixHandlers[kind] = fn
}

// consumeToken advances the parser to the next Token in input stream.
// Primary mechanism for consuming tokens during parsing.
func (p *Parser) consumeToken() {
	p.currentToken = p.lexer.Scan()
}

// consumeExpectedKindToken verifies current Token matches expected kind and advances.
// Used for mandatory tokens that must appear in specific positions.
func (p *Parser) consumeExpectedKindToken(expectedKind token.Kind) (token.Token, error) {
	if p.currentToken.Kind.Is(expectedKind) {
		consumedToken := p.currentToken
		p.consumeToken()
		return consumedToken, nil
	}

	err := &ParseError{
		Message: fmt.Sprintf(
			"Expected '%s' but found '%s'. Check your syntax near this currentPosition",
			expectedKind.Literal(),
			p.currentToken.Kind.Literal(),
		),
		Token: p.currentToken,
	}
	return p.currentToken, err
}

// Parse is the main entry point for parsing complete expressions.
// Parses full expression and ensures no unexpected tokens remain.
func (p *Parser) Parse() (ast.Expr, error) {
	expr, err := p.parseExpr(token.PrecedenceLowest)
	if err != nil {
		return nil, fmt.Errorf("expression parsing failed: %w", err)
	}

	// Ensure entire input is consumed
	if !p.currentToken.Kind.Is(token.EOF) {
		return nil, &ParseError{
			Message: fmt.Sprintf(
				"Unexpected Token '%s' found after complete expression. Expected end of input",
				p.currentToken.Literal,
			),
			Token: p.currentToken,
		}
	}

	return expr, nil
}

// parseExpr implements core Pratt parser with precedence climbing.
// Handles operator precedence automatically through recursive parsing.
func (p *Parser) parseExpr(precedence int) (ast.Expr, error) {
	// Get prefix parsing function for current Token
	prefixHandler, exists := p.prefixHandlers[p.currentToken.Kind]
	if !exists {
		return nil, &ParseError{
			Message: fmt.Sprintf(
				"Unexpected Token '%s'. Expected an identifier, number, string, boolean, or unary operator",
				p.currentToken.Literal,
			),
			Token: p.currentToken,
		}
	}

	// Parse left side using prefix function
	leftExpr, err := prefixHandler()
	if err != nil {
		return nil, fmt.Errorf("failed to parse prefix expression: %w", err)
	}

	// Continue parsing infix expressions while precedence allows
	for {
		if p.currentToken.Kind.Is(token.EOF) {
			break
		}

		// Stop if current precedence is higher or equal
		if precedence >= p.currentToken.Kind.Precedence() {
			break
		}

		infixHandler, exists := p.infixHandlers[p.currentToken.Kind]
		if !exists {
			break
		}

		leftExpr, err = infixHandler(leftExpr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse infix expression: %w", err)
		}
	}

	return leftExpr, nil
}

// parseIdent parses identifier tokens for variable/field names.
// Creates Ident AST node with identifier information.
func (p *Parser) parseIdent() (ast.Expr, error) {
	identToken, err := p.consumeExpectedKindToken(token.IDENT)
	if err != nil {
		return nil, fmt.Errorf("failed to parse identifier: %w", err)
	}

	return &ast.Ident{
		Token: identToken,
		Value: identToken.Literal,
	}, nil
}

// parseLiteral parses literal values including numbers, strings, booleans.
// Creates Index AST node with value and type information.
func (p *Parser) parseLiteral() (ast.Expr, error) {
	literalToken := p.currentToken

	if !literalToken.Kind.IsLiteral() {
		return nil, &ParseError{
			Message: fmt.Sprintf(
				"Expected a literal value (number, string, 'true', or 'false') but found '%s'",
				p.currentToken.Literal,
			),
			Token: p.currentToken,
		}
	}

	p.consumeToken()

	return &ast.Literal{
		Token: literalToken,
		Value: literalToken.Literal,
	}, nil
}

// parseParen handles parentheses for both expression grouping and array literals.
// Distinguishes between (expr) grouping and (val1, val2) arrays based on content.
func (p *Parser) parseParen() (ast.Expr, error) {
	leftParen, err := p.consumeExpectedKindToken(token.LPAREN)
	if err != nil {
		return nil, fmt.Errorf("failed to parse opening parenthesis: %w", err)
	}

	// Reject empty parentheses
	if p.currentToken.Kind.Is(token.RPAREN) {
		return nil, &ParseError{
			Message: "Empty parentheses are not allowed. Use parentheses for grouping expressions or creating arrays",
			Token:   p.currentToken,
		}
	}

	// Parse first expression
	firstExpr, err := p.parseExpr(token.PrecedenceLowest)
	if err != nil {
		return nil, fmt.Errorf("failed to parse expression inside parentheses: %w", err)
	}

	// Determine type: array literal or grouped expression
	if p.currentToken.Kind.Is(token.COMMA) {
		return p.parseListLiteral(leftParen, firstExpr)
	}

	if p.currentToken.Kind.Is(token.RPAREN) {
		_, err = p.consumeExpectedKindToken(token.RPAREN)
		if err != nil {
			return nil, fmt.Errorf("failed to parse closing parenthesis: %w", err)
		}
		return firstExpr, nil
	}

	return nil, &ParseError{
		Message: fmt.Sprintf(
			"Invalid syntax in parentheses. Expected ',' (for array) or ')' (for grouping) but found '%s'",
			p.currentToken.Literal,
		),
		Token: p.currentToken,
	}
}

// parseListLiteral parses array literals with type consistency enforcement.
// Ensures all elements are literals of the same type, no trailing commas.
func (p *Parser) parseListLiteral(leftParen token.Token, firstExpr ast.Expr) (ast.Expr, error) {
	// Ensure first element is literal
	firstLiteral, isLiteral := firstExpr.(*ast.Literal)
	if !isLiteral {
		return nil, &ParseError{
			Message: "Array elements must be literal values (numbers, strings, or booleans)",
			Token:   leftParen,
		}
	}

	literals := []*ast.Literal{firstLiteral}

	// Parse remaining elements
	for p.currentToken.Kind.Is(token.COMMA) {
		_, err := p.consumeExpectedKindToken(token.COMMA)
		if err != nil {
			return nil, fmt.Errorf("failed to consumeExpectedKindToken comma in array: %w", err)
		}

		// Check for trailing comma
		if p.currentToken.Kind.Is(token.RPAREN) {
			return nil, &ParseError{
				Message: "Trailing commas are not allowed in arrays. Remove the comma before ')'",
				Token:   p.currentToken,
			}
		}

		nextLiteral, err := p.parseLiteral()
		if err != nil {
			return nil, fmt.Errorf("failed to parse array element: %w", err)
		}

		literalValue, isLiteral := nextLiteral.(*ast.Literal)
		if !isLiteral {
			return nil, &ParseError{
				Message: "Array elements must be literal values (numbers, strings, or booleans)",
				Token:   p.currentToken,
			}
		}

		// Enforce type consistency
		if !firstLiteral.IsSameKind(literalValue) {
			return nil, &ParseError{
				Message: fmt.Sprintf(
					"Type mismatch in array: all elements must be of type %s, but found %s",
					firstLiteral.Token.Kind.Name(),
					literalValue.Token.Kind.Name(),
				),
				Token: literalValue.Token,
			}
		}

		literals = append(literals, literalValue)
	}

	rightParen, err := p.consumeExpectedKindToken(token.RPAREN)
	if err != nil {
		return nil, fmt.Errorf("failed to parse closing parenthesis for array: %w", err)
	}

	return &ast.ListLiteral{
		Lparen: leftParen,
		Rparen: rightParen,
		Values: literals,
	}, nil
}

// parseUnaryExpr parses unary expressions with single operand.
// Ensures operand implements ComputedExpr interface for evaluation.
func (p *Parser) parseUnaryExpr() (ast.Expr, error) {
	op := p.currentToken

	if !op.Kind.IsUnaryOperator() {
		return nil, &ParseError{
			Message: fmt.Sprintf("'%s' is not a valid unary operator", op.Literal),
			Token:   op,
		}
	}

	p.consumeToken()

	rightExpr, err := p.parseExpr(op.Kind.Precedence())
	if err != nil {
		return nil, fmt.Errorf("failed to parse operand for unary operator '%s': %w", op.Literal, err)
	}

	computedExpr, isComputed := rightExpr.(ast.ComputedExpr)
	if !isComputed {
		return nil, &ParseError{
			Message: fmt.Sprintf(
				"The unary operator '%s' cannot be applied to this expression type",
				op.Literal,
			),
			Token: op,
		}
	}

	return &ast.UnaryExpr{
		Op:    op,
		Right: computedExpr,
	}, nil
}

// parseBinaryExpr parses binary expressions with two operands.
// Uses operator precedence for correct expression tree construction.
func (p *Parser) parseBinaryExpr(leftExpr ast.Expr) (ast.Expr, error) {
	op := p.currentToken

	if !op.Kind.IsBinaryOperator() {
		return nil, &ParseError{
			Message: fmt.Sprintf("'%s' is not a valid binary operator", op.Literal),
			Token:   op,
		}
	}

	p.consumeToken()

	rightExpr, err := p.parseExpr(op.Kind.Precedence())
	if err != nil {
		return nil, fmt.Errorf("failed to parse right operand for operator '%s': %w", op.Literal, err)
	}

	return &ast.BinaryExpr{
		Left:  leftExpr,
		Op:    op,
		Right: rightExpr,
	}, nil
}

// parseIndexExpr parses array/object index access with nested support.
// Validates index is non-boolean literal, handles left-associativity automatically.
func (p *Parser) parseIndexExpr(leftExpr ast.Expr) (ast.Expr, error) {
	leftBrack, err := p.consumeExpectedKindToken(token.LBRACK)
	if err != nil {
		return nil, fmt.Errorf("failed to parse opening bracket for index access: %w", err)
	}

	indexLiteral, err := p.parseLiteral()
	if err != nil {
		return nil, fmt.Errorf("failed to parse index value: %w", err)
	}

	literalValue := indexLiteral.(*ast.Literal)
	if literalValue.IsBool() {
		return nil, &ParseError{
			Message: "Index must be a number (for arrays) or string (for objects), not a boolean value",
			Token:   literalValue.Token,
		}
	}

	rightBrack, err := p.consumeExpectedKindToken(token.RBRACK)
	if err != nil {
		return nil, fmt.Errorf("failed to parse closing bracket for index access: %w", err)
	}

	return &ast.IndexExpr{
		LBrack: leftBrack,
		RBrack: rightBrack,
		Left:   leftExpr,
		Index:  literalValue,
	}, nil
}
