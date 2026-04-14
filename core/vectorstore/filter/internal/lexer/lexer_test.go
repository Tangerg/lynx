package lexer

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// TestNewLexer tests basic lexer creation and tokenization
func TestNewLexer(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "Valid input",
			input:       "name == 'Tom'",
			shouldError: false,
		},
		{
			name:        "Empty input",
			input:       "",
			shouldError: true,
			errorMsg:    "input cannot be empty",
		},
		{
			name:        "Whitespace only",
			input:       "   ",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer, err := NewLexer(tt.input)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message containing '%s', got '%s'",
						tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if lexer == nil {
				t.Error("Expected non-nil lexer")
			}
		})
	}
}

// TestBasicTokenization tests basic token recognition
func TestBasicTokenization(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedKinds []token.Kind
	}{
		{
			name:  "Single identifier",
			input: "name",
			expectedKinds: []token.Kind{
				token.IDENT,
				token.EOF,
			},
		},
		{
			name:  "String literal",
			input: "'hello'",
			expectedKinds: []token.Kind{
				token.STRING,
				token.EOF,
			},
		},
		{
			name:  "Integer number",
			input: "42",
			expectedKinds: []token.Kind{
				token.NUMBER,
				token.EOF,
			},
		},
		{
			name:  "Float number",
			input: "3.14",
			expectedKinds: []token.Kind{
				token.NUMBER,
				token.EOF,
			},
		},
		{
			name:  "Negative number",
			input: "-100",
			expectedKinds: []token.Kind{
				token.NUMBER,
				token.EOF,
			},
		},
		{
			name:  "Boolean true",
			input: "true",
			expectedKinds: []token.Kind{
				token.TRUE,
				token.EOF,
			},
		},
		{
			name:  "Boolean false",
			input: "false",
			expectedKinds: []token.Kind{
				token.FALSE,
				token.EOF,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer, err := NewLexer(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			tokens := lexer.Tokens()
			if len(tokens) != len(tt.expectedKinds) {
				t.Errorf("Expected %d tokens, got %d", len(tt.expectedKinds), len(tokens))
				return
			}

			for i, expectedKind := range tt.expectedKinds {
				if !tokens[i].Kind.Is(expectedKind) {
					t.Errorf("Token %d: expected kind %s, got %s",
						i, expectedKind.Name(), tokens[i].Kind.Name())
				}
			}
		})
	}
}

// TestOperators tests all operator tokenization
func TestOperators(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedKind token.Kind
	}{
		{"Equality", "==", token.EQ},
		{"Inequality", "!=", token.NE},
		{"Less than", "<", token.LT},
		{"Less or equal", "<=", token.LE},
		{"Greater than", ">", token.GT},
		{"Greater or equal", ">=", token.GE},
		{"Left paren", "(", token.LPAREN},
		{"Right paren", ")", token.RPAREN},
		{"Left bracket", "[", token.LBRACK},
		{"Right bracket", "]", token.RBRACK},
		{"Comma", ",", token.COMMA},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer, err := NewLexer(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			tok := lexer.Scan()
			if !tok.Kind.Is(tt.expectedKind) {
				t.Errorf("Expected kind %s, got %s",
					tt.expectedKind.Name(), tok.Kind.Name())
			}
		})
	}
}

// TestKeywords tests all keyword recognition
func TestKeywords(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedKind token.Kind
	}{
		{"AND keyword", "AND", token.AND},
		{"OR keyword", "OR", token.OR},
		{"NOT keyword", "NOT", token.NOT},
		{"IN keyword", "IN", token.IN},
		{"LIKE keyword", "LIKE", token.LIKE},
		{"TRUE keyword", "TRUE", token.TRUE},
		{"FALSE keyword", "FALSE", token.FALSE},
		// Test case insensitivity
		{"and lowercase", "and", token.AND},
		{"or lowercase", "or", token.OR},
		{"not lowercase", "not", token.NOT},
		{"Mixed case AND", "AnD", token.AND},
		{"Mixed case OR", "oR", token.OR},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer, err := NewLexer(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			tok := lexer.Scan()
			if !tok.Kind.Is(tt.expectedKind) {
				t.Errorf("Expected kind %s, got %s",
					tt.expectedKind.Name(), tok.Kind.Name())
			}
		})
	}
}

// TestStringLiterals tests string literal tokenization
func TestStringLiterals(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedValue  string
		shouldBeString bool
	}{
		{
			name:           "Simple string",
			input:          "'hello'",
			expectedValue:  "hello",
			shouldBeString: true,
		},
		{
			name:           "Empty string",
			input:          "''",
			expectedValue:  "",
			shouldBeString: true,
		},
		{
			name:           "String with spaces",
			input:          "'hello world'",
			expectedValue:  "hello world",
			shouldBeString: true,
		},
		{
			name:           "String with newline escape",
			input:          "'hello\\nworld'",
			expectedValue:  "hello\nworld",
			shouldBeString: true,
		},
		{
			name:           "String with tab escape",
			input:          "'hello\\tworld'",
			expectedValue:  "hello\tworld",
			shouldBeString: true,
		},
		{
			name:           "String with quote escape",
			input:          "'it\\'s working'",
			expectedValue:  "it's working",
			shouldBeString: true,
		},
		{
			name:           "String with backslash escape",
			input:          "'path\\\\to\\\\file'",
			expectedValue:  "path\\to\\file",
			shouldBeString: true,
		},
		{
			name:           "String with multiple escapes",
			input:          "'line1\\nline2\\ttab'",
			expectedValue:  "line1\nline2\ttab",
			shouldBeString: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer, err := NewLexer(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			tok := lexer.Scan()

			if tt.shouldBeString && !tok.Kind.Is(token.STRING) {
				t.Errorf("Expected STRING token, got %s", tok.Kind.Name())
				return
			}

			if tok.Literal != tt.expectedValue {
				t.Errorf("Expected literal '%s', got '%s'",
					tt.expectedValue, tok.Literal)
			}
		})
	}
}

// TestNumericLiterals tests number tokenization
func TestNumericLiterals(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedValue string
		shouldBeValid bool
	}{
		{
			name:          "Positive integer",
			input:         "42",
			expectedValue: "42",
			shouldBeValid: true,
		},
		{
			name:          "Zero",
			input:         "0",
			expectedValue: "0",
			shouldBeValid: true,
		},
		{
			name:          "Negative integer",
			input:         "-42",
			expectedValue: "-42",
			shouldBeValid: true,
		},
		{
			name:          "Positive float",
			input:         "3.14",
			expectedValue: "3.14",
			shouldBeValid: true,
		},
		{
			name:          "Negative float",
			input:         "-3.14",
			expectedValue: "-3.14",
			shouldBeValid: true,
		},
		{
			name:          "Float starting with zero",
			input:         "0.5",
			expectedValue: "0.5",
			shouldBeValid: true,
		},
		{
			name:          "Large number",
			input:         "999999.99",
			expectedValue: "999999.99",
			shouldBeValid: true,
		},
		{
			name:          "Very small decimal",
			input:         "0.001",
			expectedValue: "0.001",
			shouldBeValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer, err := NewLexer(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			tok := lexer.Scan()

			if tt.shouldBeValid && !tok.Kind.Is(token.NUMBER) {
				t.Errorf("Expected NUMBER token, got %s", tok.Kind.Name())
				return
			}

			if tok.Literal != tt.expectedValue {
				t.Errorf("Expected literal '%s', got '%s'",
					tt.expectedValue, tok.Literal)
			}
		})
	}
}

// TestInvalidTokens tests error handling for invalid input
func TestInvalidTokens(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedKind token.Kind
	}{
		{
			name:         "Unterminated string",
			input:        "'hello",
			expectedKind: token.ERROR,
		},
		{
			name:         "Invalid decimal number",
			input:        "1.2.3",
			expectedKind: token.NUMBER, // First valid number will be tokenized
		},
		{
			name:         "Invalid operator",
			input:        "=",
			expectedKind: token.ERROR,
		},
		{
			name:         "Invalid operator",
			input:        "!",
			expectedKind: token.ERROR,
		},
		{
			name:         "Illegal character",
			input:        "@",
			expectedKind: token.ERROR,
		},
		{
			name:         "Illegal character",
			input:        "#",
			expectedKind: token.ERROR,
		},
		{
			name:         "Minus without digit",
			input:        "- ",
			expectedKind: token.ERROR,
		},
		{
			name:         "Decimal without digit",
			input:        "5.",
			expectedKind: token.ERROR,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer, err := NewLexer(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			tok := lexer.Scan()

			if !tok.Kind.Is(tt.expectedKind) {
				t.Errorf("Expected kind %s, got %s",
					tt.expectedKind.Name(), tok.Kind.Name())
			}
		})
	}
}

// TestWhitespaceHandling tests whitespace skipping
func TestWhitespaceHandling(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Leading spaces",
			input: "   name",
		},
		{
			name:  "Trailing spaces",
			input: "name   ",
		},
		{
			name:  "Multiple spaces between tokens",
			input: "name    ==    'value'",
		},
		{
			name:  "Tabs",
			input: "name\t==\t'value'",
		},
		{
			name:  "Newlines",
			input: "name\n==\n'value'",
		},
		{
			name:  "Mixed whitespace",
			input: "  name \t\n == \n\t 'value'  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer, err := NewLexer(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			tokens := lexer.Tokens()

			// Should have at least some tokens (not counting whitespace)
			nonWhitespaceCount := 0
			for _, tok := range tokens {
				if !tok.Kind.Is(token.EOF) {
					nonWhitespaceCount++
				}
			}

			if nonWhitespaceCount == 0 {
				t.Error("Expected some non-EOF tokens")
			}
		})
	}
}

// TestPositionTracking tests position information in tokens
func TestPositionTracking(t *testing.T) {
	input := "name == 'John'\nage >= 18"
	lexer, err := NewLexer(input)
	if err != nil {
		t.Fatal(err)
	}

	tokens := lexer.Tokens()

	// Test first token (name)
	if tokens[0].Start.Line != 1 || tokens[0].Start.Column != 1 {
		t.Errorf("First token start: expected (1,1), got (%d,%d)",
			tokens[0].Start.Line, tokens[0].Start.Column)
	}

	// Find token on second line
	var secondLineToken *token.Token
	for i := range tokens {
		if tokens[i].Start.Line == 2 {
			secondLineToken = &tokens[i]
			break
		}
	}

	if secondLineToken == nil {
		t.Error("Expected to find token on line 2")
	} else if secondLineToken.Start.Line != 2 {
		t.Errorf("Second line token: expected line 2, got %d",
			secondLineToken.Start.Line)
	}
}

// TestComplexExpression tests complete expression tokenization
func TestComplexExpression(t *testing.T) {
	input := "name == 'Tom' AND age >= 18 OR status IN ['active', 'pending']"
	lexer, err := NewLexer(input)
	if err != nil {
		t.Fatal(err)
	}

	expectedKinds := []token.Kind{
		token.IDENT,  // name
		token.EQ,     // ==
		token.STRING, // 'Tom'
		token.AND,    // AND
		token.IDENT,  // age
		token.GE,     // >=
		token.NUMBER, // 18
		token.OR,     // OR
		token.IDENT,  // status
		token.IN,     // IN
		token.LBRACK, // [
		token.STRING, // 'active'
		token.COMMA,  // ,
		token.STRING, // 'pending'
		token.RBRACK, // ]
		token.EOF,
	}

	tokens := lexer.Tokens()

	if len(tokens) != len(expectedKinds) {
		t.Errorf("Expected %d tokens, got %d", len(expectedKinds), len(tokens))

		// Log actual tokens for debugging
		for i, tok := range tokens {
			t.Logf("Token %d: %s", i, tok.String())
		}
		return
	}

	for i, expectedKind := range expectedKinds {
		if !tokens[i].Kind.Is(expectedKind) {
			t.Errorf("Token %d: expected %s, got %s",
				i, expectedKind.Name(), tokens[i].Kind.Name())
		}
	}
}

// TestNestedConditions tests complex nested expressions
func TestNestedConditions(t *testing.T) {
	input := "((name == 'Alice' OR name == 'Bob') AND age >= 21)"
	lexer, err := NewLexer(input)
	if err != nil {
		t.Fatal(err)
	}

	tokens := lexer.Tokens()

	// Verify parentheses are properly tokenized
	parenCount := 0
	for _, tok := range tokens {
		if tok.Kind.Is(token.LPAREN) {
			parenCount++
		} else if tok.Kind.Is(token.RPAREN) {
			parenCount--
		}
	}

	if parenCount != 0 {
		t.Errorf("Unbalanced parentheses: count = %d", parenCount)
	}

	// Should have at least some tokens
	if len(tokens) < 10 {
		t.Errorf("Expected more tokens for complex expression, got %d", len(tokens))
	}
}

// TestScanIterator tests the Scan method for sequential token retrieval
func TestScanIterator(t *testing.T) {
	input := "a AND b"
	lexer, err := NewLexer(input)
	if err != nil {
		t.Fatal(err)
	}

	expectedKinds := []token.Kind{
		token.IDENT,
		token.AND,
		token.IDENT,
		token.EOF,
	}

	for i, expectedKind := range expectedKinds {
		tok := lexer.Scan()
		if !tok.Kind.Is(expectedKind) {
			t.Errorf("Scan %d: expected %s, got %s",
				i, expectedKind.Name(), tok.Kind.Name())
		}
	}

	// After EOF, should continue returning EOF
	tok := lexer.Scan()
	if !tok.Kind.Is(token.EOF) {
		t.Errorf("After EOF, expected EOF, got %s", tok.Kind.Name())
	}
}

// TestTokensIterator tests the Token iterator
func TestTokensIterator(t *testing.T) {
	input := "x == 5"
	lexer, err := NewLexer(input)
	if err != nil {
		t.Fatal(err)
	}

	expectedKinds := []token.Kind{
		token.IDENT,
		token.EQ,
		token.NUMBER,
		token.EOF,
	}

	i := 0
	for tok := range lexer.Token() {
		if i >= len(expectedKinds) {
			t.Error("Too many tokens from iterator")
			break
		}

		if !tok.Kind.Is(expectedKinds[i]) {
			t.Errorf("Token %d: expected %s, got %s",
				i, expectedKinds[i].Name(), tok.Kind.Name())
		}

		i++

		// Stop after EOF
		if tok.Kind.Is(token.EOF) {
			break
		}
	}

	if i != len(expectedKinds) {
		t.Errorf("Expected %d tokens, got %d", len(expectedKinds), i)
	}
}

// TestReset tests lexer reset functionality
func TestReset(t *testing.T) {
	input := "name == 'value'"
	lexer, err := NewLexer(input)
	if err != nil {
		t.Fatal(err)
	}

	// First pass
	tokens1 := lexer.Tokens()

	// Reset and second pass
	lexer.Reset()
	tokens2 := lexer.Tokens()

	if len(tokens1) != len(tokens2) {
		t.Errorf("Token count mismatch after reset: %d vs %d",
			len(tokens1), len(tokens2))
	}

	for i := range tokens1 {
		if !tokens1[i].Kind.Is(tokens2[i].Kind) {
			t.Errorf("Token %d kind mismatch after reset: %s vs %s",
				i, tokens1[i].Kind.Name(), tokens2[i].Kind.Name())
		}

		if tokens1[i].Literal != tokens2[i].Literal {
			t.Errorf("Token %d literal mismatch after reset: '%s' vs '%s'",
				i, tokens1[i].Literal, tokens2[i].Literal)
		}
	}
}

// TestMultilineInput tests handling of multiline input
func TestMultilineInput(t *testing.T) {
	input := `name == 'John' AND
age >= 18 OR
status IN ['active', 'pending']`

	lexer, err := NewLexer(input)
	if err != nil {
		t.Fatal(err)
	}

	tokens := lexer.Tokens()

	// Check that we have tokens from multiple lines
	maxLine := 0
	for _, tok := range tokens {
		if tok.Start.Line > maxLine {
			maxLine = tok.Start.Line
		}
	}

	if maxLine < 3 {
		t.Errorf("Expected tokens from at least 3 lines, max line was %d", maxLine)
	}
}

// TestIdentifierVariations tests various identifier formats
func TestIdentifierVariations(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Simple", "name"},
		{"With underscore", "user_name"},
		{"With number", "field1"},
		{"CamelCase", "firstName"},
		{"All caps", "STATUS"},
		{"Mixed", "myField_123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer, err := NewLexer(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			tok := lexer.Scan()

			if !tok.Kind.Is(token.IDENT) {
				t.Errorf("Expected IDENT, got %s", tok.Kind.Name())
			}

			if tok.Literal != tt.input {
				t.Errorf("Expected literal '%s', got '%s'", tt.input, tok.Literal)
			}
		})
	}
}

// TestEdgeCases tests various edge cases
func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedFirst token.Kind
	}{
		{
			name:          "Only whitespace",
			input:         "   \t\n  ",
			expectedFirst: token.EOF,
		},
		{
			name:          "Single character",
			input:         "a",
			expectedFirst: token.IDENT,
		},
		{
			name:          "Single digit",
			input:         "5",
			expectedFirst: token.NUMBER,
		},
		{
			name:          "Single operator",
			input:         "(",
			expectedFirst: token.LPAREN,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer, err := NewLexer(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			tok := lexer.Scan()

			if !tok.Kind.Is(tt.expectedFirst) {
				t.Errorf("Expected %s, got %s",
					tt.expectedFirst.Name(), tok.Kind.Name())
			}
		})
	}
}

// TestCompleteExampleFromDocstring tests the example from documentation
func TestCompleteExampleFromDocstring(t *testing.T) {
	lexer, err := NewLexer("name == 'Tom' AND age >= 1.8 OR age < -15")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.Tokens()

	// Should have proper token sequence
	if len(tokens) == 0 {
		t.Error("Expected some tokens")
	}

	// Last token should be EOF
	lastToken := tokens[len(tokens)-1]
	if !lastToken.Kind.Is(token.EOF) {
		t.Errorf("Last token should be EOF, got %s", lastToken.Kind.Name())
	}

	// Log all tokens for visual inspection
	for i, tok := range tokens {
		t.Logf("Token %d: %s", i, tok.String())
	}
}

// TestMultilineExample tests multiline example from documentation
func TestMultilineExample(t *testing.T) {
	lexer, err := NewLexer("name == 'John' AND \n" +
		" age >= 18.5 OR \n" +
		" status IN ['active', 'pending'] and ( email == 'John@gmail.com' )")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.Tokens()

	// Should properly handle multiline
	if len(tokens) == 0 {
		t.Error("Expected some tokens")
	}

	// Verify email is properly tokenized
	foundEmail := false
	for _, tok := range tokens {
		if tok.Kind.Is(token.STRING) && strings.Contains(tok.Literal, "@") {
			foundEmail = true
			break
		}
	}

	if !foundEmail {
		t.Error("Expected to find email string token")
	}

	for i, tok := range tokens {
		t.Logf("Token %d: %s", i, tok.String())
	}
}

// TestComplexNestedExample tests complex nested example
func TestComplexNestedExample(t *testing.T) {
	lexer, err := NewLexer("((name == 'Alice' OR name == 'Bob') AND age >= 21) OR " +
		"(status == 'premium' AND (score > 85.5 OR level IN ['gold', 'platinum']))")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.Tokens()

	// Verify complex structure is properly tokenized
	if len(tokens) < 30 {
		t.Errorf("Expected at least 30 tokens for complex expression, got %d", len(tokens))
	}

	// Count specific token types
	identCount := 0
	stringCount := 0
	operatorCount := 0

	for _, tok := range tokens {
		switch {
		case tok.Kind.Is(token.IDENT):
			identCount++
		case tok.Kind.Is(token.STRING):
			stringCount++
		case tok.Kind.IsOperator():
			operatorCount++
		}
	}

	if identCount == 0 {
		t.Error("Expected some identifiers")
	}
	if stringCount == 0 {
		t.Error("Expected some string literals")
	}
	if operatorCount == 0 {
		t.Error("Expected some operators")
	}

	for i, tok := range tokens {
		t.Logf("Token %d: %s", i, tok.String())
	}
}

// TestPeekNextChar tests the peekNextChar internal method behavior
func TestPeekNextChar(t *testing.T) {
	lexer, err := NewLexer("ab")
	if err != nil {
		t.Fatal(err)
	}

	// Consume first character
	if err := lexer.consumeChar(); err != nil {
		t.Fatal(err)
	}

	// Peek should show 'b' without consuming
	nextChar, err := lexer.peekNextChar()
	if err != nil {
		t.Fatal(err)
	}

	if nextChar != 'b' {
		t.Errorf("Expected 'b', got '%c'", nextChar)
	}

	// Current char should still be 'a'
	if lexer.currentChar != 'a' {
		t.Errorf("Current char should still be 'a', got '%c'", lexer.currentChar)
	}

	// Consume should now get 'b'
	if err := lexer.consumeChar(); err != nil {
		t.Fatal(err)
	}

	if lexer.currentChar != 'b' {
		t.Errorf("After consume, expected 'b', got '%c'", lexer.currentChar)
	}
}

// TestConsumeChar tests character consumption and position tracking
func TestConsumeChar(t *testing.T) {
	lexer, err := NewLexer("a\nb")
	if err != nil {
		t.Fatal(err)
	}

	// Consume 'a'
	if err := lexer.consumeChar(); err != nil {
		t.Fatal(err)
	}

	if lexer.currentChar != 'a' {
		t.Errorf("Expected 'a', got '%c'", lexer.currentChar)
	}

	if lexer.currentPosition.Line != 1 || lexer.currentPosition.Column != 2 {
		t.Errorf("Position should be (1,2), got (%d,%d)",
			lexer.currentPosition.Line, lexer.currentPosition.Column)
	}

	// Consume '\n'
	if err := lexer.consumeChar(); err != nil {
		t.Fatal(err)
	}

	if lexer.currentChar != '\n' {
		t.Errorf("Expected newline, got '%c'", lexer.currentChar)
	}

	// Line should increment, column should reset
	if lexer.currentPosition.Line != 2 {
		t.Errorf("After newline, line should be 2, got %d", lexer.currentPosition.Line)
	}

	// Consume 'b'
	if err := lexer.consumeChar(); err != nil {
		t.Fatal(err)
	}

	if lexer.currentChar != 'b' {
		t.Errorf("Expected 'b', got '%c'", lexer.currentChar)
	}

	if lexer.currentPosition.Line != 2 || lexer.currentPosition.Column != 2 {
		t.Errorf("Position should be (2,1), got (%d,%d)",
			lexer.currentPosition.Line, lexer.currentPosition.Column)
	}

	// Consume past end should return EOF
	err = lexer.consumeChar()
	if !errors.Is(err, io.EOF) {
		t.Errorf("Expected EOF error, got %v", err)
	}
}

// BenchmarkScan benchmarks single token scanning
func BenchmarkScan(b *testing.B) {
	input := "name == 'value' AND age >= 18"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lexer, _ := NewLexer(input)
		for {
			tok := lexer.Scan()
			if tok.Kind.Is(token.EOF) {
				break
			}
		}
	}
}

// BenchmarkAllTokens benchmarks batch tokenization
func BenchmarkAllTokens(b *testing.B) {
	input := "name == 'value' AND age >= 18 OR status IN ['a', 'b', 'c']"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lexer, _ := NewLexer(input)
		_ = lexer.Tokens()
	}
}

// BenchmarkComplexExpression benchmarks complex expression tokenization
func BenchmarkComplexExpression(b *testing.B) {
	input := "((name == 'Alice' OR name == 'Bob') AND age >= 21) OR " +
		"(status == 'premium' AND (score > 85.5 OR level IN ['gold', 'platinum']))"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lexer, _ := NewLexer(input)
		_ = lexer.Tokens()
	}
}

// BenchmarkStringEscaping benchmarks string with escape sequences
func BenchmarkStringEscaping(b *testing.B) {
	input := "'hello\\nworld\\ttab\\r\\n'"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lexer, _ := NewLexer(input)
		_ = lexer.Scan()
	}
}

// BenchmarkReset benchmarks lexer reset operation
func BenchmarkReset(b *testing.B) {
	input := "name == 'value'"
	lexer, _ := NewLexer(input)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lexer.Reset()
	}
}
