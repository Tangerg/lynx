package filter_test

import (
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/visitors"
)

// TestNewIdent tests the NewIdent function with various input types
func TestNewIdent(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantValue string
		wantErr   bool
	}{
		{
			name:      "String identifier",
			input:     "username",
			wantValue: "username",
			wantErr:   false,
		},
		{
			name:      "Identifier with underscore",
			input:     "user_name",
			wantValue: "user_name",
			wantErr:   false,
		},
		{
			name:      "CamelCase identifier",
			input:     "firstName",
			wantValue: "firstName",
			wantErr:   false,
		},
		{
			name: "Existing Ident passthrough",
			input: &ast.Ident{
				Token: token.Token{Kind: token.IDENT, Literal: "existing"},
				Value: "existing",
			},
			wantValue: "existing",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ident *ast.Ident
			switch v := tt.input.(type) {
			case string:
				ident = filter.NewIdent(v)
			case *ast.Ident:
				ident = filter.NewIdent(v)
			}

			if ident == nil {
				t.Fatal("Expected non-nil ident")
			}

			if ident.Value != tt.wantValue {
				t.Errorf("Expected value '%s', got '%s'", tt.wantValue, ident.Value)
			}

			if ident.Token.Kind != token.IDENT {
				t.Errorf("Expected IDENT token kind, got %v", ident.Token.Kind)
			}
		})
	}
}

// TestNewLiteral tests the NewLiteral function with various types
func TestNewLiteral(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantKind  token.Kind
		wantValue string
	}{
		{
			name:      "Integer literal",
			input:     42,
			wantKind:  token.NUMBER,
			wantValue: "42",
		},
		{
			name:      "Float literal",
			input:     3.14,
			wantKind:  token.NUMBER,
			wantValue: "3.14",
		},
		{
			name:      "String literal",
			input:     "hello",
			wantKind:  token.STRING,
			wantValue: "hello",
		},
		{
			name:      "Boolean true",
			input:     true,
			wantKind:  token.TRUE,
			wantValue: "true",
		},
		{
			name:      "Boolean false",
			input:     false,
			wantKind:  token.FALSE,
			wantValue: "false",
		},
		{
			name:      "Negative integer",
			input:     -100,
			wantKind:  token.NUMBER,
			wantValue: "-100",
		},
		{
			name:      "Zero",
			input:     0,
			wantKind:  token.NUMBER,
			wantValue: "0",
		},
		{
			name:      "Empty string",
			input:     "",
			wantKind:  token.STRING,
			wantValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var literal *ast.Literal
			switch v := tt.input.(type) {
			case int:
				literal = filter.NewLiteral(v)
			case float64:
				literal = filter.NewLiteral(v)
			case string:
				literal = filter.NewLiteral(v)
			case bool:
				literal = filter.NewLiteral(v)
			}

			if literal == nil {
				t.Fatal("Expected non-nil literal")
			}

			if literal.Token.Kind != tt.wantKind {
				t.Errorf("Expected token kind %v, got %v", tt.wantKind, literal.Token.Kind)
			}

			if literal.Value != tt.wantValue {
				t.Errorf("Expected value '%s', got '%s'", tt.wantValue, literal.Value)
			}
		})
	}
}

// TestNewLiterals tests the NewLiterals function
func TestNewLiterals(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantLen   int
		wantKind  token.Kind
		wantFirst string
	}{
		{
			name:      "Integer slice",
			input:     []int{1, 2, 3},
			wantLen:   3,
			wantKind:  token.NUMBER,
			wantFirst: "1",
		},
		{
			name:      "String slice",
			input:     []string{"a", "b", "c"},
			wantLen:   3,
			wantKind:  token.STRING,
			wantFirst: "a",
		},
		{
			name:      "Boolean slice",
			input:     []bool{true, false},
			wantLen:   2,
			wantKind:  token.TRUE,
			wantFirst: "true",
		},
		{
			name:      "Float slice",
			input:     []float64{1.1, 2.2, 3.3},
			wantLen:   3,
			wantKind:  token.NUMBER,
			wantFirst: "1.1",
		},
		{
			name:      "Empty slice",
			input:     []int{},
			wantLen:   0,
			wantKind:  token.EQ,
			wantFirst: "",
		},
		{
			name:      "Single element",
			input:     []string{"only"},
			wantLen:   1,
			wantKind:  token.STRING,
			wantFirst: "only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var literals []*ast.Literal

			switch v := tt.input.(type) {
			case []int:
				literals = filter.NewLiterals(v)
			case []string:
				literals = filter.NewLiterals(v)
			case []bool:
				literals = filter.NewLiterals(v)
			case []float64:
				literals = filter.NewLiterals(v)
			}

			if len(literals) != tt.wantLen {
				t.Errorf("Expected length %d, got %d", tt.wantLen, len(literals))
			}

			if tt.wantLen > 0 {
				if literals[0].Token.Kind != tt.wantKind {
					t.Errorf("Expected first token kind %v, got %v", tt.wantKind, literals[0].Token.Kind)
				}
				if literals[0].Value != tt.wantFirst {
					t.Errorf("Expected first value '%s', got '%s'", tt.wantFirst, literals[0].Value)
				}
			}
		})
	}
}

// TestNewListLiteral tests the NewListLiteral function
func TestNewListLiteral(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		wantLen  int
		expected string
	}{
		{
			name:     "Integer list",
			input:    []int{1, 2, 3},
			wantLen:  3,
			expected: "(1,2,3)",
		},
		{
			name:     "String list",
			input:    []string{"a", "b", "c"},
			wantLen:  3,
			expected: "('a','b','c')",
		},
		{
			name:     "Boolean list",
			input:    []bool{true, false, true},
			wantLen:  3,
			expected: "(true,false,true)",
		},
		{
			name:     "Float list",
			input:    []float64{1.1, 2.2, 3.3},
			wantLen:  3,
			expected: "(1.1,2.2,3.3)",
		},
		{
			name:     "Empty list",
			input:    []int{},
			wantLen:  0,
			expected: "()",
		},
		{
			name:     "Single element list",
			input:    []string{"only"},
			wantLen:  1,
			expected: "('only')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var listLiteral *ast.ListLiteral

			switch v := tt.input.(type) {
			case []int:
				listLiteral = filter.NewListLiteral(v)
			case []string:
				listLiteral = filter.NewListLiteral(v)
			case []bool:
				listLiteral = filter.NewListLiteral(v)
			case []float64:
				listLiteral = filter.NewListLiteral(v)
			}

			if listLiteral == nil {
				t.Fatal("Expected non-nil list literal")
			}

			if len(listLiteral.Values) != tt.wantLen {
				t.Errorf("Expected %d values, got %d", tt.wantLen, len(listLiteral.Values))
			}

			if listLiteral.Lparen.Kind != token.LPAREN {
				t.Errorf("Expected LPAREN token, got %v", listLiteral.Lparen.Kind)
			}

			if listLiteral.Rparen.Kind != token.RPAREN {
				t.Errorf("Expected RPAREN token, got %v", listLiteral.Rparen.Kind)
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(listLiteral)
			sql := visitor.SQL()

			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestEQ tests the EQ comparison operator
func TestEQ(t *testing.T) {
	tests := []struct {
		name     string
		left     any
		right    any
		expected string
	}{
		{
			name:     "String equality",
			left:     "name",
			right:    "John",
			expected: "name == 'John'",
		},
		{
			name:     "Integer equality",
			left:     "age",
			right:    25,
			expected: "age == 25",
		},
		{
			name:     "Boolean equality",
			left:     "active",
			right:    true,
			expected: "active == true",
		},
		{
			name:     "Float equality",
			left:     "score",
			right:    98.5,
			expected: "score == 98.5",
		},
		{
			name:     "Index expression equality",
			left:     filter.Index("data", "key"),
			right:    "value",
			expected: "data['key'] == 'value'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expr *ast.BinaryExpr

			switch l := tt.left.(type) {
			case string:
				switch r := tt.right.(type) {
				case string:
					expr = filter.EQ(l, r)
				case int:
					expr = filter.EQ(l, r)
				case bool:
					expr = filter.EQ(l, r)
				case float64:
					expr = filter.EQ(l, r)
				}
			case *ast.IndexExpr:
				switch r := tt.right.(type) {
				case string:
					expr = filter.EQ(l, r)
				}
			}

			if expr == nil {
				t.Fatal("Expected non-nil expression")
			}

			if expr.Op.Kind != token.EQ {
				t.Errorf("Expected EQ operator, got %v", expr.Op.Kind)
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestNE tests the NE comparison operator
func TestNE(t *testing.T) {
	tests := []struct {
		name     string
		left     string
		right    any
		expected string
	}{
		{
			name:     "String inequality",
			left:     "status",
			right:    "inactive",
			expected: "status != 'inactive'",
		},
		{
			name:     "Integer inequality",
			left:     "count",
			right:    0,
			expected: "count != 0",
		},
		{
			name:     "Boolean inequality",
			left:     "enabled",
			right:    false,
			expected: "enabled != false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expr *ast.BinaryExpr

			switch r := tt.right.(type) {
			case string:
				expr = filter.NE(tt.left, r)
			case int:
				expr = filter.NE(tt.left, r)
			case bool:
				expr = filter.NE(tt.left, r)
			}

			if expr.Op.Kind != token.NE {
				t.Errorf("Expected NE operator, got %v", expr.Op.Kind)
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestLT tests the LT comparison operator
func TestLT(t *testing.T) {
	tests := []struct {
		name     string
		left     string
		right    any
		expected string
	}{
		{
			name:     "Integer less than",
			left:     "age",
			right:    18,
			expected: "age < 18",
		},
		{
			name:     "Float less than",
			left:     "price",
			right:    99.99,
			expected: "price < 99.99",
		},
		{
			name:     "Negative less than",
			left:     "temperature",
			right:    -10,
			expected: "temperature < -10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expr *ast.BinaryExpr

			switch r := tt.right.(type) {
			case int:
				expr = filter.LT(tt.left, r)
			case float64:
				expr = filter.LT(tt.left, r)
			}

			if expr.Op.Kind != token.LT {
				t.Errorf("Expected LT operator, got %v", expr.Op.Kind)
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestLE tests the LE comparison operator
func TestLE(t *testing.T) {
	tests := []struct {
		name     string
		left     string
		right    any
		expected string
	}{
		{
			name:     "Integer less or equal",
			left:     "age",
			right:    65,
			expected: "age <= 65",
		},
		{
			name:     "Float less or equal",
			left:     "balance",
			right:    1000.50,
			expected: "balance <= 1000.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expr *ast.BinaryExpr

			switch r := tt.right.(type) {
			case int:
				expr = filter.LE(tt.left, r)
			case float64:
				expr = filter.LE(tt.left, r)
			}

			if expr.Op.Kind != token.LE {
				t.Errorf("Expected LE operator, got %v", expr.Op.Kind)
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestGT tests the GT comparison operator
func TestGT(t *testing.T) {
	tests := []struct {
		name     string
		left     string
		right    any
		expected string
	}{
		{
			name:     "Integer greater than",
			left:     "score",
			right:    100,
			expected: "score > 100",
		},
		{
			name:     "Zero comparison",
			left:     "quantity",
			right:    0,
			expected: "quantity > 0",
		},
		{
			name:     "Float greater than",
			left:     "rating",
			right:    4.5,
			expected: "rating > 4.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expr *ast.BinaryExpr

			switch r := tt.right.(type) {
			case int:
				expr = filter.GT(tt.left, r)
			case float64:
				expr = filter.GT(tt.left, r)
			}

			if expr.Op.Kind != token.GT {
				t.Errorf("Expected GT operator, got %v", expr.Op.Kind)
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestGE tests the GE comparison operator
func TestGE(t *testing.T) {
	tests := []struct {
		name     string
		left     string
		right    any
		expected string
	}{
		{
			name:     "Integer greater or equal",
			left:     "age",
			right:    18,
			expected: "age >= 18",
		},
		{
			name:     "Float greater or equal",
			left:     "rating",
			right:    4.5,
			expected: "rating >= 4.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expr *ast.BinaryExpr

			switch r := tt.right.(type) {
			case int:
				expr = filter.GE(tt.left, r)
			case float64:
				expr = filter.GE(tt.left, r)
			}

			if expr.Op.Kind != token.GE {
				t.Errorf("Expected GE operator, got %v", expr.Op.Kind)
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestIn tests the IN operator
func TestIn(t *testing.T) {
	tests := []struct {
		name     string
		left     any
		right    any
		expected string
	}{
		{
			name:     "String list",
			left:     "status",
			right:    []string{"pending", "active", "completed"},
			expected: "status in ('pending','active','completed')",
		},
		{
			name:     "Integer list",
			left:     "id",
			right:    []int{1, 2, 3, 4, 5},
			expected: "id in (1,2,3,4,5)",
		},
		{
			name:     "Single element",
			left:     "type",
			right:    []string{"premium"},
			expected: "type in ('premium')",
		},
		{
			name:     "Index expression",
			left:     filter.Index("info", "email"),
			right:    []string{"test@example.com"},
			expected: "info['email'] in ('test@example.com')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expr *ast.BinaryExpr

			switch l := tt.left.(type) {
			case string:
				switch r := tt.right.(type) {
				case []string:
					expr = filter.In(l, r)
				case []int:
					expr = filter.In(l, r)
				}
			case *ast.IndexExpr:
				switch r := tt.right.(type) {
				case []string:
					expr = filter.In(l, r)
				}
			}

			if expr.Op.Kind != token.IN {
				t.Errorf("Expected IN operator, got %v", expr.Op.Kind)
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestLike tests the LIKE operator
func TestLike(t *testing.T) {
	tests := []struct {
		name     string
		left     any
		right    string
		expected string
	}{
		{
			name:     "Prefix pattern",
			left:     "name",
			right:    "John%",
			expected: "name like 'John%'",
		},
		{
			name:     "Suffix pattern",
			left:     "email",
			right:    "%@gmail.com",
			expected: "email like '%@gmail.com'",
		},
		{
			name:     "Contains pattern",
			left:     "description",
			right:    "%test%",
			expected: "description like '%test%'",
		},
		{
			name:     "Index expression",
			left:     filter.Index("profile", "bio"),
			right:    "%developer%",
			expected: "profile['bio'] like '%developer%'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expr *ast.BinaryExpr

			switch l := tt.left.(type) {
			case string:
				expr = filter.Like(l, tt.right)
			case *ast.IndexExpr:
				expr = filter.Like(l, tt.right)
			}

			if expr.Op.Kind != token.LIKE {
				t.Errorf("Expected LIKE operator, got %v", expr.Op.Kind)
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestAnd tests the AND logical operator
func TestAnd(t *testing.T) {
	tests := []struct {
		name     string
		left     ast.ComputedExpr
		right    ast.ComputedExpr
		expected string
	}{
		{
			name:     "Two comparisons",
			left:     filter.EQ("status", "active"),
			right:    filter.GT("age", 18),
			expected: "status == 'active' and age > 18",
		},
		{
			name:     "Multiple conditions",
			left:     filter.GE("score", 100),
			right:    filter.LE("score", 200),
			expected: "score >= 100 and score <= 200",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := filter.And(tt.left, tt.right)

			if expr.Op.Kind != token.AND {
				t.Errorf("Expected AND operator, got %v", expr.Op.Kind)
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestOr tests the OR logical operator
func TestOr(t *testing.T) {
	tests := []struct {
		name     string
		left     ast.ComputedExpr
		right    ast.ComputedExpr
		expected string
	}{
		{
			name:     "Two comparisons",
			left:     filter.EQ("status", "pending"),
			right:    filter.EQ("status", "active"),
			expected: "status == 'pending' or status == 'active'",
		},
		{
			name:     "Different fields",
			left:     filter.EQ("role", "admin"),
			right:    filter.EQ("role", "moderator"),
			expected: "role == 'admin' or role == 'moderator'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := filter.Or(tt.left, tt.right)

			if expr.Op.Kind != token.OR {
				t.Errorf("Expected OR operator, got %v", expr.Op.Kind)
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestNot tests the NOT logical operator
func TestNot(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.ComputedExpr
		expected string
	}{
		{
			name:     "Simple negation",
			expr:     filter.EQ("status", "suspended"),
			expected: "not (status == 'suspended')",
		},
		{
			name:     "Negated IN",
			expr:     filter.In("status", []string{"blocked", "banned"}),
			expected: "not (status in ('blocked','banned'))",
		},
		{
			name:     "Negated AND",
			expr:     filter.And(filter.EQ("a", "1"), filter.EQ("b", "2")),
			expected: "not (a == '1' and b == '2')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := filter.Not(tt.expr)

			if expr.Op.Kind != token.NOT {
				t.Errorf("Expected NOT operator, got %v", expr.Op.Kind)
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestIndex tests the Index function
func TestIndex(t *testing.T) {
	tests := []struct {
		name     string
		build    func() *ast.IndexExpr
		expected string
	}{
		{
			name: "Simple string key",
			build: func() *ast.IndexExpr {
				return filter.Index("data", "key")
			},
			expected: "data['key']",
		},
		{
			name: "Integer index",
			build: func() *ast.IndexExpr {
				return filter.Index("array", 0)
			},
			expected: "array[0]",
		},
		{
			name: "Nested index",
			build: func() *ast.IndexExpr {
				return filter.Index(filter.Index("data", "users"), 0)
			},
			expected: "data['users'][0]",
		},
		{
			name: "Triple nested",
			build: func() *ast.IndexExpr {
				return filter.Index(filter.Index(filter.Index("a", "b"), "c"), "d")
			},
			expected: "a['b']['c']['d']",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indexExpr := tt.build()

			if indexExpr.LBrack.Kind != token.LBRACK {
				t.Errorf("Expected LBRACK token, got %v", indexExpr.LBrack.Kind)
			}

			if indexExpr.RBrack.Kind != token.RBRACK {
				t.Errorf("Expected RBRACK token, got %v", indexExpr.RBrack.Kind)
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(indexExpr)
			sql := visitor.SQL()

			if sql != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
			}
		})
	}
}

// TestComplexExpressions tests complex combinations of operators
func TestComplexExpressions(t *testing.T) {
	tests := []struct {
		name  string
		build func() ast.Expr
	}{
		{
			name: "AND with OR",
			build: func() ast.Expr {
				return filter.And(
					filter.EQ("type", "user"),
					filter.Or(
						filter.EQ("status", "active"),
						filter.EQ("status", "pending"),
					),
				)
			},
		},
		{
			name: "NOT with AND",
			build: func() ast.Expr {
				return filter.Not(
					filter.And(
						filter.EQ("blocked", true),
						filter.EQ("suspended", true),
					),
				)
			},
		},
		{
			name: "Index with comparison",
			build: func() ast.Expr {
				return filter.EQ(filter.Index("profile", "country"), "US")
			},
		},
		{
			name: "Multiple operators",
			build: func() ast.Expr {
				return filter.And(
					filter.GT("age", 18),
					filter.And(
						filter.LT("age", 65),
						filter.In("status", []string{"active", "verified"}),
					),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := tt.build()

			if expr == nil {
				t.Fatal("Expected non-nil expression")
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if sql == "" {
				t.Error("Expected non-empty SQL")
			}

			t.Logf("Generated SQL: %s", sql)
		})
	}
}

// TestNumericTypes tests all numeric types support
func TestNumericTypes(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{"int", int(42)},
		{"int8", int8(42)},
		{"int16", int16(42)},
		{"int32", int32(42)},
		{"int64", int64(42)},
		{"uint", uint(42)},
		{"uint8", uint8(42)},
		{"uint16", uint16(42)},
		{"uint32", uint32(42)},
		{"uint64", uint64(42)},
		{"float32", float32(3.14)},
		{"float64", float64(3.14)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var literal *ast.Literal

			switch v := tt.value.(type) {
			case int:
				literal = filter.NewLiteral(v)
			case int8:
				literal = filter.NewLiteral(v)
			case int16:
				literal = filter.NewLiteral(v)
			case int32:
				literal = filter.NewLiteral(v)
			case int64:
				literal = filter.NewLiteral(v)
			case uint:
				literal = filter.NewLiteral(v)
			case uint8:
				literal = filter.NewLiteral(v)
			case uint16:
				literal = filter.NewLiteral(v)
			case uint32:
				literal = filter.NewLiteral(v)
			case uint64:
				literal = filter.NewLiteral(v)
			case float32:
				literal = filter.NewLiteral(v)
			case float64:
				literal = filter.NewLiteral(v)
			}

			if literal == nil {
				t.Fatal("Expected non-nil literal")
			}

			if literal.Token.Kind != token.NUMBER {
				t.Errorf("Expected NUMBER token, got %v", literal.Token.Kind)
			}

			if literal.Value == "" {
				t.Error("Expected non-empty value")
			}
		})
	}
}

// TestAllListTypes tests all supported list types
func TestAllListTypes(t *testing.T) {
	tests := []struct {
		name  string
		build func() *ast.ListLiteral
	}{
		{"[]int", func() *ast.ListLiteral { return filter.NewListLiteral([]int{1, 2, 3}) }},
		{"[]int8", func() *ast.ListLiteral { return filter.NewListLiteral([]int8{1, 2, 3}) }},
		{"[]int16", func() *ast.ListLiteral { return filter.NewListLiteral([]int16{1, 2, 3}) }},
		{"[]int32", func() *ast.ListLiteral { return filter.NewListLiteral([]int32{1, 2, 3}) }},
		{"[]int64", func() *ast.ListLiteral { return filter.NewListLiteral([]int64{1, 2, 3}) }},
		{"[]uint", func() *ast.ListLiteral { return filter.NewListLiteral([]uint{1, 2, 3}) }},
		{"[]uint8", func() *ast.ListLiteral { return filter.NewListLiteral([]uint8{1, 2, 3}) }},
		{"[]uint16", func() *ast.ListLiteral { return filter.NewListLiteral([]uint16{1, 2, 3}) }},
		{"[]uint32", func() *ast.ListLiteral { return filter.NewListLiteral([]uint32{1, 2, 3}) }},
		{"[]uint64", func() *ast.ListLiteral { return filter.NewListLiteral([]uint64{1, 2, 3}) }},
		{"[]float32", func() *ast.ListLiteral { return filter.NewListLiteral([]float32{1.1, 2.2, 3.3}) }},
		{"[]float64", func() *ast.ListLiteral { return filter.NewListLiteral([]float64{1.1, 2.2, 3.3}) }},
		{"[]string", func() *ast.ListLiteral { return filter.NewListLiteral([]string{"a", "b", "c"}) }},
		{"[]bool", func() *ast.ListLiteral { return filter.NewListLiteral([]bool{true, false, true}) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listLiteral := tt.build()

			if listLiteral == nil {
				t.Fatal("Expected non-nil list literal")
			}

			if len(listLiteral.Values) == 0 {
				t.Error("Expected non-empty list")
			}
		})
	}
}

// BenchmarkEQ benchmarks the EQ operation
func BenchmarkEQ(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.EQ("name", "John")
	}
}

// BenchmarkComplexExpression benchmarks complex expression building
func BenchmarkComplexExpression(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.And(
			filter.EQ("type", "user"),
			filter.Or(
				filter.GT("age", 18),
				filter.In("status", []string{"active", "pending"}),
			),
		)
	}
}

// BenchmarkIndex benchmarks index expression creation
func BenchmarkIndex(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.Index(filter.Index("data", "users"), 0)
	}
}
