package ast

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// TestIdentExpr tests the Ident expression node
func TestIdentExpr(t *testing.T) {
	tests := []struct {
		name          string
		ident         *Ident
		expectedValue string
		expectedStart token.Position
		expectedEnd   token.Position
	}{
		{
			name: "Simple identifier",
			ident: &Ident{
				Token: token.Token{
					Kind:    token.IDENT,
					Start:   token.Position{Line: 1, Column: 1},
					End:     token.Position{Line: 1, Column: 5},
					Literal: "name",
				},
				Value: "name",
			},
			expectedValue: "name",
			expectedStart: token.Position{Line: 1, Column: 1},
			expectedEnd:   token.Position{Line: 1, Column: 5},
		},
		{
			name: "Identifier with underscore",
			ident: &Ident{
				Token: token.Token{
					Kind:    token.IDENT,
					Start:   token.Position{Line: 2, Column: 5},
					End:     token.Position{Line: 2, Column: 12},
					Literal: "user_id",
				},
				Value: "user_id",
			},
			expectedValue: "user_id",
			expectedStart: token.Position{Line: 2, Column: 5},
			expectedEnd:   token.Position{Line: 2, Column: 12},
		},
		{
			name: "Identifier with numbers",
			ident: &Ident{
				Token: token.Token{
					Kind:    token.IDENT,
					Start:   token.Position{Line: 1, Column: 1},
					End:     token.Position{Line: 1, Column: 7},
					Literal: "field1",
				},
				Value: "field1",
			},
			expectedValue: "field1",
			expectedStart: token.Position{Line: 1, Column: 1},
			expectedEnd:   token.Position{Line: 1, Column: 7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.ident.Value != tt.expectedValue {
				t.Errorf("Value = %v, want %v", tt.ident.Value, tt.expectedValue)
			}

			if tt.ident.Start() != tt.expectedStart {
				t.Errorf("Start() = %v, want %v", tt.ident.Start(), tt.expectedStart)
			}

			if tt.ident.End() != tt.expectedEnd {
				t.Errorf("End() = %v, want %v", tt.ident.End(), tt.expectedEnd)
			}

			// Test interface implementations
			var _ Expr = tt.ident
			var _ AtomicExpr = tt.ident
		})
	}
}

// TestLiteralString tests string literal operations
func TestLiteralString(t *testing.T) {
	tests := []struct {
		name           string
		literal        *Literal
		expectedValue  string
		shouldBeString bool
	}{
		{
			name: "Valid string literal",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.STRING,
					Start:   token.Position{Line: 1, Column: 1},
					End:     token.Position{Line: 1, Column: 7},
					Literal: "hello",
				},
				Value: "hello",
			},
			expectedValue:  "hello",
			shouldBeString: true,
		},
		{
			name: "Empty string literal",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.STRING,
					Start:   token.Position{Line: 1, Column: 1},
					End:     token.Position{Line: 1, Column: 3},
					Literal: "",
				},
				Value: "",
			},
			expectedValue:  "",
			shouldBeString: true,
		},
		{
			name: "String with spaces",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.STRING,
					Start:   token.Position{Line: 1, Column: 1},
					End:     token.Position{Line: 1, Column: 13},
					Literal: "hello world",
				},
				Value: "hello world",
			},
			expectedValue:  "hello world",
			shouldBeString: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.literal.IsString() != tt.shouldBeString {
				t.Errorf("IsString() = %v, want %v", tt.literal.IsString(), tt.shouldBeString)
			}

			if tt.shouldBeString {
				value, err := tt.literal.AsString()
				if err != nil {
					t.Errorf("AsString() error = %v, want nil", err)
				}
				if value != tt.expectedValue {
					t.Errorf("AsString() = %v, want %v", value, tt.expectedValue)
				}
			}
		})
	}
}

// TestLiteralStringError tests string literal error cases
func TestLiteralStringError(t *testing.T) {
	numberLiteral := &Literal{
		Token: token.Token{
			Kind:    token.NUMBER,
			Literal: "42",
		},
		Value: "42",
	}

	if numberLiteral.IsString() {
		t.Errorf("NUMBER literal should not be identified as string")
	}

	_, err := numberLiteral.AsString()
	if err == nil {
		t.Errorf("AsString() on NUMBER literal should return error")
	}

	if !strings.Contains(err.Error(), "type mismatch") {
		t.Errorf("Error message should contain 'type mismatch', got: %v", err.Error())
	}
}

// TestLiteralNumber tests number literal operations
func TestLiteralNumber(t *testing.T) {
	tests := []struct {
		name           string
		literal        *Literal
		expectedValue  float64
		shouldBeNumber bool
		shouldError    bool
	}{
		{
			name: "Integer literal",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.NUMBER,
					Literal: "42",
				},
				Value: "42",
			},
			expectedValue:  42.0,
			shouldBeNumber: true,
			shouldError:    false,
		},
		{
			name: "Float literal",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.NUMBER,
					Literal: "3.14",
				},
				Value: "3.14",
			},
			expectedValue:  3.14,
			shouldBeNumber: true,
			shouldError:    false,
		},
		{
			name: "Negative number",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.NUMBER,
					Literal: "-100",
				},
				Value: "-100",
			},
			expectedValue:  -100.0,
			shouldBeNumber: true,
			shouldError:    false,
		},
		{
			name: "Zero",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.NUMBER,
					Literal: "0",
				},
				Value: "0",
			},
			expectedValue:  0.0,
			shouldBeNumber: true,
			shouldError:    false,
		},
		{
			name: "Scientific notation",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.NUMBER,
					Literal: "1.23e2",
				},
				Value: "1.23e2",
			},
			expectedValue:  123.0,
			shouldBeNumber: true,
			shouldError:    false,
		},
		{
			name: "Large number",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.NUMBER,
					Literal: "999999.99",
				},
				Value: "999999.99",
			},
			expectedValue:  999999.99,
			shouldBeNumber: true,
			shouldError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.literal.IsNumber() != tt.shouldBeNumber {
				t.Errorf("IsNumber() = %v, want %v", tt.literal.IsNumber(), tt.shouldBeNumber)
			}

			if tt.shouldBeNumber {
				value, err := tt.literal.AsNumber()
				if (err != nil) != tt.shouldError {
					t.Errorf("AsNumber() error = %v, shouldError = %v", err, tt.shouldError)
				}
				if !tt.shouldError && value != tt.expectedValue {
					t.Errorf("AsNumber() = %v, want %v", value, tt.expectedValue)
				}
			}
		})
	}
}

// TestLiteralNumberError tests number literal error cases
func TestLiteralNumberError(t *testing.T) {
	tests := []struct {
		name    string
		literal *Literal
	}{
		{
			name: "String literal as number",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.STRING,
					Literal: "hello",
				},
				Value: "hello",
			},
		},
		{
			name: "Invalid number format",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.NUMBER,
					Literal: "abc",
				},
				Value: "abc",
			},
		},
		{
			name: "Boolean as number",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.TRUE,
					Literal: "true",
				},
				Value: "true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.literal.AsNumber()
			if err == nil {
				t.Errorf("AsNumber() should return error for invalid number")
			}
		})
	}
}

// TestLiteralBool tests boolean literal operations
func TestLiteralBool(t *testing.T) {
	tests := []struct {
		name          string
		literal       *Literal
		expectedValue bool
		shouldBeBool  bool
	}{
		{
			name: "TRUE literal",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.TRUE,
					Literal: "true",
				},
				Value: "true",
			},
			expectedValue: true,
			shouldBeBool:  true,
		},
		{
			name: "FALSE literal",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.FALSE,
					Literal: "false",
				},
				Value: "false",
			},
			expectedValue: false,
			shouldBeBool:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.literal.IsBool() != tt.shouldBeBool {
				t.Errorf("IsBool() = %v, want %v", tt.literal.IsBool(), tt.shouldBeBool)
			}

			if tt.shouldBeBool {
				value, err := tt.literal.AsBool()
				if err != nil {
					t.Errorf("AsBool() error = %v, want nil", err)
				}
				if value != tt.expectedValue {
					t.Errorf("AsBool() = %v, want %v", value, tt.expectedValue)
				}
			}
		})
	}
}

// TestLiteralBoolError tests boolean literal error cases
func TestLiteralBoolError(t *testing.T) {
	tests := []struct {
		name    string
		literal *Literal
	}{
		{
			name: "String as bool",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.STRING,
					Literal: "true",
				},
				Value: "true",
			},
		},
		{
			name: "Number as bool",
			literal: &Literal{
				Token: token.Token{
					Kind:    token.NUMBER,
					Literal: "1",
				},
				Value: "1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.literal.IsBool() {
				t.Errorf("%s literal should not be identified as bool", tt.literal.Token.Kind.Name())
			}

			_, err := tt.literal.AsBool()
			if err == nil {
				t.Errorf("AsBool() on %s literal should return error", tt.literal.Token.Kind.Name())
			}

			if !strings.Contains(err.Error(), "type mismatch") {
				t.Errorf("Error message should contain 'type mismatch', got: %v", err.Error())
			}
		})
	}
}

// TestLiteralIsSameKind tests the IsSameKind method
func TestLiteralIsSameKind(t *testing.T) {
	stringLit := &Literal{
		Token: token.Token{Kind: token.STRING},
		Value: "hello",
	}

	anotherStringLit := &Literal{
		Token: token.Token{Kind: token.STRING},
		Value: "world",
	}

	numberLit := &Literal{
		Token: token.Token{Kind: token.NUMBER},
		Value: "42",
	}

	anotherNumberLit := &Literal{
		Token: token.Token{Kind: token.NUMBER},
		Value: "100",
	}

	trueLit := &Literal{
		Token: token.Token{Kind: token.TRUE},
		Value: "true",
	}

	falseLit := &Literal{
		Token: token.Token{Kind: token.FALSE},
		Value: "false",
	}

	tests := []struct {
		name     string
		lit1     *Literal
		lit2     *Literal
		expected bool
	}{
		{
			name:     "Same string kind",
			lit1:     stringLit,
			lit2:     anotherStringLit,
			expected: true,
		},
		{
			name:     "Same number kind",
			lit1:     numberLit,
			lit2:     anotherNumberLit,
			expected: true,
		},
		{
			name:     "Different kind - string and number",
			lit1:     stringLit,
			lit2:     numberLit,
			expected: false,
		},
		{
			name:     "Same bool kind - true and false",
			lit1:     trueLit,
			lit2:     falseLit,
			expected: true,
		},
		{
			name:     "Different kind - bool and string",
			lit1:     trueLit,
			lit2:     stringLit,
			expected: false,
		},
		{
			name:     "Different kind - bool and number",
			lit1:     falseLit,
			lit2:     numberLit,
			expected: false,
		},
		{
			name:     "Nil comparison",
			lit1:     stringLit,
			lit2:     nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.lit1.IsSameKind(tt.lit2)
			if result != tt.expected {
				t.Errorf("IsSameKind() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestLiteralPositions tests position methods for Literal
func TestLiteralPositions(t *testing.T) {
	literal := &Literal{
		Token: token.Token{
			Kind:    token.STRING,
			Start:   token.Position{Line: 5, Column: 10},
			End:     token.Position{Line: 5, Column: 20},
			Literal: "test",
		},
		Value: "test",
	}

	if literal.Start() != literal.Token.Start {
		t.Errorf("Start() = %v, want %v", literal.Start(), literal.Token.Start)
	}

	if literal.End() != literal.Token.End {
		t.Errorf("End() = %v, want %v", literal.End(), literal.Token.End)
	}

	// Test interface implementations
	var _ Expr = literal
	var _ AtomicExpr = literal
}

// TestListLiteral tests the ListLiteral expression node
func TestListLiteral(t *testing.T) {
	tests := []struct {
		name          string
		listLiteral   *ListLiteral
		expectedCount int
		expectedStart token.Position
		expectedEnd   token.Position
	}{
		{
			name: "Empty list",
			listLiteral: &ListLiteral{
				Lparen: token.Token{
					Start: token.Position{Line: 1, Column: 1},
				},
				Rparen: token.Token{
					End: token.Position{Line: 1, Column: 3},
				},
				Values: []*Literal{},
			},
			expectedCount: 0,
			expectedStart: token.Position{Line: 1, Column: 1},
			expectedEnd:   token.Position{Line: 1, Column: 3},
		},
		{
			name: "List with numbers",
			listLiteral: &ListLiteral{
				Lparen: token.Token{
					Start: token.Position{Line: 1, Column: 1},
				},
				Rparen: token.Token{
					End: token.Position{Line: 1, Column: 10},
				},
				Values: []*Literal{
					{
						Token: token.Token{Kind: token.NUMBER},
						Value: "1",
					},
					{
						Token: token.Token{Kind: token.NUMBER},
						Value: "2",
					},
					{
						Token: token.Token{Kind: token.NUMBER},
						Value: "3",
					},
				},
			},
			expectedCount: 3,
			expectedStart: token.Position{Line: 1, Column: 1},
			expectedEnd:   token.Position{Line: 1, Column: 10},
		},
		{
			name: "List with strings",
			listLiteral: &ListLiteral{
				Lparen: token.Token{
					Start: token.Position{Line: 2, Column: 5},
				},
				Rparen: token.Token{
					End: token.Position{Line: 2, Column: 25},
				},
				Values: []*Literal{
					{
						Token: token.Token{Kind: token.STRING},
						Value: "apple",
					},
					{
						Token: token.Token{Kind: token.STRING},
						Value: "banana",
					},
				},
			},
			expectedCount: 2,
			expectedStart: token.Position{Line: 2, Column: 5},
			expectedEnd:   token.Position{Line: 2, Column: 25},
		},
		{
			name: "List with single element",
			listLiteral: &ListLiteral{
				Lparen: token.Token{
					Start: token.Position{Line: 1, Column: 1},
				},
				Rparen: token.Token{
					End: token.Position{Line: 1, Column: 5},
				},
				Values: []*Literal{
					{
						Token: token.Token{Kind: token.TRUE},
						Value: "true",
					},
				},
			},
			expectedCount: 1,
			expectedStart: token.Position{Line: 1, Column: 1},
			expectedEnd:   token.Position{Line: 1, Column: 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.listLiteral.Values) != tt.expectedCount {
				t.Errorf("Values count = %v, want %v",
					len(tt.listLiteral.Values), tt.expectedCount)
			}

			if tt.listLiteral.Start() != tt.expectedStart {
				t.Errorf("Start() = %v, want %v",
					tt.listLiteral.Start(), tt.expectedStart)
			}

			if tt.listLiteral.End() != tt.expectedEnd {
				t.Errorf("End() = %v, want %v",
					tt.listLiteral.End(), tt.expectedEnd)
			}

			// Test interface implementations
			var _ Expr = tt.listLiteral
			var _ AtomicExpr = tt.listLiteral
		})
	}
}

// TestUnaryExpr tests the UnaryExpr expression node
func TestUnaryExpr(t *testing.T) {
	tests := []struct {
		name               string
		unaryExpr          *UnaryExpr
		expectedPrecedence int
		expectedStart      token.Position
		expectedEnd        token.Position
	}{
		{
			name: "NOT expression with binary expr",
			unaryExpr: &UnaryExpr{
				Op: token.Token{
					Kind:  token.NOT,
					Start: token.Position{Line: 1, Column: 1},
				},
				Right: &BinaryExpr{
					Left: &Ident{
						Token: token.Token{
							Kind:  token.IDENT,
							Start: token.Position{Line: 1, Column: 5},
						},
					},
					Op: token.Token{Kind: token.EQ},
					Right: &Literal{
						Token: token.Token{
							Kind: token.TRUE,
							End:  token.Position{Line: 1, Column: 15},
						},
					},
				},
			},
			expectedPrecedence: token.PrecedenceNOT,
			expectedStart:      token.Position{Line: 1, Column: 1},
			expectedEnd:        token.Position{Line: 1, Column: 15},
		},
		{
			name: "NOT expression with identifier",
			unaryExpr: &UnaryExpr{
				Op: token.Token{
					Kind:  token.NOT,
					Start: token.Position{Line: 2, Column: 1},
				},
				Right: &BinaryExpr{
					Left: &Ident{
						Token: token.Token{
							Kind: token.IDENT,
						},
					},
					Op: token.Token{Kind: token.EQ},
					Right: &Literal{
						Token: token.Token{
							End: token.Position{Line: 2, Column: 10},
						},
					},
				},
			},
			expectedPrecedence: token.PrecedenceNOT,
			expectedStart:      token.Position{Line: 2, Column: 1},
			expectedEnd:        token.Position{Line: 2, Column: 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.unaryExpr.Precedence() != tt.expectedPrecedence {
				t.Errorf("Precedence() = %v, want %v",
					tt.unaryExpr.Precedence(), tt.expectedPrecedence)
			}

			if tt.unaryExpr.Start() != tt.expectedStart {
				t.Errorf("Start() = %v, want %v",
					tt.unaryExpr.Start(), tt.expectedStart)
			}

			if tt.unaryExpr.End() != tt.expectedEnd {
				t.Errorf("End() = %v, want %v",
					tt.unaryExpr.End(), tt.expectedEnd)
			}

			// Test interface implementations
			var _ Expr = tt.unaryExpr
			var _ ComputedExpr = tt.unaryExpr
			var _ precedenceAble = tt.unaryExpr
		})
	}
}

// TestUnaryExprIsRightLower tests IsRightLower with various right operands
func TestUnaryExprIsRightLower(t *testing.T) {
	tests := []struct {
		name      string
		unaryExpr *UnaryExpr
		expected  bool
	}{
		{
			name: "Right is lower precedence (OR < NOT)",
			unaryExpr: &UnaryExpr{
				Op: token.Token{Kind: token.NOT}, // Precedence: 3
				Right: &BinaryExpr{
					Op:    token.Token{Kind: token.OR}, // Precedence: 1
					Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
					Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
				},
			},
			expected: true,
		},
		{
			name: "Right is lower precedence (AND < NOT)",
			unaryExpr: &UnaryExpr{
				Op: token.Token{Kind: token.NOT}, // Precedence: 3
				Right: &BinaryExpr{
					Op:    token.Token{Kind: token.AND}, // Precedence: 2
					Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
					Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
				},
			},
			expected: true,
		},
		{
			name: "Right is same precedence (NOT == NOT)",
			unaryExpr: &UnaryExpr{
				Op: token.Token{Kind: token.NOT}, // Precedence: 3
				Right: &UnaryExpr{
					Op: token.Token{Kind: token.NOT}, // Precedence: 3
					Right: &BinaryExpr{
						Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
						Op:    token.Token{Kind: token.EQ},
						Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
					},
				},
			},
			expected: false,
		},
		{
			name: "Right is higher precedence (CMP > NOT)",
			unaryExpr: &UnaryExpr{
				Op: token.Token{Kind: token.NOT}, // Precedence: 3
				Right: &BinaryExpr{
					Op:    token.Token{Kind: token.EQ}, // Precedence: 4
					Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
					Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
				},
			},
			expected: false,
		},
		{
			name: "Right is atomic expr (no precedence comparison)",
			unaryExpr: &UnaryExpr{
				Op: token.Token{Kind: token.NOT},
				Right: &BinaryExpr{
					Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
					Op:    token.Token{Kind: token.EQ},
					Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.unaryExpr.IsRightLower()
			if result != tt.expected {
				t.Errorf("IsRightLower() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestBinaryExpr tests the BinaryExpr expression node
func TestBinaryExpr(t *testing.T) {
	tests := []struct {
		name               string
		binaryExpr         *BinaryExpr
		expectedPrecedence int
		expectedStart      token.Position
		expectedEnd        token.Position
	}{
		{
			name: "Equality expression",
			binaryExpr: &BinaryExpr{
				Left: &Ident{
					Token: token.Token{
						Kind:  token.IDENT,
						Start: token.Position{Line: 1, Column: 1},
					},
				},
				Op: token.Token{Kind: token.EQ},
				Right: &Literal{
					Token: token.Token{
						Kind: token.NUMBER,
						End:  token.Position{Line: 1, Column: 10},
					},
				},
			},
			expectedPrecedence: token.PrecedenceCMP,
			expectedStart:      token.Position{Line: 1, Column: 1},
			expectedEnd:        token.Position{Line: 1, Column: 10},
		},
		{
			name: "AND expression",
			binaryExpr: &BinaryExpr{
				Left: &Ident{
					Token: token.Token{
						Kind:  token.IDENT,
						Start: token.Position{Line: 1, Column: 1},
					},
				},
				Op: token.Token{Kind: token.AND},
				Right: &Ident{
					Token: token.Token{
						Kind: token.IDENT,
						End:  token.Position{Line: 1, Column: 15},
					},
				},
			},
			expectedPrecedence: token.PrecedenceAND,
			expectedStart:      token.Position{Line: 1, Column: 1},
			expectedEnd:        token.Position{Line: 1, Column: 15},
		},
		{
			name: "OR expression",
			binaryExpr: &BinaryExpr{
				Left: &Ident{
					Token: token.Token{
						Kind:  token.IDENT,
						Start: token.Position{Line: 1, Column: 1},
					},
				},
				Op: token.Token{Kind: token.OR},
				Right: &Ident{
					Token: token.Token{
						Kind: token.IDENT,
						End:  token.Position{Line: 1, Column: 10},
					},
				},
			},
			expectedPrecedence: token.PrecedenceOR,
			expectedStart:      token.Position{Line: 1, Column: 1},
			expectedEnd:        token.Position{Line: 1, Column: 10},
		},
		{
			name: "IN expression",
			binaryExpr: &BinaryExpr{
				Left: &Ident{
					Token: token.Token{
						Kind:  token.IDENT,
						Start: token.Position{Line: 2, Column: 1},
					},
				},
				Op: token.Token{Kind: token.IN},
				Right: &ListLiteral{
					Lparen: token.Token{},
					Rparen: token.Token{
						End: token.Position{Line: 2, Column: 20},
					},
				},
			},
			expectedPrecedence: token.PrecedenceMatch,
			expectedStart:      token.Position{Line: 2, Column: 1},
			expectedEnd:        token.Position{Line: 2, Column: 20},
		},
		{
			name: "LIKE expression",
			binaryExpr: &BinaryExpr{
				Left: &Ident{
					Token: token.Token{
						Kind:  token.IDENT,
						Start: token.Position{Line: 1, Column: 1},
					},
				},
				Op: token.Token{Kind: token.LIKE},
				Right: &Literal{
					Token: token.Token{
						Kind: token.STRING,
						End:  token.Position{Line: 1, Column: 15},
					},
				},
			},
			expectedPrecedence: token.PrecedenceMatch,
			expectedStart:      token.Position{Line: 1, Column: 1},
			expectedEnd:        token.Position{Line: 1, Column: 15},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.binaryExpr.Precedence() != tt.expectedPrecedence {
				t.Errorf("Precedence() = %v, want %v",
					tt.binaryExpr.Precedence(), tt.expectedPrecedence)
			}

			if tt.binaryExpr.Start() != tt.expectedStart {
				t.Errorf("Start() = %v, want %v",
					tt.binaryExpr.Start(), tt.expectedStart)
			}

			if tt.binaryExpr.End() != tt.expectedEnd {
				t.Errorf("End() = %v, want %v",
					tt.binaryExpr.End(), tt.expectedEnd)
			}

			// Test interface implementations
			var _ Expr = tt.binaryExpr
			var _ ComputedExpr = tt.binaryExpr
			var _ precedenceAble = tt.binaryExpr
		})
	}
}

// TestBinaryExprPrecedence tests precedence comparison methods
func TestBinaryExprPrecedence(t *testing.T) {
	tests := []struct {
		name               string
		binaryExpr         *BinaryExpr
		leftShouldBeLower  bool
		rightShouldBeLower bool
	}{
		{
			name: "Left is lower precedence (OR < AND)",
			binaryExpr: &BinaryExpr{
				Left: &BinaryExpr{
					Op:    token.Token{Kind: token.OR}, // Precedence: 1
					Left:  &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
					Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
				},
				Op:    token.Token{Kind: token.AND}, // Precedence: 2
				Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
			},
			leftShouldBeLower:  true,
			rightShouldBeLower: false,
		},
		{
			name: "Right is lower precedence (OR < AND)",
			binaryExpr: &BinaryExpr{
				Left: &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
				Op:   token.Token{Kind: token.AND}, // Precedence: 2
				Right: &BinaryExpr{
					Op:    token.Token{Kind: token.OR}, // Precedence: 1
					Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
					Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
				},
			},
			leftShouldBeLower:  false,
			rightShouldBeLower: true,
		},
		{
			name: "Left is lower precedence (AND < CMP)",
			binaryExpr: &BinaryExpr{
				Left: &BinaryExpr{
					Op:    token.Token{Kind: token.AND}, // Precedence: 2
					Left:  &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
					Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
				},
				Op:    token.Token{Kind: token.EQ}, // Precedence: 4
				Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
			},
			leftShouldBeLower:  true,
			rightShouldBeLower: false,
		},
		{
			name: "Right is lower precedence (NOT < CMP)",
			binaryExpr: &BinaryExpr{
				Left: &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
				Op:   token.Token{Kind: token.EQ}, // Precedence: 4
				Right: &UnaryExpr{
					Op: token.Token{Kind: token.NOT}, // Precedence: 3
					Right: &BinaryExpr{
						Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
						Op:    token.Token{Kind: token.EQ},
						Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
					},
				},
			},
			leftShouldBeLower:  false,
			rightShouldBeLower: true,
		},
		{
			name: "Both same precedence (AND == AND)",
			binaryExpr: &BinaryExpr{
				Left: &BinaryExpr{
					Op:    token.Token{Kind: token.AND}, // Precedence: 2
					Left:  &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
					Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
				},
				Op: token.Token{Kind: token.AND}, // Precedence: 2
				Right: &BinaryExpr{
					Op:    token.Token{Kind: token.AND}, // Precedence: 2
					Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 8}}},
					Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
				},
			},
			leftShouldBeLower:  false,
			rightShouldBeLower: false,
		},
		{
			name: "Left is higher precedence (CMP > AND)",
			binaryExpr: &BinaryExpr{
				Left: &BinaryExpr{
					Op:    token.Token{Kind: token.EQ}, // Precedence: 4
					Left:  &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
					Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
				},
				Op:    token.Token{Kind: token.AND}, // Precedence: 2
				Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
			},
			leftShouldBeLower:  false,
			rightShouldBeLower: false,
		},
		{
			name: "Right is higher precedence (IN > AND)",
			binaryExpr: &BinaryExpr{
				Left: &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
				Op:   token.Token{Kind: token.AND}, // Precedence: 2
				Right: &BinaryExpr{
					Op:   token.Token{Kind: token.IN}, // Precedence: 5
					Left: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
					Right: &ListLiteral{
						Lparen: token.Token{},
						Rparen: token.Token{End: token.Position{Line: 1, Column: 10}},
					},
				},
			},
			leftShouldBeLower:  false,
			rightShouldBeLower: false,
		},
		{
			name: "Atomic operands (no precedence)",
			binaryExpr: &BinaryExpr{
				Left:  &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
				Op:    token.Token{Kind: token.EQ},
				Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
			},
			leftShouldBeLower:  false,
			rightShouldBeLower: false,
		},
		{
			name: "Left is lower (OR < NOT in unary)",
			binaryExpr: &BinaryExpr{
				Left: &BinaryExpr{
					Op:    token.Token{Kind: token.OR}, // Precedence: 1
					Left:  &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
					Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
				},
				Op: token.Token{Kind: token.AND}, // Precedence: 2
				Right: &UnaryExpr{
					Op: token.Token{Kind: token.NOT}, // Precedence: 3
					Right: &BinaryExpr{
						Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 8}}},
						Op:    token.Token{Kind: token.EQ},
						Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
					},
				},
			},
			leftShouldBeLower:  true,
			rightShouldBeLower: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.binaryExpr.IsLeftLower() != tt.leftShouldBeLower {
				t.Errorf("IsLeftLower() = %v, want %v",
					tt.binaryExpr.IsLeftLower(), tt.leftShouldBeLower)
			}

			if tt.binaryExpr.IsRightLower() != tt.rightShouldBeLower {
				t.Errorf("IsRightLower() = %v, want %v",
					tt.binaryExpr.IsRightLower(), tt.rightShouldBeLower)
			}
		})
	}
}

// TestIndexExpr tests the IndexExpr expression node
func TestIndexExpr(t *testing.T) {
	tests := []struct {
		name          string
		indexExpr     *IndexExpr
		expectedStart token.Position
		expectedEnd   token.Position
	}{
		{
			name: "Simple array index",
			indexExpr: &IndexExpr{
				Left: &Ident{
					Token: token.Token{
						Kind:  token.IDENT,
						Start: token.Position{Line: 1, Column: 1},
					},
				},
				LBrack: token.Token{
					Kind: token.LBRACK,
				},
				Index: &Literal{
					Token: token.Token{Kind: token.NUMBER},
					Value: "0",
				},
				RBrack: token.Token{
					Kind: token.RBRACK,
					End:  token.Position{Line: 1, Column: 8},
				},
			},
			expectedStart: token.Position{Line: 1, Column: 1},
			expectedEnd:   token.Position{Line: 1, Column: 8},
		},
		{
			name: "String key index",
			indexExpr: &IndexExpr{
				Left: &Ident{
					Token: token.Token{
						Kind:  token.IDENT,
						Start: token.Position{Line: 1, Column: 1},
					},
				},
				LBrack: token.Token{
					Kind: token.LBRACK,
				},
				Index: &Literal{
					Token: token.Token{Kind: token.STRING},
					Value: "key",
				},
				RBrack: token.Token{
					Kind: token.RBRACK,
					End:  token.Position{Line: 1, Column: 12},
				},
			},
			expectedStart: token.Position{Line: 1, Column: 1},
			expectedEnd:   token.Position{Line: 1, Column: 12},
		},
		{
			name: "Nested index expression",
			indexExpr: &IndexExpr{
				Left: &IndexExpr{
					Left: &Ident{
						Token: token.Token{
							Kind:  token.IDENT,
							Start: token.Position{Line: 2, Column: 1},
						},
					},
					LBrack: token.Token{Kind: token.LBRACK},
					Index:  &Literal{Value: "0"},
					RBrack: token.Token{Kind: token.RBRACK},
				},
				LBrack: token.Token{Kind: token.LBRACK},
				Index:  &Literal{Value: "1"},
				RBrack: token.Token{
					Kind: token.RBRACK,
					End:  token.Position{Line: 2, Column: 12},
				},
			},
			expectedStart: token.Position{Line: 2, Column: 1},
			expectedEnd:   token.Position{Line: 2, Column: 12},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.indexExpr.Start() != tt.expectedStart {
				t.Errorf("Start() = %v, want %v",
					tt.indexExpr.Start(), tt.expectedStart)
			}

			if tt.indexExpr.End() != tt.expectedEnd {
				t.Errorf("End() = %v, want %v",
					tt.indexExpr.End(), tt.expectedEnd)
			}

			// Test interface implementations
			var _ Expr = tt.indexExpr
			var _ ComputedExpr = tt.indexExpr
		})
	}
}

// TestExprInterfaces tests that all expression types implement required interfaces
func TestExprInterfaces(t *testing.T) {
	tests := []struct {
		name             string
		expr             Expr
		shouldBeAtomic   bool
		shouldBeComputed bool
	}{
		{
			name:             "Ident is atomic",
			expr:             &Ident{},
			shouldBeAtomic:   true,
			shouldBeComputed: false,
		},
		{
			name:             "Literal is atomic",
			expr:             &Literal{},
			shouldBeAtomic:   true,
			shouldBeComputed: false,
		},
		{
			name:             "ListLiteral is atomic",
			expr:             &ListLiteral{},
			shouldBeAtomic:   true,
			shouldBeComputed: false,
		},
		{
			name:             "UnaryExpr is computed",
			expr:             &UnaryExpr{},
			shouldBeAtomic:   false,
			shouldBeComputed: true,
		},
		{
			name:             "BinaryExpr is computed",
			expr:             &BinaryExpr{},
			shouldBeAtomic:   false,
			shouldBeComputed: true,
		},
		{
			name:             "IndexExpr is computed",
			expr:             &IndexExpr{},
			shouldBeAtomic:   false,
			shouldBeComputed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// All should implement Expr
			var _ Expr = tt.expr

			if tt.shouldBeAtomic {
				if _, ok := tt.expr.(AtomicExpr); !ok {
					t.Errorf("%T should implement AtomicExpr", tt.expr)
				}
			}

			if tt.shouldBeComputed {
				if _, ok := tt.expr.(ComputedExpr); !ok {
					t.Errorf("%T should implement ComputedExpr", tt.expr)
				}
			}
		})
	}
}

// TestComplexExpressionTree tests a complex expression tree
func TestComplexExpressionTree(t *testing.T) {
	// Construct expression: (age > 18) AND (name == 'John')
	// Precedence: CMP (4) > AND (2), so comparison operators bind tighter
	expr := &BinaryExpr{
		Left: &BinaryExpr{
			Left: &Ident{
				Token: token.Token{
					Kind:  token.IDENT,
					Start: token.Position{Line: 1, Column: 1},
					End:   token.Position{Line: 1, Column: 4},
				},
				Value: "age",
			},
			Op: token.Token{
				Kind: token.GT,
			},
			Right: &Literal{
				Token: token.Token{
					Kind: token.NUMBER,
					End:  token.Position{Line: 1, Column: 8},
				},
				Value: "18",
			},
		},
		Op: token.Token{
			Kind: token.AND,
		},
		Right: &BinaryExpr{
			Left: &Ident{
				Token: token.Token{
					Kind: token.IDENT,
				},
				Value: "name",
			},
			Op: token.Token{
				Kind: token.EQ,
			},
			Right: &Literal{
				Token: token.Token{
					Kind: token.STRING,
					End:  token.Position{Line: 1, Column: 30},
				},
				Value: "John",
			},
		},
	}

	// Test precedence
	if expr.Precedence() != token.PrecedenceAND {
		t.Errorf("Root expression should have AND precedence (2), got %d", expr.Precedence())
	}

	// Test that left operand (GT with precedence 4) has HIGHER precedence than AND (2)
	// So IsLeftLower should return false
	if expr.IsLeftLower() {
		t.Errorf("Left operand (GT, precedence 4) should NOT be lower than AND (precedence 2)")
	}

	// Test that right operand (EQ with precedence 4) has HIGHER precedence than AND (2)
	// So IsRightLower should return false
	if expr.IsRightLower() {
		t.Errorf("Right operand (EQ, precedence 4) should NOT be lower than AND (precedence 2)")
	}

	// Test positions
	expectedStart := token.Position{Line: 1, Column: 1}
	expectedEnd := token.Position{Line: 1, Column: 30}

	if expr.Start() != expectedStart {
		t.Errorf("Start() = %v, want %v", expr.Start(), expectedStart)
	}

	if expr.End() != expectedEnd {
		t.Errorf("End() = %v, want %v", expr.End(), expectedEnd)
	}
}

// TestPrecedenceOrdering tests the correct precedence ordering
func TestPrecedenceOrdering(t *testing.T) {
	// Test that precedence values follow the correct ordering
	// Lower values = lower precedence (evaluated later)
	// Higher values = higher precedence (evaluated first)

	precedenceTests := []struct {
		name  string
		kind  token.Kind
		value int
	}{
		{"OR has lowest precedence", token.OR, token.PrecedenceOR},
		{"AND has higher precedence than OR", token.AND, token.PrecedenceAND},
		{"NOT has higher precedence than AND", token.NOT, token.PrecedenceNOT},
		{"CMP has higher precedence than NOT", token.EQ, token.PrecedenceCMP},
		{"MATCH has higher precedence than CMP", token.IN, token.PrecedenceMatch},
	}

	for _, tt := range precedenceTests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.kind.Precedence() != tt.value {
				t.Errorf("%s: got precedence %d, want %d",
					tt.name, tt.kind.Precedence(), tt.value)
			}
		})
	}

	// Verify ordering
	if token.PrecedenceOR >= token.PrecedenceAND {
		t.Errorf("OR precedence (%d) should be less than AND precedence (%d)",
			token.PrecedenceOR, token.PrecedenceAND)
	}

	if token.PrecedenceAND >= token.PrecedenceNOT {
		t.Errorf("AND precedence (%d) should be less than NOT precedence (%d)",
			token.PrecedenceAND, token.PrecedenceNOT)
	}

	if token.PrecedenceNOT >= token.PrecedenceCMP {
		t.Errorf("NOT precedence (%d) should be less than CMP precedence (%d)",
			token.PrecedenceNOT, token.PrecedenceCMP)
	}

	if token.PrecedenceCMP >= token.PrecedenceMatch {
		t.Errorf("CMP precedence (%d) should be less than MATCH precedence (%d)",
			token.PrecedenceCMP, token.PrecedenceMatch)
	}
}

// TestExpressionPrecedenceScenarios tests various real-world precedence scenarios
func TestExpressionPrecedenceScenarios(t *testing.T) {
	tests := []struct {
		name        string
		description string
		buildExpr   func() *BinaryExpr
		test        func(*testing.T, *BinaryExpr)
	}{
		{
			name:        "OR with AND operands",
			description: "expr1 AND expr2 OR expr3 AND expr4",
			buildExpr: func() *BinaryExpr {
				// (expr1 AND expr2) OR (expr3 AND expr4)
				return &BinaryExpr{
					Left: &BinaryExpr{
						Op:    token.Token{Kind: token.AND}, // Precedence: 2
						Left:  &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
						Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
					},
					Op: token.Token{Kind: token.OR}, // Precedence: 1
					Right: &BinaryExpr{
						Op:    token.Token{Kind: token.AND}, // Precedence: 2
						Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 8}}},
						Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
					},
				}
			},
			test: func(t *testing.T, expr *BinaryExpr) {
				// AND (2) > OR (1), so AND has HIGHER precedence
				// IsLeftLower checks if left.Precedence() < this.Precedence()
				// 2 < 1 is false, so IsLeftLower should return false
				if expr.IsLeftLower() {
					t.Errorf("Left AND (prec 2) should NOT be lower than OR (prec 1)")
				}
				if expr.IsRightLower() {
					t.Errorf("Right AND (prec 2) should NOT be lower than OR (prec 1)")
				}
			},
		},
		{
			name:        "Comparison in logical expression",
			description: "a > b AND c < d",
			buildExpr: func() *BinaryExpr {
				return &BinaryExpr{
					Left: &BinaryExpr{
						Op:    token.Token{Kind: token.GT}, // Precedence: 4
						Left:  &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
						Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 3}}},
					},
					Op: token.Token{Kind: token.AND}, // Precedence: 2
					Right: &BinaryExpr{
						Op:    token.Token{Kind: token.LT}, // Precedence: 4
						Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 6}}},
						Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 8}}},
					},
				}
			},
			test: func(t *testing.T, expr *BinaryExpr) {
				// CMP (4) > AND (2), so comparisons have higher precedence
				if expr.IsLeftLower() {
					t.Errorf("Left GT (prec 4) should NOT be lower than AND (prec 2)")
				}
				if expr.IsRightLower() {
					t.Errorf("Right LT (prec 4) should NOT be lower than AND (prec 2)")
				}
			},
		},
		{
			name:        "NOT with comparison",
			description: "NOT a == b AND c",
			buildExpr: func() *BinaryExpr {
				return &BinaryExpr{
					Left: &UnaryExpr{
						Op: token.Token{Kind: token.NOT}, // Precedence: 3
						Right: &BinaryExpr{
							Op:    token.Token{Kind: token.EQ}, // Precedence: 4
							Left:  &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 5}}},
							Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 8}}},
						},
					},
					Op:    token.Token{Kind: token.AND}, // Precedence: 2
					Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 15}}},
				}
			},
			test: func(t *testing.T, expr *BinaryExpr) {
				// NOT (3) > AND (2), so NOT has higher precedence
				if expr.IsLeftLower() {
					t.Errorf("Left NOT (prec 3) should NOT be lower than AND (prec 2)")
				}
			},
		},
		{
			name:        "IN expression with AND",
			description: "status IN ['a', 'b'] AND age > 18",
			buildExpr: func() *BinaryExpr {
				return &BinaryExpr{
					Left: &BinaryExpr{
						Op:   token.Token{Kind: token.IN}, // Precedence: 5
						Left: &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
						Right: &ListLiteral{
							Lparen: token.Token{},
							Rparen: token.Token{End: token.Position{Line: 1, Column: 20}},
						},
					},
					Op: token.Token{Kind: token.AND}, // Precedence: 2
					Right: &BinaryExpr{
						Op:    token.Token{Kind: token.GT}, // Precedence: 4
						Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 25}}},
						Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 30}}},
					},
				}
			},
			test: func(t *testing.T, expr *BinaryExpr) {
				// IN (5) > AND (2), so IN has higher precedence
				if expr.IsLeftLower() {
					t.Errorf("Left IN (prec 5) should NOT be lower than AND (prec 2)")
				}
				// GT (4) > AND (2), so GT has higher precedence
				if expr.IsRightLower() {
					t.Errorf("Right GT (prec 4) should NOT be lower than AND (prec 2)")
				}
			},
		},
		{
			name:        "Mixed precedence",
			description: "a OR b AND c > d",
			buildExpr: func() *BinaryExpr {
				return &BinaryExpr{
					Left: &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
					Op:   token.Token{Kind: token.OR}, // Precedence: 1
					Right: &BinaryExpr{
						Left: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
						Op:   token.Token{Kind: token.AND}, // Precedence: 2
						Right: &BinaryExpr{
							Op:    token.Token{Kind: token.GT}, // Precedence: 4
							Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
							Right: &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 15}}},
						},
					},
				}
			},
			test: func(t *testing.T, expr *BinaryExpr) {
				// Right is AND (2) > OR (1)
				if expr.IsRightLower() {
					t.Errorf("Right AND (prec 2) should NOT be lower than OR (prec 1)")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := tt.buildExpr()
			tt.test(t, expr)
		})
	}
}

// TestNestedExpressions tests deeply nested expression structures
func TestNestedExpressions(t *testing.T) {
	// Test expression: NOT (a AND b) OR (c > d AND e IN [1,2,3])
	expr := &BinaryExpr{
		Left: &UnaryExpr{
			Op: token.Token{
				Kind:  token.NOT,
				Start: token.Position{Line: 1, Column: 1},
			},
			Right: &BinaryExpr{
				Left: &Ident{
					Token: token.Token{Kind: token.IDENT},
				},
				Op: token.Token{Kind: token.AND},
				Right: &Ident{
					Token: token.Token{End: token.Position{Line: 1, Column: 15}},
				},
			},
		},
		Op: token.Token{Kind: token.OR},
		Right: &BinaryExpr{
			Left: &BinaryExpr{
				Left: &Ident{
					Token: token.Token{Kind: token.IDENT},
				},
				Op: token.Token{Kind: token.GT},
				Right: &Ident{
					Token: token.Token{End: token.Position{Line: 1, Column: 25}},
				},
			},
			Op: token.Token{Kind: token.AND},
			Right: &BinaryExpr{
				Left: &Ident{
					Token: token.Token{Kind: token.IDENT},
				},
				Op: token.Token{Kind: token.IN},
				Right: &ListLiteral{
					Lparen: token.Token{},
					Rparen: token.Token{End: token.Position{Line: 1, Column: 40}},
					Values: []*Literal{
						{Token: token.Token{Kind: token.NUMBER}, Value: "1"},
						{Token: token.Token{Kind: token.NUMBER}, Value: "2"},
						{Token: token.Token{Kind: token.NUMBER}, Value: "3"},
					},
				},
			},
		},
	}

	// Test root precedence
	if expr.Precedence() != token.PrecedenceOR {
		t.Errorf("Root should have OR precedence, got %d", expr.Precedence())
	}

	// Test left side (NOT expression)
	leftUnary, ok := expr.Left.(*UnaryExpr)
	if !ok {
		t.Fatal("Left should be UnaryExpr")
	}
	if leftUnary.Precedence() != token.PrecedenceNOT {
		t.Errorf("Left UnaryExpr should have NOT precedence, got %d", leftUnary.Precedence())
	}

	// Test right side (AND expression)
	rightBinary, ok := expr.Right.(*BinaryExpr)
	if !ok {
		t.Fatal("Right should be BinaryExpr")
	}
	if rightBinary.Precedence() != token.PrecedenceAND {
		t.Errorf("Right BinaryExpr should have AND precedence, got %d", rightBinary.Precedence())
	}

	// Test positions
	if expr.Start().Line != 1 || expr.Start().Column != 1 {
		t.Errorf("Start position incorrect: %v", expr.Start())
	}
	if expr.End().Line != 1 || expr.End().Column != 40 {
		t.Errorf("End position incorrect: %v", expr.End())
	}
}

// TestIndexExprWithComplexLeft tests index expressions with complex left operands
func TestIndexExprWithComplexLeft(t *testing.T) {
	// Test: obj[key1][key2]
	expr := &IndexExpr{
		Left: &IndexExpr{
			Left: &Ident{
				Token: token.Token{
					Kind:  token.IDENT,
					Start: token.Position{Line: 1, Column: 1},
					End:   token.Position{Line: 1, Column: 4},
				},
				Value: "obj",
			},
			LBrack: token.Token{Kind: token.LBRACK},
			Index: &Literal{
				Token: token.Token{Kind: token.STRING},
				Value: "key1",
			},
			RBrack: token.Token{
				Kind: token.RBRACK,
				End:  token.Position{Line: 1, Column: 11},
			},
		},
		LBrack: token.Token{Kind: token.LBRACK},
		Index: &Literal{
			Token: token.Token{Kind: token.STRING},
			Value: "key2",
		},
		RBrack: token.Token{
			Kind: token.RBRACK,
			End:  token.Position{Line: 1, Column: 18},
		},
	}

	// Verify structure
	innerIndex, ok := expr.Left.(*IndexExpr)
	if !ok {
		t.Fatal("Left should be IndexExpr")
	}

	innerIdent, ok := innerIndex.Left.(*Ident)
	if !ok {
		t.Fatal("Inner left should be Ident")
	}

	if innerIdent.Value != "obj" {
		t.Errorf("Inner ident value = %s, want 'obj'", innerIdent.Value)
	}

	// Test positions
	if expr.Start() != (token.Position{Line: 1, Column: 1}) {
		t.Errorf("Start() = %v, want {1, 1}", expr.Start())
	}
	if expr.End() != (token.Position{Line: 1, Column: 18}) {
		t.Errorf("End() = %v, want {1, 18}", expr.End())
	}
}

// TestLiteralTypeConversions tests all type conversion methods
func TestLiteralTypeConversions(t *testing.T) {
	stringLit := &Literal{
		Token: token.Token{Kind: token.STRING},
		Value: "test",
	}
	numberLit := &Literal{
		Token: token.Token{Kind: token.NUMBER},
		Value: "42",
	}
	trueLit := &Literal{
		Token: token.Token{Kind: token.TRUE},
		Value: "true",
	}

	// Test IsString
	if !stringLit.IsString() {
		t.Error("String literal should return true for IsString()")
	}
	if numberLit.IsString() {
		t.Error("Number literal should return false for IsString()")
	}
	if trueLit.IsString() {
		t.Error("Bool literal should return false for IsString()")
	}

	// Test IsNumber
	if stringLit.IsNumber() {
		t.Error("String literal should return false for IsNumber()")
	}
	if !numberLit.IsNumber() {
		t.Error("Number literal should return true for IsNumber()")
	}
	if trueLit.IsNumber() {
		t.Error("Bool literal should return false for IsNumber()")
	}

	// Test IsBool
	if stringLit.IsBool() {
		t.Error("String literal should return false for IsBool()")
	}
	if numberLit.IsBool() {
		t.Error("Number literal should return false for IsBool()")
	}
	if !trueLit.IsBool() {
		t.Error("Bool literal should return true for IsBool()")
	}

	// Test AsString conversions
	if _, err := stringLit.AsString(); err != nil {
		t.Errorf("String literal AsString() should not error: %v", err)
	}
	if _, err := numberLit.AsString(); err == nil {
		t.Error("Number literal AsString() should error")
	}
	if _, err := trueLit.AsString(); err == nil {
		t.Error("Bool literal AsString() should error")
	}

	// Test AsNumber conversions
	if _, err := stringLit.AsNumber(); err == nil {
		t.Error("String literal AsNumber() should error")
	}
	if _, err := numberLit.AsNumber(); err != nil {
		t.Errorf("Number literal AsNumber() should not error: %v", err)
	}
	if _, err := trueLit.AsNumber(); err == nil {
		t.Error("Bool literal AsNumber() should error")
	}

	// Test AsBool conversions
	if _, err := stringLit.AsBool(); err == nil {
		t.Error("String literal AsBool() should error")
	}
	if _, err := numberLit.AsBool(); err == nil {
		t.Error("Number literal AsBool() should error")
	}
	if _, err := trueLit.AsBool(); err != nil {
		t.Errorf("Bool literal AsBool() should not error: %v", err)
	}
}

// TestListLiteralWithMixedTypes tests list literals containing different types
func TestListLiteralWithMixedTypes(t *testing.T) {
	list := &ListLiteral{
		Lparen: token.Token{
			Start: token.Position{Line: 1, Column: 1},
		},
		Rparen: token.Token{
			End: token.Position{Line: 1, Column: 20},
		},
		Values: []*Literal{
			{Token: token.Token{Kind: token.NUMBER}, Value: "1"},
			{Token: token.Token{Kind: token.STRING}, Value: "hello"},
			{Token: token.Token{Kind: token.TRUE}, Value: "true"},
		},
	}

	if len(list.Values) != 3 {
		t.Errorf("List should have 3 values, got %d", len(list.Values))
	}

	// Check first element is number
	if !list.Values[0].IsNumber() {
		t.Error("First element should be number")
	}

	// Check second element is string
	if !list.Values[1].IsString() {
		t.Error("Second element should be string")
	}

	// Check third element is bool
	if !list.Values[2].IsBool() {
		t.Error("Third element should be bool")
	}

	// Test interface
	var _ Expr = list
	var _ AtomicExpr = list
}

// TestExpressionWithAllOperators tests expressions using all operator types
func TestExpressionWithAllOperators(t *testing.T) {
	operators := []struct {
		kind       token.Kind
		precedence int
	}{
		{token.OR, token.PrecedenceOR},
		{token.AND, token.PrecedenceAND},
		{token.NOT, token.PrecedenceNOT},
		{token.EQ, token.PrecedenceCMP},
		{token.NE, token.PrecedenceCMP},
		{token.LT, token.PrecedenceCMP},
		{token.LE, token.PrecedenceCMP},
		{token.GT, token.PrecedenceCMP},
		{token.GE, token.PrecedenceCMP},
		{token.IN, token.PrecedenceMatch},
		{token.LIKE, token.PrecedenceMatch},
	}

	for _, op := range operators {
		t.Run(op.kind.Name(), func(t *testing.T) {
			if op.kind == token.NOT {
				// Test unary expression
				unary := &UnaryExpr{
					Op: token.Token{Kind: op.kind},
					Right: &BinaryExpr{
						Left:  &Ident{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
						Op:    token.Token{Kind: token.EQ},
						Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
					},
				}
				if unary.Precedence() != op.precedence {
					t.Errorf("UnaryExpr with %s: precedence = %d, want %d",
						op.kind.Name(), unary.Precedence(), op.precedence)
				}
			} else {
				// Test binary expression
				binary := &BinaryExpr{
					Left:  &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
					Op:    token.Token{Kind: op.kind},
					Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 10}}},
				}
				if binary.Precedence() != op.precedence {
					t.Errorf("BinaryExpr with %s: precedence = %d, want %d",
						op.kind.Name(), binary.Precedence(), op.precedence)
				}
			}
		})
	}
}

// TestPositionPropagation tests that positions propagate correctly through the tree
func TestPositionPropagation(t *testing.T) {
	// Build expression: (a > 5) AND (b < 10)
	expr := &BinaryExpr{
		Left: &BinaryExpr{
			Left: &Ident{
				Token: token.Token{
					Kind:  token.IDENT,
					Start: token.Position{Line: 1, Column: 2},
					End:   token.Position{Line: 1, Column: 3},
				},
			},
			Op: token.Token{Kind: token.GT},
			Right: &Literal{
				Token: token.Token{
					Kind:  token.NUMBER,
					Start: token.Position{Line: 1, Column: 6},
					End:   token.Position{Line: 1, Column: 7},
				},
			},
		},
		Op: token.Token{Kind: token.AND},
		Right: &BinaryExpr{
			Left: &Ident{
				Token: token.Token{
					Kind:  token.IDENT,
					Start: token.Position{Line: 1, Column: 15},
					End:   token.Position{Line: 1, Column: 16},
				},
			},
			Op: token.Token{Kind: token.LT},
			Right: &Literal{
				Token: token.Token{
					Kind:  token.NUMBER,
					Start: token.Position{Line: 1, Column: 19},
					End:   token.Position{Line: 1, Column: 21},
				},
			},
		},
	}

	// Check root positions
	rootStart := expr.Start()
	rootEnd := expr.End()

	if rootStart.Line != 1 || rootStart.Column != 2 {
		t.Errorf("Root start position = %v, want {1, 2}", rootStart)
	}

	if rootEnd.Line != 1 || rootEnd.Column != 21 {
		t.Errorf("Root end position = %v, want {1, 21}", rootEnd)
	}

	// Check left subexpression
	leftBinary := expr.Left.(*BinaryExpr)
	leftStart := leftBinary.Start()
	leftEnd := leftBinary.End()

	if leftStart.Line != 1 || leftStart.Column != 2 {
		t.Errorf("Left start position = %v, want {1, 2}", leftStart)
	}

	if leftEnd.Line != 1 || leftEnd.Column != 7 {
		t.Errorf("Left end position = %v, want {1, 7}", leftEnd)
	}

	// Check right subexpression
	rightBinary := expr.Right.(*BinaryExpr)
	rightStart := rightBinary.Start()
	rightEnd := rightBinary.End()

	if rightStart.Line != 1 || rightStart.Column != 15 {
		t.Errorf("Right start position = %v, want {1, 15}", rightStart)
	}

	if rightEnd.Line != 1 || rightEnd.Column != 21 {
		t.Errorf("Right end position = %v, want {1, 21}", rightEnd)
	}
}

// TestEmptyAndNilCases tests edge cases with empty and nil values
func TestEmptyAndNilCases(t *testing.T) {
	// Empty list literal
	emptyList := &ListLiteral{
		Lparen: token.Token{Start: token.Position{Line: 1, Column: 1}},
		Rparen: token.Token{End: token.Position{Line: 1, Column: 3}},
		Values: []*Literal{},
	}

	if len(emptyList.Values) != 0 {
		t.Errorf("Empty list should have 0 values, got %d", len(emptyList.Values))
	}

	// Nil list values
	nilList := &ListLiteral{
		Lparen: token.Token{Start: token.Position{Line: 1, Column: 1}},
		Rparen: token.Token{End: token.Position{Line: 1, Column: 3}},
		Values: nil,
	}

	if nilList.Values != nil {
		// This is expected, but we test that it doesn't panic
		if len(nilList.Values) != 0 {
			t.Errorf("Nil values list should have 0 length")
		}
	}

	// Test IsSameKind with nil
	lit := &Literal{Token: token.Token{Kind: token.STRING}, Value: "test"}
	if lit.IsSameKind(nil) {
		t.Error("IsSameKind should return false for nil argument")
	}
}

// TestComplexPrecedenceScenario tests a very complex precedence scenario
func TestComplexPrecedenceScenario(t *testing.T) {
	// Expression: a OR b AND c > d IN [1,2] OR e LIKE 'test'
	// Expected grouping based on precedence:
	// a OR (b AND (c > (d IN [1,2]))) OR (e LIKE 'test')
	//
	// Precedence order (low to high):
	// OR (1) < AND (2) < NOT (3) < CMP (4) < MATCH (5)

	expr := &BinaryExpr{
		Left: &Ident{
			Token: token.Token{
				Kind:  token.IDENT,
				Start: token.Position{Line: 1, Column: 1},
			},
			Value: "a",
		},
		Op: token.Token{Kind: token.OR}, // Precedence: 1
		Right: &BinaryExpr{
			Left: &BinaryExpr{
				Left: &Ident{
					Token: token.Token{Kind: token.IDENT},
					Value: "b",
				},
				Op: token.Token{Kind: token.AND}, // Precedence: 2
				Right: &BinaryExpr{
					Left: &Ident{
						Token: token.Token{Kind: token.IDENT},
						Value: "c",
					},
					Op: token.Token{Kind: token.GT}, // Precedence: 4
					Right: &BinaryExpr{
						Left: &Ident{
							Token: token.Token{Kind: token.IDENT},
							Value: "d",
						},
						Op: token.Token{Kind: token.IN}, // Precedence: 5
						Right: &ListLiteral{
							Lparen: token.Token{},
							Rparen: token.Token{},
							Values: []*Literal{
								{Token: token.Token{Kind: token.NUMBER}, Value: "1"},
								{Token: token.Token{Kind: token.NUMBER}, Value: "2"},
							},
						},
					},
				},
			},
			Op: token.Token{Kind: token.OR}, // Precedence: 1
			Right: &BinaryExpr{
				Left: &Ident{
					Token: token.Token{Kind: token.IDENT},
					Value: "e",
				},
				Op: token.Token{Kind: token.LIKE}, // Precedence: 5
				Right: &Literal{
					Token: token.Token{
						Kind: token.STRING,
						End:  token.Position{Line: 1, Column: 50},
					},
					Value: "test",
				},
			},
		},
	}

	// Verify root is OR
	if expr.Precedence() != token.PrecedenceOR {
		t.Errorf("Root precedence = %d, want %d (OR)", expr.Precedence(), token.PrecedenceOR)
	}

	// Verify right side structure
	rightOr, ok := expr.Right.(*BinaryExpr)
	if !ok {
		t.Fatal("Right should be BinaryExpr")
	}
	if rightOr.Op.Kind != token.OR {
		t.Errorf("Right operator = %s, want OR", rightOr.Op.Kind.Name())
	}

	// Verify AND expression
	leftAnd, ok := rightOr.Left.(*BinaryExpr)
	if !ok {
		t.Fatal("Right.Left should be BinaryExpr (AND)")
	}
	if leftAnd.Op.Kind != token.AND {
		t.Errorf("Right.Left operator = %s, want AND", leftAnd.Op.Kind.Name())
	}

	// Verify GT expression
	gtExpr, ok := leftAnd.Right.(*BinaryExpr)
	if !ok {
		t.Fatal("AND.Right should be BinaryExpr (GT)")
	}
	if gtExpr.Op.Kind != token.GT {
		t.Errorf("GT operator = %s, want GT", gtExpr.Op.Kind.Name())
	}

	// Verify IN expression (highest precedence comparison)
	inExpr, ok := gtExpr.Right.(*BinaryExpr)
	if !ok {
		t.Fatal("GT.Right should be BinaryExpr (IN)")
	}
	if inExpr.Op.Kind != token.IN {
		t.Errorf("IN operator = %s, want IN", inExpr.Op.Kind.Name())
	}
	if inExpr.Precedence() != token.PrecedenceMatch {
		t.Errorf("IN precedence = %d, want %d", inExpr.Precedence(), token.PrecedenceMatch)
	}

	// Verify LIKE expression
	likeExpr, ok := rightOr.Right.(*BinaryExpr)
	if !ok {
		t.Fatal("Right.Right should be BinaryExpr (LIKE)")
	}
	if likeExpr.Op.Kind != token.LIKE {
		t.Errorf("LIKE operator = %s, want LIKE", likeExpr.Op.Kind.Name())
	}
	if likeExpr.Precedence() != token.PrecedenceMatch {
		t.Errorf("LIKE precedence = %d, want %d", likeExpr.Precedence(), token.PrecedenceMatch)
	}
}

// BenchmarkLiteralAsNumber benchmarks number conversion
func BenchmarkLiteralAsNumber(b *testing.B) {
	lit := &Literal{
		Token: token.Token{Kind: token.NUMBER},
		Value: "3.14159",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lit.AsNumber()
	}
}

// BenchmarkLiteralIsSameKind benchmarks kind comparison
func BenchmarkLiteralIsSameKind(b *testing.B) {
	lit1 := &Literal{
		Token: token.Token{Kind: token.STRING},
		Value: "hello",
	}

	lit2 := &Literal{
		Token: token.Token{Kind: token.STRING},
		Value: "world",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = lit1.IsSameKind(lit2)
	}
}

// BenchmarkBinaryExprPrecedence benchmarks precedence checking
func BenchmarkBinaryExprPrecedence(b *testing.B) {
	expr := &BinaryExpr{
		Left: &BinaryExpr{
			Op:    token.Token{Kind: token.OR},
			Left:  &Ident{},
			Right: &Ident{},
		},
		Op: token.Token{Kind: token.AND},
		Right: &BinaryExpr{
			Op:    token.Token{Kind: token.EQ},
			Left:  &Ident{},
			Right: &Literal{},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = expr.IsLeftLower()
		_ = expr.IsRightLower()
	}
}

// BenchmarkComplexExpressionTraversal benchmarks traversing a complex expression
func BenchmarkComplexExpressionTraversal(b *testing.B) {
	expr := &BinaryExpr{
		Left: &BinaryExpr{
			Left: &BinaryExpr{
				Left:  &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 1}}},
				Op:    token.Token{Kind: token.GT},
				Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 5}}},
			},
			Op: token.Token{Kind: token.AND},
			Right: &BinaryExpr{
				Left:  &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 10}}},
				Op:    token.Token{Kind: token.LT},
				Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 15}}},
			},
		},
		Op: token.Token{Kind: token.OR},
		Right: &BinaryExpr{
			Left:  &Ident{Token: token.Token{Start: token.Position{Line: 1, Column: 20}}},
			Op:    token.Token{Kind: token.EQ},
			Right: &Literal{Token: token.Token{End: token.Position{Line: 1, Column: 25}}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = expr.Start()
		_ = expr.End()
		_ = expr.Precedence()
		_ = expr.IsLeftLower()
		_ = expr.IsRightLower()
	}
}
