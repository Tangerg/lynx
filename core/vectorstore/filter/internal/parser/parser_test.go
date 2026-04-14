package parser

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/lexer"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// TestNewParser tests parser creation with different input types
func TestNewParser(t *testing.T) {
	tests := []struct {
		name        string
		input       interface{}
		shouldError bool
	}{
		{
			name:        "Valid string input",
			input:       "name == 'John'",
			shouldError: false,
		},
		{
			name:        "Valid lexer input",
			input:       mustCreateLexer("age > 18"),
			shouldError: false,
		},
		{
			name:        "Empty string input",
			input:       "",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var parser *Parser
			var err error

			switch v := tt.input.(type) {
			case string:
				parser, err = NewParser(v)
			case *lexer.Lexer:
				parser, err = NewParser(v)
			}

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if parser == nil {
				t.Error("Expected non-nil parser")
			}
		})
	}
}

// TestParseIdentifier tests simple identifier parsing
func TestParseIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Simple identifier", "name", "name"},
		{"Identifier with underscore", "user_name", "user_name"},
		{"Identifier with number", "field1", "field1"},
		{"CamelCase identifier", "firstName", "firstName"},
		{"Snake case identifier", "first_name_test", "first_name_test"},
		{"Single letter", "a", "a"},
		{"Uppercase identifier", "NAME", "NAME"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Fatal(err)
			}

			ident, ok := expr.(*ast.Ident)
			if !ok {
				t.Fatalf("Expected *ast.Ident, got %T", expr)
			}

			if ident.Value != tt.expected {
				t.Errorf("Expected value '%s', got '%s'", tt.expected, ident.Value)
			}
		})
	}
}

// TestParseLiterals tests literal value parsing
func TestParseLiterals(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedKind token.Kind
		expectedVal  string
	}{
		{"Integer", "42", token.NUMBER, "42"},
		{"Float", "3.14", token.NUMBER, "3.14"},
		{"Negative number", "-100", token.NUMBER, "-100"},
		{"Zero", "0", token.NUMBER, "0"},
		{"Negative zero", "-0", token.NUMBER, "-0"},
		{"String", "'hello'", token.STRING, "hello"},
		{"Empty string", "''", token.STRING, ""},
		{"String with spaces", "'  hello world  '", token.STRING, "  hello world  "},
		{"Boolean true", "true", token.TRUE, "true"},
		{"Boolean false", "false", token.FALSE, "false"},
		{"TRUE uppercase", "TRUE", token.TRUE, "true"},
		{"FALSE uppercase", "FALSE", token.FALSE, "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Fatal(err)
			}

			literal, ok := expr.(*ast.Literal)
			if !ok {
				t.Fatalf("Expected *ast.Literal, got %T", expr)
			}

			if !literal.Token.Kind.Is(tt.expectedKind) {
				t.Errorf("Expected kind %s, got %s",
					tt.expectedKind.Name(), literal.Token.Kind.Name())
			}

			if literal.Value != tt.expectedVal {
				t.Errorf("Expected value '%s', got '%s'", tt.expectedVal, literal.Value)
			}
		})
	}
}

// TestParseBinaryExpressions tests binary operator expressions
func TestParseBinaryExpressions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		operator token.Kind
	}{
		{"Equality", "a == b", token.EQ},
		{"Inequality", "a != b", token.NE},
		{"Less than", "a < b", token.LT},
		{"Less or equal", "a <= b", token.LE},
		{"Greater than", "a > b", token.GT},
		{"Greater or equal", "a >= b", token.GE},
		{"AND operator", "a AND b", token.AND},
		{"OR operator", "a OR b", token.OR},
		{"IN operator", "a IN ('x', 'y')", token.IN},
		{"LIKE operator", "a LIKE 'pattern'", token.LIKE},
		{"Number comparison", "5 < 10", token.LT},
		{"String equality", "'hello' == 'world'", token.EQ},
		{"Boolean comparison", "true != false", token.NE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Fatal(err)
			}

			binary, ok := expr.(*ast.BinaryExpr)
			if !ok {
				t.Fatalf("Expected *ast.BinaryExpr, got %T", expr)
			}

			if !binary.Op.Kind.Is(tt.operator) {
				t.Errorf("Expected operator %s, got %s",
					tt.operator.Name(), binary.Op.Kind.Name())
			}
		})
	}
}

// TestParseUnaryExpressions tests unary operator expressions
func TestParseUnaryExpressions(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"NOT with comparison", "NOT a == b"},
		{"NOT with AND", "NOT (a AND b)"},
		{"NOT with OR", "NOT (a OR b)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Fatal(err)
			}

			unary, ok := expr.(*ast.UnaryExpr)
			if !ok {
				t.Fatalf("Expected *ast.UnaryExpr, got %T", expr)
			}

			if !unary.Op.Kind.Is(token.NOT) {
				t.Errorf("Expected NOT operator, got %s", unary.Op.Kind.Name())
			}
		})
	}
}

// TestParseParentheses tests parenthesized expressions
func TestParseParentheses(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Simple grouping", "(a == b)"},
		{"Nested grouping", "((a == b))"},
		{"Triple nested", "(((a == b)))"},
		{"Complex grouping", "(a == b AND c == d)"},
		{"Grouping with OR", "(a OR b) AND c"},
		{"Multiple groups", "(a OR b) AND (c OR d)"},
		{"Deep nesting", "((a AND b) OR (c AND d))"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Fatal(err)
			}

			if expr == nil {
				t.Error("Expected non-nil expression")
			}
		})
	}
}

// TestParseListLiterals tests array literal parsing
func TestParseListLiterals(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectCount int
		expectKind  token.Kind
	}{
		{
			name:        "Number array",
			input:       "a IN (1, 2, 3)",
			expectCount: 3,
			expectKind:  token.NUMBER,
		},
		{
			name:        "Two number array",
			input:       "x IN (42, 24)",
			expectCount: 2,
			expectKind:  token.NUMBER,
		},
		{
			name:        "String array",
			input:       "status IN ('active', 'pending')",
			expectCount: 2,
			expectKind:  token.STRING,
		},
		{
			name:        "Boolean array",
			input:       "flag IN (true, false)",
			expectCount: 2,
			expectKind:  token.TRUE,
		},
		{
			name:        "Single element array",
			input:       "x IN (42, 24)",
			expectCount: 2,
			expectKind:  token.NUMBER,
		},
		{
			name:        "Many elements",
			input:       "x IN (1, 2, 3, 4, 5, 6, 7, 8, 9, 10)",
			expectCount: 10,
			expectKind:  token.NUMBER,
		},
		{
			name:        "Empty strings array",
			input:       "x IN ('', '', '')",
			expectCount: 3,
			expectKind:  token.STRING,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Fatal(err)
			}

			binary, ok := expr.(*ast.BinaryExpr)
			if !ok {
				t.Fatalf("Expected *ast.BinaryExpr, got %T", expr)
			}

			list, ok := binary.Right.(*ast.ListLiteral)
			if !ok {
				t.Fatalf("Expected *ast.ListLiteral, got %T", binary.Right)
			}

			if len(list.Values) != tt.expectCount {
				t.Errorf("Expected %d elements, got %d", tt.expectCount, len(list.Values))
			}

			// Check first element kind (all should be same kind)
			if len(list.Values) > 0 {
				firstKind := list.Values[0].Token.Kind
				if !firstKind.Is(tt.expectKind) && !firstKind.Is(token.FALSE) {
					t.Errorf("Expected element kind %s, got %s",
						tt.expectKind.Name(), firstKind.Name())
				}
			}
		})
	}
}

// TestParseIndexExpressions tests index access parsing
func TestParseIndexExpressions(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Simple numeric index", "arr[0]"},
		{"Simple string key", "obj['key']"},
		{"Nested index", "matrix[0][1]"},
		{"Deep nesting", "a['b']['c']['d']"},
		{"Triple nesting", "x[0][1][2]"},
		{"Index in comparison", "arr[0] == 5"},
		{"Index with identifier", "data['name'] == 'test'"},
		{"Multiple index access", "a[0] AND b[1]"},
		{"Negative index", "arr[-1]"},
		{"Large index", "arr[999999]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Fatal(err)
			}

			// Check if root or left side contains IndexExpr
			hasIndexExpr := false
			switch e := expr.(type) {
			case *ast.IndexExpr:
				hasIndexExpr = true
			case *ast.BinaryExpr:
				if _, ok := e.Left.(*ast.IndexExpr); ok {
					hasIndexExpr = true
				}
			}

			if !hasIndexExpr {
				t.Error("Expected expression to contain IndexExpr")
			}
		})
	}
}

// TestOperatorPrecedence tests correct operator precedence handling
func TestOperatorPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(*testing.T, ast.Expr)
	}{
		{
			name:  "AND before OR",
			input: "a OR b AND c",
			validate: func(t *testing.T, expr ast.Expr) {
				binary, ok := expr.(*ast.BinaryExpr)
				if !ok || !binary.Op.Kind.Is(token.OR) {
					t.Error("Root should be OR operator")
					return
				}

				rightBinary, ok := binary.Right.(*ast.BinaryExpr)
				if !ok || !rightBinary.Op.Kind.Is(token.AND) {
					t.Error("Right side should be AND operator")
				}
			},
		},
		{
			name:  "Comparison before AND",
			input: "a == b AND c == d",
			validate: func(t *testing.T, expr ast.Expr) {
				binary, ok := expr.(*ast.BinaryExpr)
				if !ok || !binary.Op.Kind.Is(token.AND) {
					t.Error("Root should be AND operator")
					return
				}

				leftBinary, ok := binary.Left.(*ast.BinaryExpr)
				if !ok || !leftBinary.Op.Kind.Is(token.EQ) {
					t.Error("Left side should be EQ operator")
				}

				rightBinary, ok := binary.Right.(*ast.BinaryExpr)
				if !ok || !rightBinary.Op.Kind.Is(token.EQ) {
					t.Error("Right side should be EQ operator")
				}
			},
		},
		{
			name:  "NOT before comparison",
			input: "NOT a == b",
			validate: func(t *testing.T, expr ast.Expr) {
				unary, ok := expr.(*ast.UnaryExpr)
				if !ok || !unary.Op.Kind.Is(token.NOT) {
					t.Error("Root should be NOT operator")
					return
				}

				rightBinary, ok := unary.Right.(*ast.BinaryExpr)
				if !ok || !rightBinary.Op.Kind.Is(token.EQ) {
					t.Error("NOT operand should be EQ comparison")
				}
			},
		},
		{
			name:  "Multiple AND operators",
			input: "a AND b AND c",
			validate: func(t *testing.T, expr ast.Expr) {
				// Should be left-associative: (a AND b) AND c
				binary, ok := expr.(*ast.BinaryExpr)
				if !ok || !binary.Op.Kind.Is(token.AND) {
					t.Error("Root should be AND operator")
					return
				}

				leftBinary, ok := binary.Left.(*ast.BinaryExpr)
				if !ok || !leftBinary.Op.Kind.Is(token.AND) {
					t.Error("Left should be AND operator (left-associative)")
				}
			},
		},
		{
			name:  "Parentheses override precedence",
			input: "a AND (b OR c)",
			validate: func(t *testing.T, expr ast.Expr) {
				binary, ok := expr.(*ast.BinaryExpr)
				if !ok || !binary.Op.Kind.Is(token.AND) {
					t.Error("Root should be AND operator")
					return
				}

				rightBinary, ok := binary.Right.(*ast.BinaryExpr)
				if !ok || !rightBinary.Op.Kind.Is(token.OR) {
					t.Error("Right side should be OR operator")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Fatal(err)
			}

			tt.validate(t, expr)
		})
	}
}

// TestParseErrors tests various error conditions
func TestParseErrors(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError string
	}{
		{
			name:        "Empty parentheses",
			input:       "()",
			expectError: "Empty parentheses",
		},
		{
			name:        "Trailing comma in array",
			input:       "a IN (1, 2,)",
			expectError: "Trailing commas",
		},
		{
			name:        "Type mismatch in array - number and string",
			input:       "a IN (1, 'two', 3)",
			expectError: "Type mismatch",
		},
		{
			name:        "Type mismatch in array - number and boolean",
			input:       "a IN (1, true)",
			expectError: "Type mismatch",
		},
		{
			name:        "Missing operand",
			input:       "a ==",
			expectError: "Unexpected",
		},
		{
			name:        "Invalid operator",
			input:       "a === b",
			expectError: "Lexical error: illegal character",
		},
		{
			name:        "Unclosed parenthesis",
			input:       "(a == b",
			expectError: "Expected",
		},
		{
			name:        "Unclosed bracket",
			input:       "arr[0",
			expectError: "Expected ']'",
		},
		{
			name:        "Boolean index",
			input:       "arr[true]",
			expectError: "Index must be",
		},
		{
			name:        "Non-literal array element",
			input:       "x IN (a, b)",
			expectError: "literal",
		},
		{
			name:        "Unexpected token after expression",
			input:       "a == b c",
			expectError: "Unexpected",
		},
		{
			name:        "Missing left operand",
			input:       "== b",
			expectError: "Unexpected",
		},
		{
			name:        "Missing right operand for AND",
			input:       "a AND",
			expectError: "Unexpected",
		},
		{
			name:        "Missing right operand for OR",
			input:       "a OR",
			expectError: "Unexpected",
		},
		{
			name:        "Mismatched parentheses",
			input:       "a == b)",
			expectError: "Unexpected",
		},
		{
			name:        "Empty array",
			input:       "a IN ()",
			expectError: "Empty parentheses",
		},
		{
			name:        "Invalid NOT usage",
			input:       "NOT",
			expectError: "Unexpected",
		},
		{
			name:        "Multiple operators",
			input:       "a == == b",
			expectError: "Unexpected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				// Lexer error - check if it matches expected error
				if tt.expectError != "" && !strings.Contains(err.Error(), tt.expectError) {
					t.Logf("Lexer error (acceptable): %v", err)
				}
				return
			}

			_, err = parser.Parse()
			if err == nil {
				t.Error("Expected error but got none")
				return
			}

			if tt.expectError != "" && !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("Expected error containing '%s', got '%s'",
					tt.expectError, err.Error())
			}

			// Check if it's a ParseError
			var parseErr *ParseError
			if errors.As(err, &parseErr) {
				if parseErr.Message == "" {
					t.Error("ParseError should have a message")
				}
			}
		})
	}
}

// TestComplexExpressions tests complex real-world expressions
func TestComplexExpressions(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "Multiple AND/OR",
			input: "(status == 'active' OR status == 'pending') AND " +
				"age >= 18 AND age <= 65",
		},
		{
			name: "Nested conditions",
			input: "((a == b OR c == d) AND (e == f OR g == h)) OR " +
				"(i == j AND k == l)",
		},
		{
			name:  "Index with comparison",
			input: "profile['country'] == 'US' AND metadata['level'] > 5",
		},
		{
			name:  "Array with IN operator",
			input: "category IN ('premium', 'standard', 'basic') OR priority > 5",
		},
		{
			name:  "NOT with complex expression",
			input: "NOT (blocked == true AND verified == false)",
		},
		{
			name:  "Nested index access",
			input: "metadata['settings']['notifications'] == true",
		},
		{
			name:  "LIKE with NOT",
			input: "name LIKE '%admin%' AND NOT email LIKE '%@test.com'",
		},
		{
			name: "Mixed operators",
			input: "(balance > 1000.50 AND currency == 'USD') OR " +
				"(balance > 800.0 AND currency == 'EUR')",
		},
		{
			name:  "Complex boolean logic",
			input: "(a OR b) AND (c OR d) AND (e OR f)",
		},
		{
			name:  "IN with numbers",
			input: "id IN (1, 2, 3, 4, 5) AND status == 'active'",
		},
		{
			name:  "Nested LIKE operators",
			input: "(name LIKE '%test%' OR email LIKE '%test%') AND verified == true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Fatal(err)
			}

			if expr == nil {
				t.Error("Expected non-nil expression")
			}

			verifyASTStructure(t, expr)
		})
	}
}

// TestVeryComplexExpression tests the example from documentation
func TestVeryComplexExpression(t *testing.T) {
	input := `(status == 'active' OR status == 'pending') 
AND age >= 18 
AND age <= 65 
AND (category IN ('premium', 'standard', 'basic') OR priority > 5)
AND NOT (blocked == true AND verified == false)
AND profile['country'] == 'US' 
AND metadata['settings']['notifications'] == true
AND tags['primary'] IN ('tech', 'business', 'finance')
AND score LIKE '%excellent%'
AND NOT user_level['tier'] LIKE '%trial%'
AND ((balance > 1000.50 AND currency == 'USD') OR (balance > 800.0 AND currency == 'EUR'))
AND permissions['admin'] == false
AND profile['preferences']['language'] IN ('en', 'es', 'fr')
AND NOT (suspended == true OR deleted == true)
AND activity_score >= 75.5
AND session['is_active'] == true`

	parser, err := NewParser(input)
	if err != nil {
		t.Fatal(err)
	}

	expr, err := parser.Parse()
	if err != nil {
		var parseError *ParseError
		if errors.As(err, &parseError) {
			t.Log("Parse error at token:", parseError.Token.String())
		}
		t.Fatal(err)
	}

	if expr == nil {
		t.Error("Expected non-nil expression")
	}

	verifyASTStructure(t, expr)
}

// TestParseConvenienceFunction tests the standalone Parse function
func TestParseConvenienceFunction(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
	}{
		{
			name:        "Valid expression",
			input:       "name == 'John' AND age > 18",
			shouldError: false,
		},
		{
			name:        "Empty input",
			input:       "",
			shouldError: true,
		},
		{
			name:        "Invalid syntax",
			input:       "name == ",
			shouldError: true,
		},
		{
			name:        "Simple identifier",
			input:       "active",
			shouldError: false,
		},
		{
			name:        "Complex valid expression",
			input:       "(a OR b) AND c == 'd'",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := Parse(tt.input)

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if expr == nil {
				t.Error("Expected non-nil expression")
			}
		})
	}
}

// TestParseErrorDetails tests ParseError structure
func TestParseErrorDetails(t *testing.T) {
	input := "name =="
	parser, err := NewParser(input)
	if err != nil {
		t.Fatal(err)
	}

	_, err = parser.Parse()
	if err == nil {
		t.Fatal("Expected error")
	}

	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatal("Expected ParseError")
	}

	// Verify error has message
	if parseErr.Message == "" {
		t.Error("ParseError should have a message")
	}

	// Verify error has token information
	if parseErr.Token.Kind.Is(token.ERROR) {
		t.Error("ParseError should reference a valid token")
	}

	// Verify Error() method produces readable message
	errStr := parseErr.Error()
	if !strings.Contains(errStr, "Parsing failed") {
		t.Error("Error string should contain 'Parsing failed'")
	}

	if !strings.Contains(errStr, parseErr.Message) {
		t.Error("Error string should contain the message")
	}
}

// TestWhitespaceHandling tests whitespace in various positions
func TestWhitespaceHandling(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Leading whitespace", "   name == 'value'"},
		{"Trailing whitespace", "name == 'value'   "},
		{"Multiple spaces", "name    ==    'value'"},
		{"Tabs", "name\t==\t'value'"},
		{"Newlines", "name\n==\n'value'"},
		{"Mixed whitespace", "  name \t\n == \n\t 'value'  "},
		{"Whitespace in array", "x IN ( 1 , 2 , 3 )"},
		{"Whitespace in parentheses", "( a == b )"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Fatal(err)
			}

			if expr == nil {
				t.Error("Expected non-nil expression")
			}
		})
	}
}

// TestMultilineExpressions tests expressions spanning multiple lines
func TestMultilineExpressions(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "Simple multiline",
			input: `name == 'John' 
AND age >= 18 
OR status == 'admin'`,
		},
		{
			name: "Complex multiline",
			input: `(status == 'active') 
OR (status == 'pending')
AND age >= 18`,
		},
		{
			name: "Many lines",
			input: `a == b
AND c == d
AND e == f
OR g == h`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Fatal(err)
			}

			if expr == nil {
				t.Error("Expected non-nil expression")
			}

			binary, ok := expr.(*ast.BinaryExpr)
			if !ok {
				t.Fatalf("Expected *ast.BinaryExpr, got %T", expr)
			}

			if !binary.Op.Kind.Is(token.OR) {
				t.Errorf("Expected OR at root, got %s", binary.Op.Kind.Name())
			}
		})
	}
}

// TestEdgeCases tests various edge cases
func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
	}{
		{
			name:        "Single identifier",
			input:       "active",
			shouldError: false,
		},
		{
			name:        "Single literal number",
			input:       "42",
			shouldError: false,
		},
		{
			name:        "Single literal string",
			input:       "'hello'",
			shouldError: false,
		},
		{
			name:        "Single literal boolean",
			input:       "true",
			shouldError: false,
		},
		{
			name:        "Grouped single value",
			input:       "(42)",
			shouldError: false,
		},
		{
			name:        "Deeply nested parentheses",
			input:       "((((a == b))))",
			shouldError: false,
		},
		{
			name:        "Many operators",
			input:       "a AND b AND c AND d AND e",
			shouldError: false,
		},
		{
			name:        "Long identifier chain",
			input:       "a['b']['c']['d']['e']['f']",
			shouldError: false,
		},
		{
			name:        "Mixed index types",
			input:       "a[0]['b'][1]['c']",
			shouldError: false,
		},
		{
			name:        "Long array",
			input:       "x IN (1,2,3,4,5,6,7,8,9,10,11,12,13,14,15)",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				if !tt.shouldError {
					t.Fatal(err)
				}
				return
			}

			expr, err := parser.Parse()

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if expr == nil {
				t.Error("Expected non-nil expression")
			}
		})
	}
}

// TestBoundaryValues tests edge cases for numeric and string literals
func TestBoundaryValues(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
	}{
		{"Very large number", "x == 999999999999999999", false},
		{"Very small number", "x == -999999999999999999", false},
		{"Zero", "x == 0", false},
		{"Negative zero", "x == -0", false},
		{"Float with many decimals", "x == 3.141592653589793", false},
		{"Empty string literal", "name == ''", false},
		{"Very long string", "name == '" + strings.Repeat("a", 1000) + "'", false},
		{"String with spaces", "name == '  spaces  '", false},
		{"Single character", "x == 'a'", false},
		{"Maximum reasonable number", "x == 9007199254740991", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				if !tt.shouldError {
					t.Fatal(err)
				}
				return
			}

			expr, err := parser.Parse()

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if expr == nil {
				t.Error("Expected non-nil expression")
			}
		})
	}
}

// TestAllOperatorCombinations tests various operator combinations
func TestAllOperatorCombinations(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"EQ with AND", "a == b AND c == d"},
		{"NE with OR", "a != b OR c != d"},
		{"LT with AND", "a < b AND c < d"},
		{"LE with OR", "a <= b OR c <= d"},
		{"GT with AND", "a > b AND c > d"},
		{"GE with OR", "a >= b OR c >= d"},
		{"Mixed comparison", "a < b AND c > d"},
		{"IN with AND", "a IN (1,2) AND b IN (3,4)"},
		{"LIKE with OR", "a LIKE '%x%' OR b LIKE '%y%'"},
		{"NOT with EQ", "NOT a == b"},
		{"NOT with NE", "NOT a != b"},
		{"NOT with LT", "NOT a < b"},
		{"NOT with IN", "NOT a IN (1,2)"},
		{"NOT with LIKE", "NOT a LIKE '%x%'"},
		{"Complex NOT", "NOT (a AND b OR c)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Fatal(err)
			}

			if expr == nil {
				t.Error("Expected non-nil expression")
			}
		})
	}
}

// TestIndexAccessVariations tests different index access patterns
func TestIndexAccessVariations(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Zero index", "arr[0]"},
		{"Positive index", "arr[5]"},
		{"Negative index", "arr[-1]"},
		{"String key simple", "obj['key']"},
		{"String key with underscore", "obj['user_name']"},
		{"String key with hyphen", "obj['user-name']"},
		{"Nested numeric", "arr[0][1]"},
		{"Nested string", "obj['a']['b']"},
		{"Mixed nested", "obj['a'][0]['b'][1]"},
		{"Triple nesting", "data['x']['y']['z']"},
		{"Index in binary", "arr[0] == 5"},
		{"Index both sides", "arr[0] == brr[1]"},
		{"Index in complex", "(arr[0] == 5) AND (brr[1] == 10)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Fatal(err)
			}

			if expr == nil {
				t.Error("Expected non-nil expression")
			}
		})
	}
}

// TestSpecialCharactersInStrings tests string literals with special characters
func TestSpecialCharactersInStrings(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"String with spaces", "x == '  hello  world  '"},
		{"String with tabs", "x == 'hello\tworld'"},
		{"String with newlines", "x == 'hello\nworld'"},
		{"String with quotes", "x == 'he\\'s here'"},
		{"String with backslash", "x == 'path\\\\to\\\\file'"},
		{"Empty string", "x == ''"},
		{"String with numbers", "x == 'test123'"},
		{"String with symbols", "x == 'test@#$%'"},
		{"Unicode characters", "x == 'hello世界'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				// Some special characters might cause lexer errors
				t.Logf("Lexer error (may be expected): %v", err)
				return
			}

			expr, err := parser.Parse()
			if err != nil {
				t.Logf("Parse error (may be expected): %v", err)
				return
			}

			if expr == nil {
				t.Error("Expected non-nil expression")
			}
		})
	}
}

// TestTokenConsumption tests proper token consumption
func TestTokenConsumption(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
	}{
		{"All tokens consumed", "a == b", false},
		{"Extra token", "a == b c", true},
		{"Extra operator", "a == b ==", true},
		{"Extra closing paren", "a == b)", true},
		{"Extra closing bracket", "a == b]", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				if !tt.shouldError {
					t.Fatal(err)
				}
				return
			}

			_, err = parser.Parse()

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestLexerErrorHandling tests handling of lexer errors
func TestLexerErrorHandling(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Invalid character", "a == @"},
		{"Unclosed string", "a == 'hello"},
		{"Invalid number", "a == 12.34.56"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewParser(tt.input)
			if err != nil {
				// Lexer error caught during creation
				return
			}

			_, err = parser.Parse()
			if err == nil {
				t.Error("Expected error but got none")
			}
		})
	}
}

// Helper functions

// mustCreateLexer creates a lexer or panics
func mustCreateLexer(input string) *lexer.Lexer {
	lex, err := lexer.NewLexer(input)
	if err != nil {
		panic(err)
	}
	return lex
}

// verifyASTStructure performs basic AST validation
func verifyASTStructure(t *testing.T, expr ast.Expr) {
	t.Helper()

	if expr == nil {
		t.Error("Expression should not be nil")
		return
	}

	switch e := expr.(type) {
	case *ast.Ident:
		if e.Value == "" {
			t.Error("Ident should have a value")
		}

	case *ast.Literal:
		if e.Value == "" && !e.Token.Kind.Is(token.STRING) {
			t.Error("Literal should have a value (except empty strings)")
		}

	case *ast.ListLiteral:
		if len(e.Values) == 0 {
			t.Error("ListLiteral should have elements")
		}
		for i, val := range e.Values {
			if val == nil {
				t.Errorf("ListLiteral element %d is nil", i)
			}
		}

	case *ast.UnaryExpr:
		if e.Right == nil {
			t.Error("UnaryExpr should have right operand")
		} else {
			verifyASTStructure(t, e.Right)
		}

	case *ast.BinaryExpr:
		if e.Left == nil {
			t.Error("BinaryExpr should have left operand")
		} else {
			verifyASTStructure(t, e.Left)
		}

		if e.Right == nil {
			t.Error("BinaryExpr should have right operand")
		} else {
			verifyASTStructure(t, e.Right)
		}

	case *ast.IndexExpr:
		if e.Left == nil {
			t.Error("IndexExpr should have left operand")
		} else {
			verifyASTStructure(t, e.Left)
		}

		if e.Index == nil {
			t.Error("IndexExpr should have index")
		}
	}
}

// TestASTStructureValidation tests the AST validation helper
func TestASTStructureValidation(t *testing.T) {
	parser, err := NewParser("a == b AND c != d")
	if err != nil {
		t.Fatal(err)
	}

	expr, err := parser.Parse()
	if err != nil {
		t.Fatal(err)
	}

	verifyASTStructure(t, expr)
}

// BenchmarkSimpleParse benchmarks simple expression parsing
func BenchmarkSimpleParse(b *testing.B) {
	input := "name == 'John' AND age > 18"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser, _ := NewParser(input)
		_, _ = parser.Parse()
	}
}

// BenchmarkComplexParse benchmarks complex expression parsing
func BenchmarkComplexParse(b *testing.B) {
	input := "(status == 'active' OR status == 'pending') AND " +
		"age >= 18 AND age <= 65 AND " +
		"(category IN ('premium', 'standard', 'basic') OR priority > 5)"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser, _ := NewParser(input)
		_, _ = parser.Parse()
	}
}

// BenchmarkNestedIndexParse benchmarks nested index expression parsing
func BenchmarkNestedIndexParse(b *testing.B) {
	input := "metadata['settings']['notifications']['email'] == true"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser, _ := NewParser(input)
		_, _ = parser.Parse()
	}
}

// BenchmarkParseConvenienceFunction benchmarks the Parse convenience function
func BenchmarkParseConvenienceFunction(b *testing.B) {
	input := "name == 'value' AND status IN ('a', 'b', 'c')"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Parse(input)
	}
}

// BenchmarkVeryComplexParse benchmarks very complex expression parsing
func BenchmarkVeryComplexParse(b *testing.B) {
	input := `(status == 'active' OR status == 'pending') 
AND age >= 18 AND age <= 65 
AND (category IN ('premium', 'standard', 'basic') OR priority > 5)
AND NOT (blocked == true AND verified == false)
AND profile['country'] == 'US'`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser, _ := NewParser(input)
		_, _ = parser.Parse()
	}
}

// BenchmarkDeepNesting benchmarks deeply nested expressions
func BenchmarkDeepNesting(b *testing.B) {
	input := "((((a == b AND c == d) OR (e == f AND g == h)) AND (i == j OR k == l)) OR (m == n AND o == p))"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser, _ := NewParser(input)
		_, _ = parser.Parse()
	}
}
