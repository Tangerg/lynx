package token

import (
	"errors"
	"strings"
	"testing"
)

// TestTokenString tests the String method for Token
func TestTokenString(t *testing.T) {
	tests := []struct {
		name          string
		token         Token
		expectedParts []string // Parts that should be present in the output
	}{
		{
			name: "Complete token",
			token: Token{
				Kind:    IDENT,
				Start:   Position{Line: 1, Column: 5},
				End:     Position{Line: 1, Column: 10},
				Literal: "username",
			},
			expectedParts: []string{"IDENT", "1:5", "1:10", "username", "Token"},
		},
		{
			name: "Number token",
			token: Token{
				Kind:    NUMBER,
				Start:   Position{Line: 2, Column: 1},
				End:     Position{Line: 2, Column: 4},
				Literal: "123",
			},
			expectedParts: []string{"NUMBER", "2:1", "2:4", "123"},
		},
		{
			name: "Operator token",
			token: Token{
				Kind:    EQ,
				Start:   Position{Line: 1, Column: 10},
				End:     Position{Line: 1, Column: 12},
				Literal: "==",
			},
			expectedParts: []string{"EQ", "1:10", "1:12", "=="},
		},
		{
			name: "EOF token",
			token: Token{
				Kind:    EOF,
				Start:   NoPosition,
				End:     Position{Line: 10, Column: 1},
				Literal: "",
			},
			expectedParts: []string{"EOF", "0:0", "10:1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.token.String()

			for _, part := range tt.expectedParts {
				if !strings.Contains(result, part) {
					t.Errorf("String() output should contain %q, got:\n%s", part, result)
				}
			}
		})
	}
}

// TestOf tests the Of constructor function
func TestOf(t *testing.T) {
	tests := []struct {
		name    string
		kind    Kind
		literal string
		start   Position
		end     Position
	}{
		{
			name:    "Create identifier token",
			kind:    IDENT,
			literal: "name",
			start:   Position{Line: 1, Column: 1},
			end:     Position{Line: 1, Column: 5},
		},
		{
			name:    "Create number token",
			kind:    NUMBER,
			literal: "42",
			start:   Position{Line: 1, Column: 10},
			end:     Position{Line: 1, Column: 12},
		},
		{
			name:    "Create operator token",
			kind:    AND,
			literal: "and",
			start:   Position{Line: 2, Column: 5},
			end:     Position{Line: 2, Column: 8},
		},
		{
			name:    "Create empty literal token",
			kind:    EOF,
			literal: "",
			start:   NoPosition,
			end:     Position{Line: 5, Column: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := Of(tt.kind, tt.literal, tt.start, tt.end)

			if token.Kind != tt.kind {
				t.Errorf("Of() Kind = %v, want %v", token.Kind, tt.kind)
			}

			if token.Literal != tt.literal {
				t.Errorf("Of() Literal = %v, want %v", token.Literal, tt.literal)
			}

			if token.Start != tt.start {
				t.Errorf("Of() Start = %v, want %v", token.Start, tt.start)
			}

			if token.End != tt.end {
				t.Errorf("Of() End = %v, want %v", token.End, tt.end)
			}
		})
	}
}

// TestOfKind tests the OfKind constructor function
func TestOfKind(t *testing.T) {
	tests := []struct {
		name            string
		kind            Kind
		start           Position
		end             Position
		expectedLiteral string
	}{
		{
			name:            "Create TRUE token",
			kind:            TRUE,
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 5},
			expectedLiteral: "true",
		},
		{
			name:            "Create FALSE token",
			kind:            FALSE,
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 6},
			expectedLiteral: "false",
		},
		{
			name:            "Create AND token",
			kind:            AND,
			start:           Position{Line: 2, Column: 1},
			end:             Position{Line: 2, Column: 4},
			expectedLiteral: "and",
		},
		{
			name:            "Create OR token",
			kind:            OR,
			start:           Position{Line: 2, Column: 5},
			end:             Position{Line: 2, Column: 7},
			expectedLiteral: "or",
		},
		{
			name:            "Create EQ token",
			kind:            EQ,
			start:           Position{Line: 3, Column: 1},
			end:             Position{Line: 3, Column: 3},
			expectedLiteral: "==",
		},
		{
			name:            "Create LPAREN token",
			kind:            LPAREN,
			start:           Position{Line: 1, Column: 10},
			end:             Position{Line: 1, Column: 11},
			expectedLiteral: "(",
		},
		{
			name:            "Create token with no literal",
			kind:            IDENT,
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 5},
			expectedLiteral: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := OfKind(tt.kind, tt.start, tt.end)

			if token.Kind != tt.kind {
				t.Errorf("OfKind() Kind = %v, want %v", token.Kind, tt.kind)
			}

			if token.Literal != tt.expectedLiteral {
				t.Errorf("OfKind() Literal = %v, want %v", token.Literal, tt.expectedLiteral)
			}

			if token.Start != tt.start {
				t.Errorf("OfKind() Start = %v, want %v", token.Start, tt.start)
			}

			if token.End != tt.end {
				t.Errorf("OfKind() End = %v, want %v", token.End, tt.end)
			}
		})
	}
}

// TestOfEOF tests the OfEOF constructor function
func TestOfEOF(t *testing.T) {
	tests := []struct {
		name string
		pos  Position
	}{
		{
			name: "EOF at end of single line",
			pos:  Position{Line: 1, Column: 50},
		},
		{
			name: "EOF at end of multiple lines",
			pos:  Position{Line: 100, Column: 1},
		},
		{
			name: "EOF at arbitrary position",
			pos:  Position{Line: 42, Column: 17},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := OfEOF(tt.pos)

			if token.Kind != EOF {
				t.Errorf("OfEOF() Kind = %v, want EOF", token.Kind)
			}

			if token.Start != NoPosition {
				t.Errorf("OfEOF() Start = %v, want NoPosition", token.Start)
			}

			if token.End != tt.pos {
				t.Errorf("OfEOF() End = %v, want %v", token.End, tt.pos)
			}

			if token.Literal != "" {
				t.Errorf("OfEOF() Literal = %v, want empty string", token.Literal)
			}
		})
	}
}

// TestOfError tests the OfError constructor function
func TestOfError(t *testing.T) {
	tests := []struct {
		name                string
		err                 error
		pos                 Position
		expectedLiteralPart string
	}{
		{
			name:                "Error with custom message",
			err:                 errors.New("syntax error"),
			pos:                 Position{Line: 5, Column: 10},
			expectedLiteralPart: "syntax error",
		},
		{
			name:                "Error with formatted message",
			err:                 errors.New("unexpected token '}'"),
			pos:                 Position{Line: 10, Column: 1},
			expectedLiteralPart: "unexpected token '}'",
		},
		{
			name:                "Nil error uses default message",
			err:                 nil,
			pos:                 Position{Line: 1, Column: 1},
			expectedLiteralPart: "unexpected error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := OfError(tt.err, tt.pos)

			if token.Kind != ERROR {
				t.Errorf("OfError() Kind = %v, want ERROR", token.Kind)
			}

			if token.Start != tt.pos {
				t.Errorf("OfError() Start = %v, want %v", token.Start, tt.pos)
			}

			if token.End != NoPosition {
				t.Errorf("OfError() End = %v, want NoPosition", token.End)
			}

			if !strings.Contains(token.Literal, tt.expectedLiteralPart) {
				t.Errorf("OfError() Literal = %v, should contain %v",
					token.Literal, tt.expectedLiteralPart)
			}
		})
	}
}

// TestOfIllegal tests the OfIllegal constructor function
func TestOfIllegal(t *testing.T) {
	tests := []struct {
		name                string
		char                rune
		pos                 Position
		expectedLiteralPart string
	}{
		{
			name:                "Illegal character '@'",
			char:                '@',
			pos:                 Position{Line: 1, Column: 5},
			expectedLiteralPart: "'@'",
		},
		{
			name:                "Illegal character '#'",
			char:                '#',
			pos:                 Position{Line: 2, Column: 10},
			expectedLiteralPart: "'#'",
		},
		{
			name:                "Illegal character '$'",
			char:                '$',
			pos:                 Position{Line: 3, Column: 1},
			expectedLiteralPart: "'$'",
		},
		{
			name:                "Illegal Unicode character",
			char:                '™',
			pos:                 Position{Line: 1, Column: 1},
			expectedLiteralPart: "'™'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := OfIllegal(tt.char, tt.pos)

			if token.Kind != ERROR {
				t.Errorf("OfIllegal() Kind = %v, want ERROR", token.Kind)
			}

			if token.Start != tt.pos {
				t.Errorf("OfIllegal() Start = %v, want %v", token.Start, tt.pos)
			}

			if token.End != NoPosition {
				t.Errorf("OfIllegal() End = %v, want NoPosition", token.End)
			}

			if !strings.Contains(token.Literal, tt.expectedLiteralPart) {
				t.Errorf("OfIllegal() Literal = %v, should contain %v",
					token.Literal, tt.expectedLiteralPart)
			}

			if !strings.Contains(token.Literal, "illegal character") {
				t.Errorf("OfIllegal() Literal should contain 'illegal character', got %v",
					token.Literal)
			}

			// Should contain position information
			posStr := tt.pos.String()
			if !strings.Contains(token.Literal, posStr) {
				t.Errorf("OfIllegal() Literal should contain position %v, got %v",
					posStr, token.Literal)
			}
		})
	}
}

// TestOfIdent tests the OfIdent constructor function
func TestOfIdent(t *testing.T) {
	tests := []struct {
		name  string
		ident string
		start Position
		end   Position
	}{
		{
			name:  "Simple identifier",
			ident: "username",
			start: Position{Line: 1, Column: 1},
			end:   Position{Line: 1, Column: 9},
		},
		{
			name:  "Identifier with underscore",
			ident: "user_id",
			start: Position{Line: 2, Column: 5},
			end:   Position{Line: 2, Column: 12},
		},
		{
			name:  "Identifier with numbers",
			ident: "field123",
			start: Position{Line: 3, Column: 1},
			end:   Position{Line: 3, Column: 9},
		},
		{
			name:  "Single character identifier",
			ident: "x",
			start: Position{Line: 1, Column: 10},
			end:   Position{Line: 1, Column: 11},
		},
		{
			name:  "Unicode identifier",
			ident: "用户名",
			start: Position{Line: 1, Column: 1},
			end:   Position{Line: 1, Column: 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := OfIdent(tt.ident, tt.start, tt.end)

			if token.Kind != IDENT {
				t.Errorf("OfIdent() Kind = %v, want IDENT", token.Kind)
			}

			if token.Literal != tt.ident {
				t.Errorf("OfIdent() Literal = %v, want %v", token.Literal, tt.ident)
			}

			if token.Start != tt.start {
				t.Errorf("OfIdent() Start = %v, want %v", token.Start, tt.start)
			}

			if token.End != tt.end {
				t.Errorf("OfIdent() End = %v, want %v", token.End, tt.end)
			}
		})
	}
}

// TestOfLiteral tests the OfLiteral constructor function
func TestOfLiteral(t *testing.T) {
	tests := []struct {
		name            string
		kind            Kind
		literal         string
		start           Position
		end             Position
		expectedKind    Kind
		expectedLiteral string
		shouldBeError   bool
	}{
		{
			name:            "String literal",
			kind:            STRING,
			literal:         "hello",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 7},
			expectedKind:    STRING,
			expectedLiteral: "hello",
			shouldBeError:   false,
		},
		{
			name:            "Number literal - integer",
			kind:            NUMBER,
			literal:         "42",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 3},
			expectedKind:    NUMBER,
			expectedLiteral: "42",
			shouldBeError:   false,
		},
		{
			name:            "Number literal - float",
			kind:            NUMBER,
			literal:         "3.14",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 5},
			expectedKind:    NUMBER,
			expectedLiteral: "3.14",
			shouldBeError:   false,
		},
		{
			name:            "Number literal - normalized",
			kind:            NUMBER,
			literal:         "123.000",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 8},
			expectedKind:    NUMBER,
			expectedLiteral: "123",
			shouldBeError:   false,
		},
		{
			name:            "TRUE literal",
			kind:            TRUE,
			literal:         "true",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 5},
			expectedKind:    TRUE,
			expectedLiteral: "true",
			shouldBeError:   false,
		},
		{
			name:            "FALSE literal",
			kind:            FALSE,
			literal:         "false",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 6},
			expectedKind:    FALSE,
			expectedLiteral: "false",
			shouldBeError:   false,
		},
		{
			name:          "Unsupported kind",
			kind:          IDENT,
			literal:       "name",
			start:         Position{Line: 1, Column: 1},
			end:           Position{Line: 1, Column: 5},
			expectedKind:  ERROR,
			shouldBeError: true,
		},
		{
			name:          "Invalid number literal",
			kind:          NUMBER,
			literal:       "abc",
			start:         Position{Line: 1, Column: 1},
			end:           Position{Line: 1, Column: 4},
			expectedKind:  ERROR,
			shouldBeError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := OfLiteral(tt.kind, tt.literal, tt.start, tt.end)

			if token.Kind != tt.expectedKind {
				t.Errorf("OfLiteral() Kind = %v, want %v", token.Kind, tt.expectedKind)
			}

			if !tt.shouldBeError && token.Literal != tt.expectedLiteral {
				t.Errorf("OfLiteral() Literal = %v, want %v",
					token.Literal, tt.expectedLiteral)
			}

			if tt.shouldBeError && token.Kind != ERROR {
				t.Errorf("OfLiteral() should return ERROR for invalid input")
			}
		})
	}
}

// TestOfNumericLiteral tests the OfNumericLiteral constructor function
func TestOfNumericLiteral(t *testing.T) {
	tests := []struct {
		name            string
		literal         string
		start           Position
		end             Position
		expectedKind    Kind
		expectedLiteral string
		shouldBeError   bool
	}{
		{
			name:            "Integer",
			literal:         "42",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 3},
			expectedKind:    NUMBER,
			expectedLiteral: "42",
			shouldBeError:   false,
		},
		{
			name:            "Float with decimal",
			literal:         "3.14",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 5},
			expectedKind:    NUMBER,
			expectedLiteral: "3.14",
			shouldBeError:   false,
		},
		{
			name:            "Zero",
			literal:         "0",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 2},
			expectedKind:    NUMBER,
			expectedLiteral: "0",
			shouldBeError:   false,
		},
		{
			name:            "Negative number",
			literal:         "-42",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 4},
			expectedKind:    NUMBER,
			expectedLiteral: "-42",
			shouldBeError:   false,
		},
		{
			name:            "Scientific notation",
			literal:         "1.23e+02",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 9},
			expectedKind:    NUMBER,
			expectedLiteral: "123",
			shouldBeError:   false,
		},
		{
			name:            "Normalized trailing zeros",
			literal:         "123.000",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 8},
			expectedKind:    NUMBER,
			expectedLiteral: "123",
			shouldBeError:   false,
		},
		{
			name:            "Small decimal",
			literal:         "0.000123",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 9},
			expectedKind:    NUMBER,
			expectedLiteral: "0.000123",
			shouldBeError:   false,
		},
		{
			name:            "Leading zeros",
			literal:         "00123",
			start:           Position{Line: 1, Column: 1},
			end:             Position{Line: 1, Column: 6},
			expectedKind:    NUMBER,
			expectedLiteral: "123",
			shouldBeError:   false,
		},
		{
			name:          "Invalid - letters",
			literal:       "abc",
			start:         Position{Line: 1, Column: 1},
			end:           Position{Line: 1, Column: 4},
			expectedKind:  ERROR,
			shouldBeError: true,
		},
		{
			name:          "Invalid - mixed",
			literal:       "12abc",
			start:         Position{Line: 1, Column: 1},
			end:           Position{Line: 1, Column: 6},
			expectedKind:  ERROR,
			shouldBeError: true,
		},
		{
			name:          "Invalid - empty",
			literal:       "",
			start:         Position{Line: 1, Column: 1},
			end:           Position{Line: 1, Column: 1},
			expectedKind:  ERROR,
			shouldBeError: true,
		},
		{
			name:          "Invalid - special characters",
			literal:       "12.34.56",
			start:         Position{Line: 1, Column: 1},
			end:           Position{Line: 1, Column: 9},
			expectedKind:  ERROR,
			shouldBeError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := OfNumericLiteral(tt.literal, tt.start, tt.end)

			if token.Kind != tt.expectedKind {
				t.Errorf("OfNumericLiteral() Kind = %v, want %v",
					token.Kind, tt.expectedKind)
			}

			if !tt.shouldBeError {
				if token.Literal != tt.expectedLiteral {
					t.Errorf("OfNumericLiteral() Literal = %v, want %v",
						token.Literal, tt.expectedLiteral)
				}

				if token.Start != tt.start {
					t.Errorf("OfNumericLiteral() Start = %v, want %v",
						token.Start, tt.start)
				}

				if token.End != tt.end {
					t.Errorf("OfNumericLiteral() End = %v, want %v",
						token.End, tt.end)
				}
			} else {
				if token.Kind != ERROR {
					t.Errorf("OfNumericLiteral() should return ERROR for invalid input")
				}
			}
		})
	}
}

// TestTokenEquality tests token comparison
func TestTokenEquality(t *testing.T) {
	token1 := Token{
		Kind:    IDENT,
		Start:   Position{Line: 1, Column: 1},
		End:     Position{Line: 1, Column: 5},
		Literal: "name",
	}

	token2 := Token{
		Kind:    IDENT,
		Start:   Position{Line: 1, Column: 1},
		End:     Position{Line: 1, Column: 5},
		Literal: "name",
	}

	token3 := Token{
		Kind:    NUMBER,
		Start:   Position{Line: 1, Column: 1},
		End:     Position{Line: 1, Column: 5},
		Literal: "42",
	}

	if token1 != token2 {
		t.Errorf("Token with same values should be equal")
	}

	if token1 == token3 {
		t.Errorf("Token with different values should not be equal")
	}
}

// TestTokenZeroValue tests the zero value of Token
func TestTokenZeroValue(t *testing.T) {
	var token Token

	if token.Kind != 0 {
		t.Errorf("Zero-value Token Kind should be 0, got %v", token.Kind)
	}

	if token.Literal != "" {
		t.Errorf("Zero-value Token Literal should be empty, got %v", token.Literal)
	}

	if token.Start.Line != 0 || token.Start.Column != 0 {
		t.Errorf("Zero-value Token Start should be (0,0), got %v", token.Start)
	}

	if token.End.Line != 0 || token.End.Column != 0 {
		t.Errorf("Zero-value Token End should be (0,0), got %v", token.End)
	}
}

// TestNumericLiteralNormalization tests various normalization cases
func TestNumericLiteralNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.0", "1"},
		{"1.00", "1"},
		{"1.000", "1"},
		{"0.1", "0.1"},
		{"0.10", "0.1"},
		{"123.0", "123"},
		{"123.456", "123.456"},
		{"1e2", "100"},
		{"1e+2", "100"},
		{"1e-2", "0.01"},
		{"1.23e2", "123"},
		{"0.0001", "0.0001"},
	}

	start := Position{Line: 1, Column: 1}
	end := Position{Line: 1, Column: 10}

	for _, tt := range tests {
		t.Run("Normalize "+tt.input, func(t *testing.T) {
			token := OfNumericLiteral(tt.input, start, end)

			if token.Kind != NUMBER {
				t.Errorf("Expected NUMBER token, got %v", token.Kind)
			}

			if token.Literal != tt.expected {
				t.Errorf("OfNumericLiteral(%q) = %q, want %q",
					tt.input, token.Literal, tt.expected)
			}
		})
	}
}

// BenchmarkOf benchmarks the Of constructor
func BenchmarkOf(b *testing.B) {
	start := Position{Line: 1, Column: 1}
	end := Position{Line: 1, Column: 5}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Of(IDENT, "name", start, end)
	}
}

// BenchmarkOfKind benchmarks the OfKind constructor
func BenchmarkOfKind(b *testing.B) {
	start := Position{Line: 1, Column: 1}
	end := Position{Line: 1, Column: 4}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = OfKind(AND, start, end)
	}
}

// BenchmarkOfNumericLiteral benchmarks the OfNumericLiteral constructor
func BenchmarkOfNumericLiteral(b *testing.B) {
	start := Position{Line: 1, Column: 1}
	end := Position{Line: 1, Column: 8}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = OfNumericLiteral("123.456", start, end)
	}
}

// BenchmarkTokenString benchmarks the String method
func BenchmarkTokenString(b *testing.B) {
	token := Token{
		Kind:    IDENT,
		Start:   Position{Line: 1, Column: 1},
		End:     Position{Line: 1, Column: 5},
		Literal: "username",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = token.String()
	}
}
