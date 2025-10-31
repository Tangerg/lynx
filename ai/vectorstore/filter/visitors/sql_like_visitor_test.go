package visitors_test

import (
	"strings"
	"testing"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/visitors"
)

// TestNewSQLLikeVisitor tests the creation of a new SQLLikeVisitor instance
func TestNewSQLLikeVisitor(t *testing.T) {
	visitor := visitors.NewSQLLikeVisitor()
	if visitor == nil {
		t.Fatal("NewSQLLikeVisitor should return non-nil visitor")
	}

	if visitor.Error() != nil {
		t.Errorf("New visitor should have no error, got: %v", visitor.Error())
	}

	if visitor.SQL() != "" {
		t.Errorf("New visitor should have empty SQL, got: %s", visitor.SQL())
	}
}

// TestSQLLikeVisitorError tests the Error method
func TestSQLLikeVisitorError(t *testing.T) {
	visitor := visitors.NewSQLLikeVisitor()

	// Initially no error
	if visitor.Error() != nil {
		t.Error("New visitor should have no error")
	}

	// After visiting nil expression
	visitor.Visit(nil)
	if visitor.Error() == nil {
		t.Error("Expected error after visiting nil expression")
	}

	if !strings.Contains(visitor.Error().Error(), "expression is nil") {
		t.Errorf("Expected 'expression is nil' error, got: %v", visitor.Error())
	}
}

// TestSQLLikeVisitorSQL tests the SQL method
func TestSQLLikeVisitorSQL(t *testing.T) {
	visitor := visitors.NewSQLLikeVisitor()

	// Initially empty
	if visitor.SQL() != "" {
		t.Errorf("Expected empty SQL, got: %s", visitor.SQL())
	}

	// After visiting an expression
	expr := filter.EQ("name", "John")
	visitor.Visit(expr)

	sql := visitor.SQL()
	if sql == "" {
		t.Error("Expected non-empty SQL after visiting expression")
	}

	expected := "name == 'John'"
	if sql != expected {
		t.Errorf("Expected SQL '%s', got '%s'", expected, sql)
	}
}

// TestVisitIdentifier tests identifier conversion to SQL
func TestVisitIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		ident    string
		expected string
	}{
		{"Simple identifier", "username", "username"},
		{"Identifier with underscore", "user_name", "user_name"},
		{"CamelCase identifier", "firstName", "firstName"},
		{"Uppercase identifier", "STATUS", "STATUS"},
		{"Single letter", "x", "x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := &ast.Ident{
				Token: token.Token{Kind: token.IDENT, Literal: tt.ident},
				Value: tt.ident,
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestVisitLiterals tests literal conversion to SQL
func TestVisitLiterals(t *testing.T) {
	tests := []struct {
		name     string
		literal  *ast.Literal
		expected string
	}{
		{
			name: "String literal",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.STRING, Literal: "hello"},
				Value: "hello",
			},
			expected: "'hello'",
		},
		{
			name: "Number literal",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.NUMBER, Literal: "42"},
				Value: "42",
			},
			expected: "42",
		},
		{
			name: "Float literal",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.NUMBER, Literal: "3.14"},
				Value: "3.14",
			},
			expected: "3.14",
		},
		{
			name: "Boolean true",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.TRUE, Literal: "true"},
				Value: "true",
			},
			expected: "true",
		},
		{
			name: "Boolean false",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.FALSE, Literal: "false"},
				Value: "false",
			},
			expected: "false",
		},
		{
			name: "Empty string",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.STRING, Literal: ""},
				Value: "",
			},
			expected: "''",
		},
		{
			name: "Negative number",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.NUMBER, Literal: "-100"},
				Value: "-100",
			},
			expected: "-100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(tt.literal)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestVisitListLiterals tests list literal conversion to SQL
func TestVisitListLiterals(t *testing.T) {
	tests := []struct {
		name     string
		list     *ast.ListLiteral
		expected string
	}{
		{
			name: "Number list",
			list: &ast.ListLiteral{
				Lparen: token.Token{Literal: "("},
				Rparen: token.Token{Literal: ")"},
				Values: []*ast.Literal{
					{Token: token.Token{Kind: token.NUMBER, Literal: "1"}, Value: "1"},
					{Token: token.Token{Kind: token.NUMBER, Literal: "2"}, Value: "2"},
					{Token: token.Token{Kind: token.NUMBER, Literal: "3"}, Value: "3"},
				},
			},
			expected: "(1,2,3)",
		},
		{
			name: "String list",
			list: &ast.ListLiteral{
				Lparen: token.Token{Literal: "("},
				Rparen: token.Token{Literal: ")"},
				Values: []*ast.Literal{
					{Token: token.Token{Kind: token.STRING, Literal: "a"}, Value: "a"},
					{Token: token.Token{Kind: token.STRING, Literal: "b"}, Value: "b"},
				},
			},
			expected: "('a','b')",
		},
		{
			name: "Boolean list",
			list: &ast.ListLiteral{
				Lparen: token.Token{Literal: "("},
				Rparen: token.Token{Literal: ")"},
				Values: []*ast.Literal{
					{Token: token.Token{Kind: token.TRUE, Literal: "true"}, Value: "true"},
					{Token: token.Token{Kind: token.FALSE, Literal: "false"}, Value: "false"},
				},
			},
			expected: "(true,false)",
		},
		{
			name: "Single element list",
			list: &ast.ListLiteral{
				Lparen: token.Token{Literal: "("},
				Rparen: token.Token{Literal: ")"},
				Values: []*ast.Literal{
					{Token: token.Token{Kind: token.NUMBER, Literal: "42"}, Value: "42"},
				},
			},
			expected: "(42)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(tt.list)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestVisitUnaryExpressions tests unary expression conversion to SQL
func TestVisitUnaryExpressions(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		expected string
	}{
		{
			name:     "NOT with comparison",
			expr:     filter.Not(filter.EQ("status", "inactive")),
			expected: "not (status == 'inactive')",
		},
		{
			name: "NOT with AND",
			expr: filter.Not(filter.And(
				filter.EQ("a", "1"),
				filter.EQ("b", "2"),
			)),
			expected: "not (a == '1' and b == '2')",
		},
		{
			name: "NOT with OR",
			expr: filter.Not(filter.Or(
				filter.EQ("x", "1"),
				filter.EQ("y", "2"),
			)),
			expected: "not (x == '1' or y == '2')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(tt.expr)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestVisitBinaryExpressions tests binary expression conversion to SQL
func TestVisitBinaryExpressions(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		expected string
	}{
		{
			name:     "Simple equality",
			expr:     filter.EQ("name", "John"),
			expected: "name == 'John'",
		},
		{
			name:     "Inequality",
			expr:     filter.NE("status", "inactive"),
			expected: "status != 'inactive'",
		},
		{
			name:     "Less than",
			expr:     filter.LT("age", filter.NewLiteral(18)),
			expected: "age < 18",
		},
		{
			name:     "Less or equal",
			expr:     filter.LE("count", filter.NewLiteral(10)),
			expected: "count <= 10",
		},
		{
			name:     "Greater than",
			expr:     filter.GT("score", filter.NewLiteral(100)),
			expected: "score > 100",
		},
		{
			name:     "Greater or equal",
			expr:     filter.GE("balance", filter.NewLiteral(1000)),
			expected: "balance >= 1000",
		},
		{
			name:     "IN with strings",
			expr:     filter.In("tier", []string{"gold", "platinum"}),
			expected: "tier in ('gold','platinum')",
		},
		{
			name:     "IN with numbers",
			expr:     filter.In("id", []int{1, 2, 3}),
			expected: "id in (1,2,3)",
		},
		{
			name:     "LIKE operator",
			expr:     filter.Like("email", "%@example.com"),
			expected: "email like '%@example.com'",
		},
		{
			name: "AND operator",
			expr: filter.And(
				filter.EQ("status", "active"),
				filter.GT("age", filter.NewLiteral(18)),
			),
			expected: "status == 'active' and age > 18",
		},
		{
			name: "OR operator",
			expr: filter.Or(
				filter.EQ("type", "A"),
				filter.EQ("type", "B"),
			),
			expected: "type == 'A' or type == 'B'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(tt.expr)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestVisitIndexExpressions tests index expression conversion to SQL
func TestVisitIndexExpressions(t *testing.T) {
	tests := []struct {
		name      string
		buildExpr func() ast.Expr
		expected  string
	}{
		{
			name: "Simple array index",
			buildExpr: func() ast.Expr {
				return &ast.IndexExpr{
					LBrack: token.Token{Literal: "["},
					RBrack: token.Token{Literal: "]"},
					Left:   &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "arr"}, Value: "arr"},
					Index:  &ast.Literal{Token: token.Token{Kind: token.NUMBER, Literal: "0"}, Value: "0"},
				}
			},
			expected: "arr[0]",
		},
		{
			name: "Object string index",
			buildExpr: func() ast.Expr {
				return &ast.IndexExpr{
					LBrack: token.Token{Literal: "["},
					RBrack: token.Token{Literal: "]"},
					Left:   &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "obj"}, Value: "obj"},
					Index:  &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "key"}, Value: "key"},
				}
			},
			expected: "obj['key']",
		},
		{
			name: "Nested index",
			buildExpr: func() ast.Expr {
				return &ast.IndexExpr{
					LBrack: token.Token{Literal: "["},
					RBrack: token.Token{Literal: "]"},
					Left: &ast.IndexExpr{
						LBrack: token.Token{Literal: "["},
						RBrack: token.Token{Literal: "]"},
						Left:   &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "data"}, Value: "data"},
						Index:  &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "users"}, Value: "users"},
					},
					Index: &ast.Literal{Token: token.Token{Kind: token.NUMBER, Literal: "0"}, Value: "0"},
				}
			},
			expected: "data['users'][0]",
		},
		{
			name: "Triple nested index",
			buildExpr: func() ast.Expr {
				return &ast.IndexExpr{
					LBrack: token.Token{Literal: "["},
					RBrack: token.Token{Literal: "]"},
					Left: &ast.IndexExpr{
						LBrack: token.Token{Literal: "["},
						RBrack: token.Token{Literal: "]"},
						Left: &ast.IndexExpr{
							LBrack: token.Token{Literal: "["},
							RBrack: token.Token{Literal: "]"},
							Left:   &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "config"}, Value: "config"},
							Index:  &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "settings"}, Value: "settings"},
						},
						Index: &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "app"}, Value: "app"},
					},
					Index: &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "enabled"}, Value: "enabled"},
				}
			},
			expected: "config['settings']['app']['enabled']",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := tt.buildExpr()
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestVisitIndexInComparison tests index expressions in comparisons
func TestVisitIndexInComparison(t *testing.T) {
	tests := []struct {
		name      string
		buildExpr func() ast.Expr
		expected  string
	}{
		{
			name: "Index with equality",
			buildExpr: func() ast.Expr {
				return &ast.BinaryExpr{
					Left: &ast.IndexExpr{
						LBrack: token.Token{Literal: "["},
						RBrack: token.Token{Literal: "]"},
						Left:   &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "data"}, Value: "data"},
						Index:  &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "name"}, Value: "name"},
					},
					Op:    token.Token{Kind: token.EQ, Literal: "=="},
					Right: &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "test"}, Value: "test"},
				}
			},
			expected: "data['name'] == 'test'",
		},
		{
			name: "Nested index with comparison",
			buildExpr: func() ast.Expr {
				return &ast.BinaryExpr{
					Left: &ast.IndexExpr{
						LBrack: token.Token{Literal: "["},
						RBrack: token.Token{Literal: "]"},
						Left: &ast.IndexExpr{
							LBrack: token.Token{Literal: "["},
							RBrack: token.Token{Literal: "]"},
							Left:   &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "config"}, Value: "config"},
							Index:  &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "settings"}, Value: "settings"},
						},
						Index: &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "enabled"}, Value: "enabled"},
					},
					Op:    token.Token{Kind: token.EQ, Literal: "=="},
					Right: &ast.Literal{Token: token.Token{Kind: token.TRUE, Literal: "true"}, Value: "true"},
				}
			},
			expected: "config['settings']['enabled'] == true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := tt.buildExpr()
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestOperatorPrecedence tests proper parentheses insertion for operator precedence
func TestOperatorPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		expected string
	}{
		{
			name: "AND before OR - left side",
			expr: filter.Or(
				filter.And(
					filter.EQ("a", "1"),
					filter.EQ("b", "2"),
				),
				filter.EQ("c", "3"),
			),
			expected: "a == '1' and b == '2' or c == '3'",
		},
		{
			name: "AND before OR - right side",
			expr: filter.Or(
				filter.EQ("a", "1"),
				filter.And(
					filter.EQ("b", "2"),
					filter.EQ("c", "3"),
				),
			),
			expected: "a == '1' or b == '2' and c == '3'",
		},
		{
			name: "OR within AND requires parentheses",
			expr: filter.And(
				filter.Or(
					filter.EQ("a", "1"),
					filter.EQ("b", "2"),
				),
				filter.EQ("c", "3"),
			),
			expected: "(a == '1' or b == '2') and c == '3'",
		},
		{
			name: "Nested OR within AND",
			expr: filter.And(
				filter.EQ("x", "1"),
				filter.Or(
					filter.EQ("a", "2"),
					filter.EQ("b", "3"),
				),
			),
			expected: "x == '1' and (a == '2' or b == '3')",
		},
		{
			name: "Multiple levels of nesting",
			expr: filter.Or(
				filter.And(
					filter.Or(
						filter.EQ("a", "1"),
						filter.EQ("b", "2"),
					),
					filter.EQ("c", "3"),
				),
				filter.EQ("d", "4"),
			),
			expected: "(a == '1' or b == '2') and c == '3' or d == '4'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(tt.expr)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestComplexExpressions tests complex real-world expressions
func TestComplexExpressions(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		expected string
	}{
		{
			name: "Original example from test",
			expr: filter.Or(
				filter.And(
					filter.EQ("user_type", "individual"),
					filter.Or(
						filter.And(
							filter.GE("age", 18),
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
			expected: "user_type == 'individual' and (age >= 18 and name like '%tom' or verified == true) or not (status == 'suspended') and tier in ('gold','platinum')",
		},
		{
			name: "Multiple AND conditions",
			expr: filter.And(
				filter.And(
					filter.EQ("a", "1"),
					filter.EQ("b", "2"),
				),
				filter.EQ("c", "3"),
			),
			expected: "a == '1' and b == '2' and c == '3'",
		},
		{
			name: "Multiple OR conditions",
			expr: filter.Or(
				filter.Or(
					filter.EQ("x", "1"),
					filter.EQ("y", "2"),
				),
				filter.EQ("z", "3"),
			),
			expected: "x == '1' or y == '2' or z == '3'",
		},
		{
			name: "Mixed comparisons with AND",
			expr: filter.And(
				filter.GE("score", filter.NewLiteral(50)),
				filter.LE("score", filter.NewLiteral(100)),
			),
			expected: "score >= 50 and score <= 100",
		},
		{
			name:     "IN with NOT",
			expr:     filter.Not(filter.In("status", []string{"blocked", "suspended"})),
			expected: "not (status in ('blocked','suspended'))",
		},
		{
			name: "Complex nested with index access",
			expr: filter.And(
				&ast.BinaryExpr{
					Left: &ast.IndexExpr{
						LBrack: token.Token{Literal: "["},
						RBrack: token.Token{Literal: "]"},
						Left:   &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "profile"}, Value: "profile"},
						Index:  &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "country"}, Value: "country"},
					},
					Op:    token.Token{Kind: token.EQ, Literal: "=="},
					Right: &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "US"}, Value: "US"},
				},
				filter.GT("age", filter.NewLiteral(18)),
			),
			expected: "profile['country'] == 'US' and age > 18",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(tt.expr)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected:\n'%s'\nGot:\n'%s'", tt.expected, sql)
			}
		})
	}
}

// TestNilExpression tests handling of nil expression
func TestNilExpression(t *testing.T) {
	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(nil)

	if visitor.Error() == nil {
		t.Error("Expected error for nil expression")
		return
	}

	if !strings.Contains(visitor.Error().Error(), "expression is nil") {
		t.Errorf("Expected 'expression is nil' error, got: %v", visitor.Error())
	}

	// SQL should be empty when there's an error
	if visitor.SQL() != "" {
		t.Errorf("Expected empty SQL on error, got: %s", visitor.SQL())
	}
}

// TestErrorPropagation tests that errors stop further processing
func TestErrorPropagation(t *testing.T) {
	visitor := visitors.NewSQLLikeVisitor()

	// First visit with nil causes error
	visitor.Visit(nil)
	firstError := visitor.Error()

	if firstError == nil {
		t.Fatal("Expected error after first visit")
	}

	// Second visit should not change the error state
	validExpr := filter.EQ("name", "test")
	visitor.Visit(validExpr)
	secondError := visitor.Error()

	// The error should still be the same (or at least not nil)
	if secondError == nil {
		t.Error("Error state should persist")
	}
}

// TestAllComparisonOperators tests all comparison operators
func TestAllComparisonOperators(t *testing.T) {
	tests := []struct {
		operator string
		expr     ast.Expr
		expected string
	}{
		{"EQ", filter.EQ("x", "value"), "x == 'value'"},
		{"NE", filter.NE("x", "value"), "x != 'value'"},
		{"LT", filter.LT("x", filter.NewLiteral(10)), "x < 10"},
		{"LE", filter.LE("x", filter.NewLiteral(10)), "x <= 10"},
		{"GT", filter.GT("x", filter.NewLiteral(10)), "x > 10"},
		{"GE", filter.GE("x", filter.NewLiteral(10)), "x >= 10"},
		{"IN", filter.In("x", []string{"a", "b"}), "x in ('a','b')"},
		{"LIKE", filter.Like("x", "%pattern%"), "x like '%pattern%'"},
	}

	for _, tt := range tests {
		t.Run(tt.operator, func(t *testing.T) {
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(tt.expr)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestSpecialCharactersInStrings tests string literals with special characters
func TestSpecialCharactersInStrings(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{"String with spaces", "hello world", "name == 'hello world'"},
		{"String with quotes", "it's here", "name == 'it's here'"},
		{"String with percent", "%test%", "name == '%test%'"},
		{"Empty string", "", "name == ''"},
		{"String with numbers", "test123", "name == 'test123'"},
		{"String with symbols", "@#$%", "name == '@#$%'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := filter.EQ("name", tt.value)
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestNumberFormats tests different number formats
func TestNumberFormats(t *testing.T) {
	tests := []struct {
		name     string
		number   any
		expected string
	}{
		{"Integer", 42, "x == 42"},
		{"Negative integer", -100, "x == -100"},
		{"Float", 3.14, "x == 3.14"},
		{"Zero", 0, "x == 0"},
		{"Large number", 999999, "x == 999999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := filter.EQ("x", cast.ToFloat64(tt.number))
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestBooleanValues tests boolean literals
func TestBooleanValues(t *testing.T) {
	tests := []struct {
		name     string
		value    bool
		expected string
	}{
		{"Boolean true", true, "active == true"},
		{"Boolean false", false, "active == false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := filter.EQ("active", tt.value)
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestEmptyAndSingleElementLists tests edge cases for lists
func TestEmptyAndSingleElementLists(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		expected string
	}{
		{
			name:     "Single element number list",
			expr:     filter.In("id", []int{42}),
			expected: "id in (42)",
		},
		{
			name:     "Single element string list",
			expr:     filter.In("name", []string{"test"}),
			expected: "name in ('test')",
		},
		{
			name:     "Multiple elements",
			expr:     filter.In("id", []int{1, 2, 3, 4, 5}),
			expected: "id in (1,2,3,4,5)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(tt.expr)

			if visitor.Error() != nil {
				t.Fatalf("Unexpected error: %v", visitor.Error())
			}

			sql := visitor.SQL()
			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// BenchmarkSimpleExpression benchmarks simple expression conversion
func BenchmarkSimpleExpression(b *testing.B) {
	expr := filter.EQ("name", "John")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		visitor := visitors.NewSQLLikeVisitor()
		visitor.Visit(expr)
		_ = visitor.SQL()
	}
}

// BenchmarkComplexExpression benchmarks complex expression conversion
func BenchmarkComplexExpression(b *testing.B) {
	expr := filter.Or(
		filter.And(
			filter.EQ("user_type", "individual"),
			filter.Or(
				filter.And(
					filter.GE("age", 18),
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
		visitor := visitors.NewSQLLikeVisitor()
		visitor.Visit(expr)
		_ = visitor.SQL()
	}
}

// BenchmarkDeeplyNestedExpression benchmarks deeply nested expression conversion
func BenchmarkDeeplyNestedExpression(b *testing.B) {
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
		visitor := visitors.NewSQLLikeVisitor()
		visitor.Visit(expr)
		_ = visitor.SQL()
	}
}

// BenchmarkWithIndexAccess benchmarks expression with index access
func BenchmarkWithIndexAccess(b *testing.B) {
	expr := &ast.BinaryExpr{
		Left: &ast.IndexExpr{
			LBrack: token.Token{Literal: "["},
			RBrack: token.Token{Literal: "]"},
			Left: &ast.IndexExpr{
				LBrack: token.Token{Literal: "["},
				RBrack: token.Token{Literal: "]"},
				Left:   &ast.Ident{Token: token.Token{Kind: token.IDENT, Literal: "config"}, Value: "config"},
				Index:  &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "settings"}, Value: "settings"},
			},
			Index: &ast.Literal{Token: token.Token{Kind: token.STRING, Literal: "enabled"}, Value: "enabled"},
		},
		Op:    token.Token{Kind: token.EQ, Literal: "=="},
		Right: &ast.Literal{Token: token.Token{Kind: token.TRUE, Literal: "true"}, Value: "true"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		visitor := visitors.NewSQLLikeVisitor()
		visitor.Visit(expr)
		_ = visitor.SQL()
	}
}
