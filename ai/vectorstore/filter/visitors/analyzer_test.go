package visitors_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/visitors"
)

// TestNewAnalyzer tests the creation of a new Analyzer instance
func TestNewAnalyzer(t *testing.T) {
	analyzer := visitors.NewAnalyzer()
	if analyzer == nil {
		t.Fatal("NewAnalyzer should return non-nil analyzer")
	}

	if analyzer.Error() != nil {
		t.Errorf("New analyzer should have no error, got: %v", analyzer.Error())
	}
}

// TestAnalyzerError tests the Error method
func TestAnalyzerError(t *testing.T) {
	analyzer := visitors.NewAnalyzer()

	// Initially no error
	if analyzer.Error() != nil {
		t.Error("New analyzer should have no error")
	}

	// After visiting an invalid expression
	analyzer.Visit(nil)
	if analyzer.Error() == nil {
		t.Error("Expected error after visiting nil expression")
	}
}

// TestAnalyzeValidIdentifier tests valid identifier analysis
func TestAnalyzeValidIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
	}{
		{"Simple identifier", "username"},
		{"Identifier with underscore", "user_name"},
		{"Identifier with number", "field1"},
		{"CamelCase identifier", "firstName"},
		{"All caps identifier", "STATUS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := &ast.Ident{
				Token: token.Token{
					Kind:    token.IDENT,
					Literal: tt.identifier,
				},
				Value: tt.identifier,
			}

			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(expr)

			if analyzer.Error() != nil {
				t.Errorf("Expected no error for valid identifier '%s', got: %v",
					tt.identifier, analyzer.Error())
			}
		})
	}
}

// TestAnalyzeInvalidIdentifier tests invalid identifier analysis
func TestAnalyzeInvalidIdentifier(t *testing.T) {
	tests := []struct {
		name        string
		expr        *ast.Ident
		expectError string
	}{
		{
			name: "Reserved keyword as identifier",
			expr: &ast.Ident{
				Token: token.Token{
					Kind:    token.AND,
					Literal: "AND",
				},
				Value: "AND",
			},
			expectError: "expected identifier token",
		},
		{
			name: "Wrong token kind",
			expr: &ast.Ident{
				Token: token.Token{
					Kind:    token.NUMBER,
					Literal: "123",
				},
				Value: "123",
			},
			expectError: "expected identifier token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(tt.expr)

			if analyzer.Error() == nil {
				t.Error("Expected error but got none")
				return
			}

			if !strings.Contains(analyzer.Error().Error(), tt.expectError) {
				t.Errorf("Expected error containing '%s', got: %v",
					tt.expectError, analyzer.Error())
			}
		})
	}
}

// TestAnalyzeValidLiterals tests valid literal analysis
func TestAnalyzeValidLiterals(t *testing.T) {
	tests := []struct {
		name    string
		literal *ast.Literal
	}{
		{
			name: "String literal",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.STRING, Literal: "hello"},
				Value: "hello",
			},
		},
		{
			name: "Number literal",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.NUMBER, Literal: "42"},
				Value: "42",
			},
		},
		{
			name: "Float literal",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.NUMBER, Literal: "3.14"},
				Value: "3.14",
			},
		},
		{
			name: "Boolean true",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.TRUE, Literal: "true"},
				Value: "true",
			},
		},
		{
			name: "Boolean false",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.FALSE, Literal: "false"},
				Value: "false",
			},
		},
		{
			name: "Negative number",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.NUMBER, Literal: "-100"},
				Value: "-100",
			},
		},
		{
			name: "Empty string",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.STRING, Literal: ""},
				Value: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(tt.literal)

			if analyzer.Error() != nil {
				t.Errorf("Expected no error for valid literal, got: %v", analyzer.Error())
			}
		})
	}
}

// TestAnalyzeInvalidLiterals tests invalid literal analysis
func TestAnalyzeInvalidLiterals(t *testing.T) {
	tests := []struct {
		name        string
		literal     *ast.Literal
		expectError string
	}{
		{
			name: "Invalid number format",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.NUMBER, Literal: "abc"},
				Value: "abc",
			},
			expectError: "invalid number literal",
		},
		{
			name: "Invalid boolean",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.TRUE, Literal: "maybe"},
				Value: "maybe",
			},
			expectError: "invalid boolean literal",
		},
		{
			name: "Unsupported literal type",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.IDENT, Literal: "test"},
				Value: "test",
			},
			expectError: "unsupported literal type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(tt.literal)

			if analyzer.Error() == nil {
				t.Error("Expected error but got none")
				return
			}

			if !strings.Contains(analyzer.Error().Error(), tt.expectError) {
				t.Errorf("Expected error containing '%s', got: %v",
					tt.expectError, analyzer.Error())
			}
		})
	}
}

// TestAnalyzeValidListLiterals tests valid list literal analysis
func TestAnalyzeValidListLiterals(t *testing.T) {
	tests := []struct {
		name string
		list *ast.ListLiteral
	}{
		{
			name: "Number list",
			list: &ast.ListLiteral{
				Values: []*ast.Literal{
					{Token: token.Token{Kind: token.NUMBER, Literal: "1"}, Value: "1"},
					{Token: token.Token{Kind: token.NUMBER, Literal: "2"}, Value: "2"},
					{Token: token.Token{Kind: token.NUMBER, Literal: "3"}, Value: "3"},
				},
			},
		},
		{
			name: "String list",
			list: &ast.ListLiteral{
				Values: []*ast.Literal{
					{Token: token.Token{Kind: token.STRING, Literal: "a"}, Value: "a"},
					{Token: token.Token{Kind: token.STRING, Literal: "b"}, Value: "b"},
				},
			},
		},
		{
			name: "Boolean list",
			list: &ast.ListLiteral{
				Values: []*ast.Literal{
					{Token: token.Token{Kind: token.TRUE, Literal: "true"}, Value: "true"},
					{Token: token.Token{Kind: token.FALSE, Literal: "false"}, Value: "false"},
				},
			},
		},
		{
			name: "Single element list",
			list: &ast.ListLiteral{
				Values: []*ast.Literal{
					{Token: token.Token{Kind: token.NUMBER, Literal: "42"}, Value: "42"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(tt.list)

			if analyzer.Error() != nil {
				t.Errorf("Expected no error for valid list, got: %v", analyzer.Error())
			}
		})
	}
}

// TestAnalyzeInvalidListLiterals tests invalid list literal analysis
func TestAnalyzeInvalidListLiterals(t *testing.T) {
	tests := []struct {
		name        string
		list        *ast.ListLiteral
		expectError string
	}{
		{
			name: "Empty list",
			list: &ast.ListLiteral{
				Values: []*ast.Literal{},
			},
			expectError: "list literal cannot be empty",
		},
		{
			name: "Mixed types - number and string",
			list: &ast.ListLiteral{
				Values: []*ast.Literal{
					{Token: token.Token{Kind: token.NUMBER, Literal: "1"}, Value: "1"},
					{Token: token.Token{Kind: token.STRING, Literal: "a"}, Value: "a"},
				},
			},
			expectError: "all elements must have same type",
		},
		{
			name: "Mixed types - number and boolean",
			list: &ast.ListLiteral{
				Values: []*ast.Literal{
					{Token: token.Token{Kind: token.NUMBER, Literal: "1"}, Value: "1"},
					{Token: token.Token{Kind: token.TRUE, Literal: "true"}, Value: "true"},
				},
			},
			expectError: "all elements must have same type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(tt.list)

			if analyzer.Error() == nil {
				t.Error("Expected error but got none")
				return
			}

			if !strings.Contains(analyzer.Error().Error(), tt.expectError) {
				t.Errorf("Expected error containing '%s', got: %v",
					tt.expectError, analyzer.Error())
			}
		})
	}
}

// TestAnalyzeValidUnaryExpressions tests valid unary expression analysis
func TestAnalyzeValidUnaryExpressions(t *testing.T) {
	tests := []struct {
		name string
		expr *ast.UnaryExpr
	}{
		{
			name: "NOT with binary expression",
			expr: &ast.UnaryExpr{
				Op: token.Token{Kind: token.NOT, Literal: "NOT"},
				Right: &ast.BinaryExpr{
					Left:  &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "x"}, Value: "x"},
					Op:    token.Token{Kind: token.EQ, Literal: "=="},
					Right: &ast.Literal{Token: token.Token{Kind: token.NUMBER, Literal: "5"}, Value: "5"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(tt.expr)

			if analyzer.Error() != nil {
				t.Errorf("Expected no error for valid unary expression, got: %v", analyzer.Error())
			}
		})
	}
}

// TestAnalyzeInvalidUnaryExpressions tests invalid unary expression analysis
func TestAnalyzeInvalidUnaryExpressions(t *testing.T) {
	tests := []struct {
		name        string
		expr        *ast.UnaryExpr
		expectError string
	}{
		{
			name: "Invalid unary operator",
			expr: &ast.UnaryExpr{
				Op: token.Token{Kind: token.EQ, Literal: "=="},
				Right: &ast.BinaryExpr{
					Left:  &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "x"}, Value: "x"},
					Op:    token.Token{Kind: token.EQ, Literal: "=="},
					Right: &ast.Literal{Token: token.Token{Kind: token.NUMBER, Literal: "5"}, Value: "5"},
				},
			},
			expectError: "unsupported unary operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(tt.expr)

			if analyzer.Error() == nil {
				t.Error("Expected error but got none")
				return
			}

			if !strings.Contains(analyzer.Error().Error(), tt.expectError) {
				t.Errorf("Expected error containing '%s', got: %v",
					tt.expectError, analyzer.Error())
			}
		})
	}
}

// TestAnalyzeValidBinaryExpressions tests valid binary expression analysis
func TestAnalyzeValidBinaryExpressions(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
	}{
		{
			name: "Equality with identifier and literal",
			expr: filter.EQ("name", "John"),
		},
		{
			name: "Inequality with identifier and literal",
			expr: filter.NE("status", "inactive"),
		},
		{
			name: "Less than with number",
			expr: filter.LT("age", filter.NewLiteral(18)),
		},
		{
			name: "Greater than with number",
			expr: filter.GT("score", filter.NewLiteral(100)),
		},
		{
			name: "Less or equal with number",
			expr: filter.LE("count", filter.NewLiteral(50)),
		},
		{
			name: "Greater or equal with number",
			expr: filter.GE("balance", filter.NewLiteral(1000)),
		},
		{
			name: "IN with string list",
			expr: filter.In("tier", []string{"gold", "platinum"}),
		},
		{
			name: "IN with number list",
			expr: filter.In("id", []int{1, 2, 3}),
		},
		{
			name: "LIKE with string pattern",
			expr: filter.Like("email", "%@example.com"),
		},
		{
			name: "AND with two comparisons",
			expr: filter.And(
				filter.EQ("status", "active"),
				filter.GT("age", filter.NewLiteral(18)),
			),
		},
		{
			name: "OR with two comparisons",
			expr: filter.Or(
				filter.EQ("type", "A"),
				filter.EQ("type", "B"),
			),
		},
		{
			name: "Complex nested expression",
			expr: filter.And(
				filter.Or(
					filter.EQ("status", "active"),
					filter.EQ("status", "pending"),
				),
				filter.GT("age", filter.NewLiteral(18)),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(tt.expr)

			if analyzer.Error() != nil {
				t.Errorf("Expected no error for valid binary expression, got: %v", analyzer.Error())
			}
		})
	}
}

// TestAnalyzeInvalidBinaryExpressions tests invalid binary expression analysis
func TestAnalyzeInvalidBinaryExpressions(t *testing.T) {
	tests := []struct {
		name        string
		expr        *ast.BinaryExpr
		expectError string
	}{
		{
			name: "EQ with non-literal right operand",
			expr: &ast.BinaryExpr{
				Left:  &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "x"}, Value: "x"},
				Op:    token.Token{Kind: token.EQ, Literal: "=="},
				Right: &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "y"}, Value: "y"},
			},
			expectError: "requires literal value on right side",
		},
		{
			name: "GT with non-numeric right operand",
			expr: &ast.BinaryExpr{
				Left:  &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "x"}, Value: "x"},
				Op:    token.Token{Kind: token.GT, Literal: ">"},
				Right: &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "abc"}, Value: "abc"},
			},
			expectError: "requires numeric literal on right side",
		},
		{
			name: "LIKE with non-string right operand",
			expr: &ast.BinaryExpr{
				Left:  &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "x"}, Value: "x"},
				Op:    token.Token{Kind: token.LIKE, Literal: "LIKE"},
				Right: &ast.Literal{Token: token.Token{Kind: token.NUMBER, Literal: "123"}, Value: "123"},
			},
			expectError: "requires string literal on right side",
		},
		{
			name: "AND with non-computed left operand",
			expr: &ast.BinaryExpr{
				Left:  &ast.Literal{Token: token.Token{Kind: token.NUMBER, Literal: "5"}, Value: "5"},
				Op:    token.Token{Kind: token.AND, Literal: "AND"},
				Right: &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "x"}, Value: "x"},
			},
			expectError: "requires computed expression on left side",
		},
		{
			name: "Invalid left operand for comparison",
			expr: &ast.BinaryExpr{
				Left:  &ast.Literal{Token: token.Token{Kind: token.NUMBER, Literal: "5"}, Value: "5"},
				Op:    token.Token{Kind: token.EQ, Literal: "=="},
				Right: &ast.Literal{Token: token.Token{Kind: token.NUMBER, Literal: "10"}, Value: "10"},
			},
			expectError: "requires identifier or index expression on left side",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(tt.expr)

			if analyzer.Error() == nil {
				t.Error("Expected error but got none")
				return
			}

			if !strings.Contains(analyzer.Error().Error(), tt.expectError) {
				t.Errorf("Expected error containing '%s', got: %v",
					tt.expectError, analyzer.Error())
			}
		})
	}
}

// TestAnalyzeValidIndexExpressions tests valid index expression analysis
func TestAnalyzeValidIndexExpressions(t *testing.T) {
	tests := []struct {
		name string
		expr *ast.IndexExpr
	}{
		{
			name: "Array numeric index",
			expr: &ast.IndexExpr{
				Left:  &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "arr"}, Value: "arr"},
				Index: &ast.Literal{Token: token.Token{Kind: token.NUMBER, Literal: "0"}, Value: "0"},
			},
		},
		{
			name: "Object string index",
			expr: &ast.IndexExpr{
				Left:  &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "obj"}, Value: "obj"},
				Index: &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "key"}, Value: "key"},
			},
		},
		{
			name: "Nested index expression",
			expr: &ast.IndexExpr{
				Left: &ast.IndexExpr{
					Left:  &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "data"}, Value: "data"},
					Index: &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "users"}, Value: "users"},
				},
				Index: &ast.Literal{Token: token.Token{Kind: token.NUMBER, Literal: "0"}, Value: "0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(tt.expr)

			if analyzer.Error() != nil {
				t.Errorf("Expected no error for valid index expression, got: %v", analyzer.Error())
			}
		})
	}
}

// TestAnalyzeInvalidIndexExpressions tests invalid index expression analysis
func TestAnalyzeInvalidIndexExpressions(t *testing.T) {
	tests := []struct {
		name        string
		expr        *ast.IndexExpr
		expectError string
	}{
		{
			name: "Invalid left operand type",
			expr: &ast.IndexExpr{
				Left:  &ast.Literal{Token: token.Token{Kind: token.NUMBER, Literal: "5"}, Value: "5"},
				Index: &ast.Literal{Token: token.Token{Kind: token.NUMBER, Literal: "0"}, Value: "0"},
			},
			expectError: "requires identifier or index expression on left side",
		},
		{
			name: "Boolean index",
			expr: &ast.IndexExpr{
				Left:  &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "arr"}, Value: "arr"},
				Index: &ast.Literal{Token: token.Token{Kind: token.TRUE, Literal: "true"}, Value: "true"},
			},
			expectError: "index must be number or string literal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(tt.expr)

			if analyzer.Error() == nil {
				t.Error("Expected error but got none")
				return
			}

			if !strings.Contains(analyzer.Error().Error(), tt.expectError) {
				t.Errorf("Expected error containing '%s', got: %v",
					tt.expectError, analyzer.Error())
			}
		})
	}
}

// TestAnalyzeNilExpression tests nil expression handling
func TestAnalyzeNilExpression(t *testing.T) {
	analyzer := visitors.NewAnalyzer()
	analyzer.Visit(nil)

	if analyzer.Error() == nil {
		t.Error("Expected error for nil expression")
		return
	}

	if !strings.Contains(analyzer.Error().Error(), "expression cannot be nil") {
		t.Errorf("Expected 'expression cannot be nil' error, got: %v", analyzer.Error())
	}
}

// TestAnalyzeComplexExpressions tests complex real-world expressions
func TestAnalyzeComplexExpressions(t *testing.T) {
	tests := []struct {
		name        string
		expr        ast.Expr
		shouldError bool
	}{
		{
			name: "Complex valid expression",
			expr: filter.Or(
				filter.And(
					filter.EQ("user_type", "individual"),
					filter.Or(
						filter.And(
							filter.GE("age", filter.NewLiteral(14)),
							filter.Like("name", "%tom"),
						),
						filter.EQ("verified", true),
					),
				),
				filter.And(
					filter.Not(filter.EQ("status", "suspended")),
					filter.In("tier", []string{"gold", "platinum"}),
				),
			),
			shouldError: false,
		},
		{
			name: "Deeply nested valid expression",
			expr: filter.And(
				filter.Or(
					filter.And(
						filter.EQ("a", "1"),
						filter.EQ("b", "2"),
					),
					filter.And(
						filter.EQ("c", "3"),
						filter.EQ("d", "4"),
					),
				),
				filter.Or(
					filter.GT("x", filter.NewLiteral(10)),
					filter.LT("y", filter.NewLiteral(20)),
				),
			),
			shouldError: false,
		},
		{
			name: "Multiple NOT operators",
			expr: filter.And(
				filter.Not(filter.EQ("blocked", true)),
				filter.Not(filter.EQ("deleted", true)),
			),
			shouldError: false,
		},
		{
			name: "Multiple IN operators",
			expr: filter.And(
				filter.In("category", []string{"A", "B", "C"}),
				filter.In("priority", []int{1, 2, 3}),
			),
			shouldError: false,
		},
		{
			name: "Mixed comparison operators",
			expr: filter.And(
				filter.GE("score", filter.NewLiteral(50)),
				filter.LE("score", filter.NewLiteral(100)),
			),
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(tt.expr)

			if tt.shouldError {
				if analyzer.Error() == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if analyzer.Error() != nil {
					t.Errorf("Expected no error, got: %v", analyzer.Error())
				}
			}
		})
	}
}

// TestAnalyzeWithIndexExpressions tests expressions with index access
func TestAnalyzeWithIndexExpressions(t *testing.T) {
	tests := []struct {
		name        string
		buildExpr   func() ast.Expr
		shouldError bool
	}{
		{
			name: "Simple index in comparison",
			buildExpr: func() ast.Expr {
				return &ast.BinaryExpr{
					Left: &ast.IndexExpr{
						Left:  &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "data"}, Value: "data"},
						Index: &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "name"}, Value: "name"},
					},
					Op:    token.Token{Kind: token.EQ, Literal: "=="},
					Right: &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "test"}, Value: "test"},
				}
			},
			shouldError: false,
		},
		{
			name: "Nested index in comparison",
			buildExpr: func() ast.Expr {
				return &ast.BinaryExpr{
					Left: &ast.IndexExpr{
						Left: &ast.IndexExpr{
							Left:  &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "config"}, Value: "config"},
							Index: &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "settings"}, Value: "settings"},
						},
						Index: &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "enabled"}, Value: "enabled"},
					},
					Op:    token.Token{Kind: token.EQ, Literal: "=="},
					Right: &ast.Literal{Token: token.Token{Kind: token.TRUE, Literal: "true"}, Value: "true"},
				}
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := tt.buildExpr()
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(expr)

			if tt.shouldError {
				if analyzer.Error() == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if analyzer.Error() != nil {
					t.Errorf("Expected no error, got: %v", analyzer.Error())
				}
			}
		})
	}
}

// TestAnalyzerErrorPersistence tests that analyzer maintains error state
func TestAnalyzerErrorPersistence(t *testing.T) {
	analyzer := visitors.NewAnalyzer()

	// First visit with invalid expression
	analyzer.Visit(nil)
	firstError := analyzer.Error()

	if firstError == nil {
		t.Fatal("Expected error after first visit")
	}

	// Second visit with valid expression
	validExpr := filter.EQ("name", "test")
	analyzer.Visit(validExpr)
	secondError := analyzer.Error()

	// Error should be updated to nil or new error
	if secondError != nil && errors.Is(secondError, firstError) {
		t.Error("Error should be updated after second visit")
	}
}

// TestAnalyzeAllOperatorTypes tests all supported operator types
func TestAnalyzeAllOperatorTypes(t *testing.T) {
	operators := []struct {
		name        string
		buildExpr   func() ast.Expr
		shouldError bool
	}{
		{
			name:        "EQ operator",
			buildExpr:   func() ast.Expr { return filter.EQ("x", "value") },
			shouldError: false,
		},
		{
			name:        "NE operator",
			buildExpr:   func() ast.Expr { return filter.NE("x", "value") },
			shouldError: false,
		},
		{
			name:        "LT operator",
			buildExpr:   func() ast.Expr { return filter.LT("x", filter.NewLiteral(10)) },
			shouldError: false,
		},
		{
			name:        "LE operator",
			buildExpr:   func() ast.Expr { return filter.LE("x", filter.NewLiteral(10)) },
			shouldError: false,
		},
		{
			name:        "GT operator",
			buildExpr:   func() ast.Expr { return filter.GT("x", filter.NewLiteral(10)) },
			shouldError: false,
		},
		{
			name:        "GE operator",
			buildExpr:   func() ast.Expr { return filter.GE("x", filter.NewLiteral(10)) },
			shouldError: false,
		},
		{
			name:        "IN operator",
			buildExpr:   func() ast.Expr { return filter.In("x", []string{"a", "b"}) },
			shouldError: false,
		},
		{
			name:        "LIKE operator",
			buildExpr:   func() ast.Expr { return filter.Like("x", "%pattern%") },
			shouldError: false,
		},
		{
			name: "AND operator",
			buildExpr: func() ast.Expr {
				return filter.And(
					filter.EQ("x", "1"),
					filter.EQ("y", "2"),
				)
			},
			shouldError: false,
		},
		{
			name: "OR operator",
			buildExpr: func() ast.Expr {
				return filter.Or(
					filter.EQ("x", "1"),
					filter.EQ("y", "2"),
				)
			},
			shouldError: false,
		},
		{
			name:        "NOT operator",
			buildExpr:   func() ast.Expr { return filter.Not(filter.EQ("x", "value")) },
			shouldError: false,
		},
	}

	for _, op := range operators {
		t.Run(op.name, func(t *testing.T) {
			expr := op.buildExpr()
			analyzer := visitors.NewAnalyzer()
			analyzer.Visit(expr)

			if op.shouldError {
				if analyzer.Error() == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if analyzer.Error() != nil {
					t.Errorf("Expected no error, got: %v", analyzer.Error())
				}
			}
		})
	}
}

// BenchmarkAnalyzeSimpleExpression benchmarks simple expression analysis
func BenchmarkAnalyzeSimpleExpression(b *testing.B) {
	expr := filter.EQ("name", "John")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzer := visitors.NewAnalyzer()
		analyzer.Visit(expr)
	}
}

// BenchmarkAnalyzeComplexExpression benchmarks complex expression analysis
func BenchmarkAnalyzeComplexExpression(b *testing.B) {
	expr := filter.Or(
		filter.And(
			filter.EQ("user_type", "individual"),
			filter.Or(
				filter.And(
					filter.GE("age", filter.NewLiteral(14)),
					filter.Like("name", "%tom"),
				),
				filter.EQ("verified", true),
			),
		),
		filter.And(
			filter.Not(filter.EQ("status", "suspended")),
			filter.In("tier", []string{"gold", "platinum"}),
		),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzer := visitors.NewAnalyzer()
		analyzer.Visit(expr)
	}
}

// BenchmarkAnalyzeDeeplyNestedExpression benchmarks deeply nested expression analysis
func BenchmarkAnalyzeDeeplyNestedExpression(b *testing.B) {
	expr := filter.And(
		filter.And(
			filter.And(
				filter.EQ("a", "1"),
				filter.EQ("b", "2"),
			),
			filter.And(
				filter.EQ("c", "3"),
				filter.EQ("d", "4"),
			),
		),
		filter.And(
			filter.And(
				filter.EQ("e", "5"),
				filter.EQ("f", "6"),
			),
			filter.And(
				filter.EQ("g", "7"),
				filter.EQ("h", "8"),
			),
		),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzer := visitors.NewAnalyzer()
		analyzer.Visit(expr)
	}
}
