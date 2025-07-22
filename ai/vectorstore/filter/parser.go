package filter

import (
	"fmt"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// ParseError represents a parsing error with detailed position information and context
// It provides human-readable error messages for debugging and user feedback
type ParseError struct {
	message string      // Human-readable error description
	token   token.Token // The problematic token that caused the error
}

// Token returns the token that caused the parsing error
// This is useful for error reporting and debugging purposes
func (e *ParseError) Token() token.Token {
	return e.token
}

// Error implements the error interface and formats a comprehensive error message
// The message includes both the error description and the position where it occurred
func (e *ParseError) Error() string {
	return fmt.Sprintf(
		"Parsing failed: %s (at position %s, token: '%s')",
		e.message,
		e.token.Start.String(),
		e.token.Literal,
	)
}

// Parser represents the state of a recursive descent parser with Pratt parsing capabilities
// It uses the Pratt parsing algorithm to handle operator precedence efficiently
type Parser struct {
	lexer        *Lexer      // The lexer that provides tokens from the input
	currentToken token.Token // The current token being processed

	// Pratt parser function maps for different parsing strategies
	// prefixParseFns handles tokens that appear at the beginning of expressions (literals, identifiers, unary operators)
	prefixParseFns map[token.Kind]func() (ast.Expr, error)
	// infixParseFns handles tokens that appear between expressions (binary operators, index access)
	infixParseFns map[token.Kind]func(ast.Expr) (ast.Expr, error)
}

// NewParser creates a new parser instance with proper initialization
// It accepts either a string input (which gets lexed) or an existing *Lexer
// The generic constraint ensures type safety while providing flexibility
//
// Parameters:
//   - input: Either a string to be parsed or an existing *Lexer instance
//
// Returns:
//   - *Parser: A fully initialized parser ready to parse expressions
//   - error: Any error that occurred during lexer creation or parser setup
func NewParser[I string | *Lexer](input I) (*Parser, error) {
	var (
		lexer *Lexer
		err   error
	)

	// Handle different input types using type assertion
	// This provides flexibility while maintaining type safety
	switch inp := any(input).(type) {
	case string:
		// Create a new lexer from the string input
		lexer, err = NewLexer(inp)
		if err != nil {
			return nil, fmt.Errorf("failed to create lexer from input string: %w", err)
		}
	case *Lexer:
		// Use the provided lexer but reset it to start position
		lexer = inp
		lexer.Reset()
	}

	// Create the parser with initial state
	parser := &Parser{
		lexer:          lexer,
		currentToken:   lexer.Scan(), // Get the first token
		prefixParseFns: make(map[token.Kind]func() (ast.Expr, error)),
		infixParseFns:  make(map[token.Kind]func(ast.Expr) (ast.Expr, error)),
	}

	// Register prefix parse functions for tokens that can start expressions
	parser.registerPrefix(token.IDENT, parser.parseIdent)    // Variable names, field names
	parser.registerPrefix(token.NUMBER, parser.parseLiteral) // Numeric literals
	parser.registerPrefix(token.STRING, parser.parseLiteral) // String literals
	parser.registerPrefix(token.TRUE, parser.parseLiteral)   // Boolean true
	parser.registerPrefix(token.FALSE, parser.parseLiteral)  // Boolean false

	// Register unary operators (operators that take one operand)
	parser.registerPrefix(token.NOT, parser.parseUnaryExpr) // Logical NOT operator

	// Register binary operators (operators that take two operands)
	parser.registerInfix(token.AND, parser.parseBinaryExpr)  // Logical AND
	parser.registerInfix(token.OR, parser.parseBinaryExpr)   // Logical OR
	parser.registerInfix(token.EQ, parser.parseBinaryExpr)   // Equality ==
	parser.registerInfix(token.NE, parser.parseBinaryExpr)   // Not equal !=
	parser.registerInfix(token.LT, parser.parseBinaryExpr)   // Less than <
	parser.registerInfix(token.LE, parser.parseBinaryExpr)   // Less than or equal <=
	parser.registerInfix(token.GT, parser.parseBinaryExpr)   // Greater than >
	parser.registerInfix(token.GE, parser.parseBinaryExpr)   // Greater than or equal >=
	parser.registerInfix(token.IN, parser.parseBinaryExpr)   // Membership test
	parser.registerInfix(token.LIKE, parser.parseBinaryExpr) // Pattern matching

	// Register parentheses for expression grouping and array literals
	parser.registerPrefix(token.LPAREN, parser.parseParen)

	// Register brackets for array/object index access
	parser.registerInfix(token.LBRACK, parser.parseIndexExpr)

	return parser, nil
}

// registerPrefix associates a prefix parsing function with a specific token kind
// Prefix functions handle tokens that can appear at the beginning of expressions
//
// Parameters:
//   - kind: The token kind to register the function for
//   - fn: The parsing function that handles this token kind
func (p *Parser) registerPrefix(kind token.Kind, fn func() (ast.Expr, error)) {
	p.prefixParseFns[kind] = fn
}

// registerInfix associates an infix parsing function with a specific token kind
// Infix functions handle tokens that appear between expressions (binary operators)
//
// Parameters:
//   - kind: The token kind to register the function for
//   - fn: The parsing function that handles this token kind (takes left operand)
func (p *Parser) registerInfix(kind token.Kind, fn func(ast.Expr) (ast.Expr, error)) {
	p.infixParseFns[kind] = fn
}

// nextToken advances the parser to the next token in the input stream
// This is the primary mechanism for consuming tokens during parsing
func (p *Parser) nextToken() {
	p.currentToken = p.lexer.Scan()
}

// consume verifies that the current token matches the expected kind and advances
// This is used for mandatory tokens that must appear in specific positions
//
// Parameters:
//   - expectedKind: The token kind that is expected at this position
//
// Returns:
//   - token.Token: The consumed token (if successful)
//   - error: ParseError if the token doesn't match expectations
func (p *Parser) consume(expectedKind token.Kind) (token.Token, error) {
	if p.currentToken.Kind.Is(expectedKind) {
		consumedToken := p.currentToken
		p.nextToken()
		return consumedToken, nil
	}

	// Create a detailed error message for token mismatch
	parseErr := &ParseError{
		message: fmt.Sprintf(
			"Expected '%s' but found '%s'. Check your syntax near this position",
			expectedKind.Literal(),
			p.currentToken.Kind.Literal(),
		),
		token: p.currentToken,
	}
	return p.currentToken, parseErr
}

// Parse is the main entry point for parsing complete expressions
// It parses a full expression and ensures no unexpected tokens remain
//
// Returns:
//   - ast.Expr: The parsed abstract syntax tree representing the expression
//   - error: Any parsing error that occurred
func (p *Parser) Parse() (ast.Expr, error) {
	// Parse the main expression starting with lowest precedence
	expr, err := p.parseExpr(token.PrecedenceLowest)
	if err != nil {
		return nil, fmt.Errorf("expression parsing failed: %w", err)
	}

	// Ensure we've consumed the entire input (no trailing tokens)
	if !p.currentToken.Kind.Is(token.EOF) {
		return nil, &ParseError{
			message: fmt.Sprintf(
				"Unexpected token '%s' found after complete expression. Expected end of input",
				p.currentToken.Literal,
			),
			token: p.currentToken,
		}
	}

	return expr, nil
}

// parseExpr is the core Pratt parser implementation using precedence climbing
// It handles operator precedence automatically by recursively parsing higher precedence operations
//
// Parameters:
//   - precedence: The minimum precedence level for operations to be parsed at this level
//
// Returns:
//   - ast.Expr: The parsed expression tree
//   - error: Any parsing error encountered
func (p *Parser) parseExpr(precedence int) (ast.Expr, error) {
	// Get the prefix parsing function for the current token
	prefixFn, exists := p.prefixParseFns[p.currentToken.Kind]
	if !exists {
		return nil, &ParseError{
			message: fmt.Sprintf(
				"Unexpected token '%s'. Expected an identifier, number, string, boolean, or unary operator",
				p.currentToken.Literal,
			),
			token: p.currentToken,
		}
	}

	// Parse the left side of the expression using the prefix function
	leftExpr, err := prefixFn()
	if err != nil {
		return nil, fmt.Errorf("failed to parse prefix expression: %w", err)
	}

	// Continue parsing infix expressions while precedence rules allow
	for {
		// Stop at end of input
		if p.currentToken.Kind.Is(token.EOF) {
			break
		}

		// Stop if current precedence is higher or equal to the token's precedence
		// This implements the precedence climbing algorithm
		if precedence >= p.currentToken.Kind.Precedence() {
			break
		}

		// Check if we have an infix parser for the current token
		infixFn, exists := p.infixParseFns[p.currentToken.Kind]
		if !exists {
			// No infix parser means this token doesn't continue the expression
			break
		}

		// Parse the infix expression with the left operand
		leftExpr, err = infixFn(leftExpr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse infix expression: %w", err)
		}
	}

	return leftExpr, nil
}

// Prefix parsing functions (handle tokens that can start expressions)

// parseIdent parses identifier tokens (variable names, field names)
// Identifiers are used to reference variables, object properties, etc.
//
// Returns:
//   - ast.Expr: An Ident AST node containing the identifier information
//   - error: ParseError if the current token is not an identifier
func (p *Parser) parseIdent() (ast.Expr, error) {
	identToken, err := p.consume(token.IDENT)
	if err != nil {
		return nil, fmt.Errorf("failed to parse identifier: %w", err)
	}

	return &ast.Ident{
		Token: identToken,
		Value: identToken.Literal,
	}, nil
}

// parseLiteral parses literal values including numbers, strings, and booleans
// Literals represent constant values in the expression
//
// Returns:
//   - ast.Expr: A Literal AST node containing the literal value and type information
//   - error: ParseError if the current token is not a valid literal
func (p *Parser) parseLiteral() (ast.Expr, error) {
	literalToken := p.currentToken

	// Verify this is actually a literal token
	if !literalToken.Kind.IsLiteral() {
		return nil, &ParseError{
			message: fmt.Sprintf(
				"Expected a literal value (number, string, 'true', or 'false') but found '%s'",
				p.currentToken.Literal,
			),
			token: p.currentToken,
		}
	}

	p.nextToken()

	return &ast.Literal{
		Token: literalToken,
		Value: literalToken.Literal,
	}, nil
}

// parseParen handles parentheses which can represent either:
// 1. Grouped expressions: (expr) - for controlling operator precedence
// 2. Array literals: (val1, val2, val3) - for creating lists of values
// Empty parentheses () are not allowed as they're ambiguous
//
// Returns:
//   - ast.Expr: Either the grouped expression or a ListLiteral AST node
//   - error: ParseError for syntax errors or empty parentheses
func (p *Parser) parseParen() (ast.Expr, error) {
	lParenToken, err := p.consume(token.LPAREN)
	if err != nil {
		return nil, fmt.Errorf("failed to parse opening parenthesis: %w", err)
	}

	// Check for empty parentheses (not allowed)
	if p.currentToken.Kind.Is(token.RPAREN) {
		return nil, &ParseError{
			message: "Empty parentheses are not allowed. Use parentheses for grouping expressions or creating arrays",
			token:   p.currentToken,
		}
	}

	// Parse the first expression inside parentheses
	firstExpr, err := p.parseExpr(token.PrecedenceLowest)
	if err != nil {
		return nil, fmt.Errorf("failed to parse expression inside parentheses: %w", err)
	}

	// Determine if this is an array literal or grouped expression based on what follows
	if p.currentToken.Kind.Is(token.COMMA) {
		// Array literal case: (expr1, expr2, ...)
		return p.parseListLiteral(lParenToken, firstExpr)
	}

	// Grouped expression case: (expr)
	if p.currentToken.Kind.Is(token.RPAREN) {
		_, err = p.consume(token.RPAREN)
		if err != nil {
			return nil, fmt.Errorf("failed to parse closing parenthesis: %w", err)
		}
		return firstExpr, nil
	}

	// Invalid syntax - expected either comma (for array) or closing paren (for grouping)
	return nil, &ParseError{
		message: fmt.Sprintf(
			"Invalid syntax in parentheses. Expected ',' (for array) or ')' (for grouping) but found '%s'",
			p.currentToken.Literal,
		),
		token: p.currentToken,
	}
}

// parseListLiteral parses array literals after the opening parenthesis and first element
// Arrays must contain elements of the same type and trailing commas are not allowed
// Example: (1, 2, 3) or ("a", "b", "c") but not (1, "a") or (1, 2, 3,)
//
// Parameters:
//   - lParenToken: The opening parenthesis token (for error reporting)
//   - firstExpr: The first element already parsed
//
// Returns:
//   - ast.Expr: A ListLiteral AST node containing all array elements
//   - error: ParseError for type mismatches, trailing commas, or syntax errors
func (p *Parser) parseListLiteral(lParenToken token.Token, firstExpr ast.Expr) (ast.Expr, error) {
	// Ensure the first element is a literal (arrays can only contain literals)
	firstLiteral, isLiteral := firstExpr.(*ast.Literal)
	if !isLiteral {
		return nil, &ParseError{
			message: "Array elements must be literal values (numbers, strings, or booleans)",
			token:   lParenToken,
		}
	}

	literals := []*ast.Literal{firstLiteral}

	// Parse remaining elements separated by commas
	for p.currentToken.Kind.Is(token.COMMA) {
		_, err := p.consume(token.COMMA)
		if err != nil {
			return nil, fmt.Errorf("failed to consume comma in array: %w", err)
		}

		// Check for trailing comma (not allowed)
		if p.currentToken.Kind.Is(token.RPAREN) {
			return nil, &ParseError{
				message: "Trailing commas are not allowed in arrays. Remove the comma before ')'",
				token:   p.currentToken,
			}
		}

		// Parse the next element
		nextLiteral, err := p.parseLiteral()
		if err != nil {
			return nil, fmt.Errorf("failed to parse array element: %w", err)
		}

		// Ensure it's a literal
		literalValue, isLiteral := nextLiteral.(*ast.Literal)
		if !isLiteral {
			return nil, &ParseError{
				message: "Array elements must be literal values (numbers, strings, or booleans)",
				token:   p.currentToken,
			}
		}

		// Enforce type consistency within the array
		if (firstLiteral.IsBool() && !literalValue.IsBool()) ||
			(firstLiteral.IsNumber() && !literalValue.IsNumber()) ||
			(firstLiteral.IsString() && !literalValue.IsString()) {
			return nil, &ParseError{
				message: fmt.Sprintf(
					"Type mismatch in array: all elements must be of type %s, but found %s",
					firstLiteral.Token.Kind.Name(),
					literalValue.Token.Kind.Name(),
				),
				token: literalValue.Token,
			}
		}

		literals = append(literals, literalValue)
	}

	// Consume the closing parenthesis
	rParenToken, err := p.consume(token.RPAREN)
	if err != nil {
		return nil, fmt.Errorf("failed to parse closing parenthesis for array: %w", err)
	}

	return &ast.ListLiteral{
		Lparen: lParenToken,
		Rparen: rParenToken,
		Values: literals,
	}, nil
}

// parseUnaryExpr parses unary expressions (operators that take one operand)
// Example: NOT condition, -number
// The operand must be a computable expression
//
// Returns:
//   - ast.Expr: A UnaryExpr AST node containing the operator and operand
//   - error: ParseError if the operator is invalid or operand cannot be computed
func (p *Parser) parseUnaryExpr() (ast.Expr, error) {
	operatorToken := p.currentToken

	// Verify this is actually an operator token
	if !operatorToken.Kind.IsOperator() {
		return nil, &ParseError{
			message: fmt.Sprintf("'%s' is not a valid unary operator", operatorToken.Literal),
			token:   operatorToken,
		}
	}

	p.nextToken()

	// Parse the right operand using the operator's precedence
	rightExpr, err := p.parseExpr(operatorToken.Kind.Precedence())
	if err != nil {
		return nil, fmt.Errorf("failed to parse operand for unary operator '%s': %w", operatorToken.Literal, err)
	}

	// Ensure the operand can be computed (implements ComputedExpr interface)
	computedExpr, isComputed := rightExpr.(ast.ComputedExpr)
	if !isComputed {
		return nil, &ParseError{
			message: fmt.Sprintf(
				"The unary operator '%s' cannot be applied to this expression type",
				operatorToken.Literal,
			),
			token: operatorToken,
		}
	}

	return &ast.UnaryExpr{
		Op:    operatorToken,
		Right: computedExpr,
	}, nil
}

// Infix parsing functions (handle tokens that appear between expressions)

// parseBinaryExpr parses binary expressions (operators that take two operands)
// Examples: left AND right, left == right, left < right
// Uses operator precedence to handle complex expressions correctly
//
// Parameters:
//   - leftExpr: The left operand (already parsed)
//
// Returns:
//   - ast.Expr: A BinaryExpr AST node containing both operands and the operator
//   - error: ParseError if the operator is invalid or right operand cannot be parsed
func (p *Parser) parseBinaryExpr(leftExpr ast.Expr) (ast.Expr, error) {
	operatorToken := p.currentToken

	// Verify this is actually an operator token
	if !operatorToken.Kind.IsOperator() {
		return nil, &ParseError{
			message: fmt.Sprintf("'%s' is not a valid binary operator", operatorToken.Literal),
			token:   operatorToken,
		}
	}

	p.nextToken()

	// Parse the right operand using the operator's precedence
	// This implements left-associativity and proper precedence handling
	rightExpr, err := p.parseExpr(operatorToken.Kind.Precedence())
	if err != nil {
		return nil, fmt.Errorf("failed to parse right operand for operator '%s': %w", operatorToken.Literal, err)
	}

	return &ast.BinaryExpr{
		Left:  leftExpr,
		Op:    operatorToken,
		Right: rightExpr,
	}, nil
}

// parseIndexExpr parses array/object index access expressions
// Supports nested indexing like obj[key1][key2][key3]
// The Pratt parser automatically handles the left-associativity
//
// Parameters:
//   - leftExpr: The expression being indexed (array, object, etc.)
//
// Returns:
//   - ast.Expr: An IndexExpr AST node representing the index access
//   - error: ParseError if the index is invalid or syntax is incorrect
func (p *Parser) parseIndexExpr(leftExpr ast.Expr) (ast.Expr, error) {
	lBrackToken, err := p.consume(token.LBRACK)
	if err != nil {
		return nil, fmt.Errorf("failed to parse opening bracket for index access: %w", err)
	}

	// Parse the index expression (must be a literal)
	indexLiteral, err := p.parseLiteral()
	if err != nil {
		return nil, fmt.Errorf("failed to parse index value: %w", err)
	}

	// Ensure the index is a valid literal and not a boolean
	literalValue := indexLiteral.(*ast.Literal)
	if literalValue.IsBool() {
		return nil, &ParseError{
			message: "Index must be a number (for arrays) or string (for objects), not a boolean value",
			token:   literalValue.Token,
		}
	}

	// Consume the closing bracket
	rBrackToken, err := p.consume(token.RBRACK)
	if err != nil {
		return nil, fmt.Errorf("failed to parse closing bracket for index access: %w", err)
	}

	// The Pratt parser's precedence system automatically handles chained index access
	// For example: obj[a][b][c] is parsed as ((obj[a])[b])[c] due to left-associativity
	return &ast.IndexExpr{
		LBrack:  lBrackToken,
		RBrack:  rBrackToken,
		Left:    leftExpr,
		Literal: literalValue,
	}, nil
}
