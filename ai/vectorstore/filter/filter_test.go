package filter_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/visitors"
)

// TestParse tests the Parse function with various valid expressions
func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		validate func(*testing.T, ast.Expr)
	}{
		{
			name:    "Simple equality",
			input:   "name == 'John'",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				if binaryExpr.Op.Literal != "==" {
					t.Errorf("Expected '==' operator, got '%s'", binaryExpr.Op.Literal)
				}
			},
		},
		{
			name:    "Inequality",
			input:   "status != 'inactive'",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				if binaryExpr.Op.Literal != "!=" {
					t.Errorf("Expected '!=' operator, got '%s'", binaryExpr.Op.Literal)
				}
			},
		},
		{
			name:    "Less than",
			input:   "age < 18",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				if binaryExpr.Op.Literal != "<" {
					t.Errorf("Expected '<' operator, got '%s'", binaryExpr.Op.Literal)
				}
			},
		},
		{
			name:    "Less or equal",
			input:   "score <= 100",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				if binaryExpr.Op.Literal != "<=" {
					t.Errorf("Expected '<=' operator, got '%s'", binaryExpr.Op.Literal)
				}
			},
		},
		{
			name:    "Greater than",
			input:   "balance > 1000",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				if binaryExpr.Op.Literal != ">" {
					t.Errorf("Expected '>' operator, got '%s'", binaryExpr.Op.Literal)
				}
			},
		},
		{
			name:    "Greater or equal",
			input:   "rating >= 4.5",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				if binaryExpr.Op.Literal != ">=" {
					t.Errorf("Expected '>=' operator, got '%s'", binaryExpr.Op.Literal)
				}
			},
		},
		{
			name:    "IN operator with strings",
			input:   "status in ('active', 'pending')",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				if binaryExpr.Op.Literal != "in" {
					t.Errorf("Expected 'in' operator, got '%s'", binaryExpr.Op.Literal)
				}
			},
		},
		{
			name:    "IN operator with numbers",
			input:   "id in (1, 2, 3, 4, 5)",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				listLiteral, ok := binaryExpr.Right.(*ast.ListLiteral)
				if !ok {
					t.Fatal("Expected ListLiteral on right side")
				}
				if len(listLiteral.Values) != 5 {
					t.Errorf("Expected 5 values in list, got %d", len(listLiteral.Values))
				}
			},
		},
		{
			name:    "LIKE operator",
			input:   "name like 'John%'",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				if binaryExpr.Op.Literal != "like" {
					t.Errorf("Expected 'like' operator, got '%s'", binaryExpr.Op.Literal)
				}
			},
		},
		{
			name:    "AND operator",
			input:   "age > 18 and status == 'active'",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				if binaryExpr.Op.Literal != "and" {
					t.Errorf("Expected 'and' operator, got '%s'", binaryExpr.Op.Literal)
				}
			},
		},
		{
			name:    "OR operator",
			input:   "status == 'active' or status == 'pending'",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				if binaryExpr.Op.Literal != "or" {
					t.Errorf("Expected 'or' operator, got '%s'", binaryExpr.Op.Literal)
				}
			},
		},
		{
			name:    "NOT operator",
			input:   "not (status == 'suspended')",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				unaryExpr, ok := expr.(*ast.UnaryExpr)
				if !ok {
					t.Fatal("Expected UnaryExpr")
				}
				if unaryExpr.Op.Literal != "not" {
					t.Errorf("Expected 'not' operator, got '%s'", unaryExpr.Op.Literal)
				}
			},
		},
		{
			name:    "Array index with number",
			input:   "colors[0] == 'red'",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				indexExpr, ok := binaryExpr.Left.(*ast.IndexExpr)
				if !ok {
					t.Fatal("Expected IndexExpr on left side")
				}
				if indexExpr == nil {
					t.Fatal("IndexExpr should not be nil")
				}
			},
		},
		{
			name:    "Object property access",
			input:   "user['name'] == 'John'",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				indexExpr, ok := binaryExpr.Left.(*ast.IndexExpr)
				if !ok {
					t.Fatal("Expected IndexExpr on left side")
				}
				if indexExpr == nil {
					t.Fatal("IndexExpr should not be nil")
				}
			},
		},
		{
			name:    "Nested index access",
			input:   "data['users'][0]['name'] == 'John'",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				indexExpr, ok := binaryExpr.Left.(*ast.IndexExpr)
				if !ok {
					t.Fatal("Expected IndexExpr on left side")
				}
				if indexExpr == nil {
					t.Fatal("IndexExpr should not be nil")
				}
			},
		},
		{
			name:    "Boolean literal true",
			input:   "active == true",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				literal, ok := binaryExpr.Right.(*ast.Literal)
				if !ok {
					t.Fatal("Expected Literal on right side")
				}
				if literal.Value != "true" {
					t.Errorf("Expected 'true', got '%s'", literal.Value)
				}
			},
		},
		{
			name:    "Boolean literal false",
			input:   "enabled == false",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				binaryExpr, ok := expr.(*ast.BinaryExpr)
				if !ok {
					t.Fatal("Expected BinaryExpr")
				}
				literal, ok := binaryExpr.Right.(*ast.Literal)
				if !ok {
					t.Fatal("Expected Literal on right side")
				}
				if literal.Value != "false" {
					t.Errorf("Expected 'false', got '%s'", literal.Value)
				}
			},
		},
		{
			name:    "Parentheses for precedence",
			input:   "(age > 18 and status == 'active') or verified == true",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				if expr == nil {
					t.Fatal("Expected non-nil expression")
				}
			},
		},
		{
			name:    "Complex nested expression",
			input:   "user_type == 'individual' and (age >= 18 and name like '%tom' or verified == true)",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				if expr == nil {
					t.Fatal("Expected non-nil expression")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := filter.Parse(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && expr == nil {
				t.Fatal("Expected non-nil expression")
			}

			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, expr)
			}
		})
	}
}

// TestParseErrors tests Parse function with invalid inputs
func TestParseErrors(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "Empty input",
			input:     "",
			wantErr:   true,
			errSubstr: "",
		},
		{
			name:      "Incomplete expression",
			input:     "name ==",
			wantErr:   true,
			errSubstr: "",
		},
		{
			name:      "Missing operator",
			input:     "name 'John'",
			wantErr:   true,
			errSubstr: "",
		},
		{
			name:      "Unbalanced parentheses",
			input:     "(name == 'John'",
			wantErr:   true,
			errSubstr: "",
		},
		{
			name:      "Invalid operator",
			input:     "name === 'John'",
			wantErr:   true,
			errSubstr: "",
		},
		{
			name:      "Unclosed string",
			input:     "name == 'John",
			wantErr:   true,
			errSubstr: "",
		},
		{
			name:      "Invalid list syntax",
			input:     "id in (1, 2,",
			wantErr:   true,
			errSubstr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := filter.Parse(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && tt.errSubstr != "" {
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errSubstr, err.Error())
				}
			}

			if tt.wantErr && expr != nil {
				t.Error("Expected nil expression on parse error")
			}
		})
	}
}

// TestAnalyze tests the Analyze function with valid expressions
func TestAnalyze(t *testing.T) {
	tests := []struct {
		name    string
		expr    ast.Expr
		wantErr bool
	}{
		{
			name:    "Valid comparison",
			expr:    filter.EQ("name", "John"),
			wantErr: false,
		},
		{
			name:    "Valid IN expression",
			expr:    filter.In("status", []string{"active", "pending"}),
			wantErr: false,
		},
		{
			name:    "Valid AND expression",
			expr:    filter.And(filter.EQ("age", 18), filter.EQ("status", "active")),
			wantErr: false,
		},
		{
			name:    "Valid OR expression",
			expr:    filter.Or(filter.EQ("type", "A"), filter.EQ("type", "B")),
			wantErr: false,
		},
		{
			name:    "Valid NOT expression",
			expr:    filter.Not(filter.EQ("blocked", true)),
			wantErr: false,
		},
		{
			name:    "Valid nested expression",
			expr:    filter.And(filter.GT("age", 18), filter.Or(filter.EQ("status", "active"), filter.EQ("verified", true))),
			wantErr: false,
		},
		{
			name:    "Valid index expression",
			expr:    filter.EQ(filter.Index("data", "key"), "value"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := filter.Analyze(tt.expr)

			if (err != nil) != tt.wantErr {
				t.Errorf("Analyze() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestAnalyzeErrors tests Analyze with potentially invalid expressions
func TestAnalyzeErrors(t *testing.T) {
	tests := []struct {
		name    string
		expr    ast.Expr
		wantErr bool
	}{
		{
			name:    "Nil expression",
			expr:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := filter.Analyze(tt.expr)

			if (err != nil) != tt.wantErr {
				t.Errorf("Analyze() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestParseAndAnalyze tests the combined ParseAndAnalyze function
func TestParseAndAnalyze(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		validate func(*testing.T, ast.Expr)
	}{
		{
			name:    "Valid simple expression",
			input:   "name == 'John'",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				if expr == nil {
					t.Fatal("Expected non-nil expression")
				}
			},
		},
		{
			name:    "Valid complex expression",
			input:   "age > 18 and (status == 'active' or verified == true)",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				if expr == nil {
					t.Fatal("Expected non-nil expression")
				}
			},
		},
		{
			name:    "Valid IN expression",
			input:   "id in (1, 2, 3)",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				if expr == nil {
					t.Fatal("Expected non-nil expression")
				}
			},
		},
		{
			name:    "Valid NOT expression",
			input:   "not (status == 'suspended')",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				if expr == nil {
					t.Fatal("Expected non-nil expression")
				}
			},
		},
		{
			name:    "Valid index expression",
			input:   "data['key'] == 'value'",
			wantErr: false,
			validate: func(t *testing.T, expr ast.Expr) {
				if expr == nil {
					t.Fatal("Expected non-nil expression")
				}
			},
		},
		{
			name:    "Parse error - empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "Parse error - invalid syntax",
			input:   "name ==",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := filter.ParseAndAnalyze(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAndAnalyze() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if expr == nil {
					t.Fatal("Expected non-nil expression")
				}
				if tt.validate != nil {
					tt.validate(t, expr)
				}
			}

			if tt.wantErr && expr != nil {
				t.Error("Expected nil expression on error")
			}
		})
	}
}

// TestParseAndAnalyzeWithVisitor tests ParseAndAnalyze with SQL generation
func TestParseAndAnalyzeWithVisitor(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		expectedSQL string
	}{
		{
			name:        "Simple equality",
			input:       "name == 'John'",
			wantErr:     false,
			expectedSQL: "name == 'John'",
		},
		{
			name:        "Multiple conditions with AND",
			input:       "age > 18 and status == 'active'",
			wantErr:     false,
			expectedSQL: "age > 18 and status == 'active'",
		},
		{
			name:        "OR with parentheses",
			input:       "(status == 'active' or status == 'pending') and age > 18",
			wantErr:     false,
			expectedSQL: "(status == 'active' or status == 'pending') and age > 18",
		},
		{
			name:        "IN operator",
			input:       "id in (1, 2, 3)",
			wantErr:     false,
			expectedSQL: "id in (1,2,3)",
		},
		{
			name:        "LIKE operator",
			input:       "name like 'John%'",
			wantErr:     false,
			expectedSQL: "name like 'John%'",
		},
		{
			name:        "NOT operator",
			input:       "not (blocked == true)",
			wantErr:     false,
			expectedSQL: "not (blocked == true)",
		},
		{
			name:        "Index access",
			input:       "data['key'] == 'value'",
			wantErr:     false,
			expectedSQL: "data['key'] == 'value'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := filter.ParseAndAnalyze(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAndAnalyze() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				visitor := visitors.NewSQLLikeVisitor()
				visitor.Visit(expr)
				sql := visitor.SQL()

				if sql != tt.expectedSQL {
					t.Errorf("Expected SQL '%s', got '%s'", tt.expectedSQL, sql)
				}
			}
		})
	}
}

// TestParseRoundTrip tests parsing and regenerating SQL
func TestParseRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Simple comparison",
			input: "name == 'John'",
		},
		{
			name:  "Multiple ANDs",
			input: "a == 'x' and b == 'y' and c == 'z'",
		},
		{
			name:  "Multiple ORs",
			input: "a == '1' or b == '2' or c == '3'",
		},
		{
			name:  "Nested expressions",
			input: "(a == '1' or b == '2') and c == '3'",
		},
		{
			name:  "Numeric comparisons",
			input: "age >= 18 and age <= 65",
		},
		{
			name:  "Boolean values",
			input: "active == true and verified == false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the input
			expr, err := filter.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			// Analyze the expression
			err = filter.Analyze(expr)
			if err != nil {
				t.Fatalf("Analyze failed: %v", err)
			}

			// Convert back to SQL
			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if sql == "" {
				t.Error("Expected non-empty SQL")
			}

			// Parse again to verify validity
			expr2, err := filter.Parse(sql)
			if err != nil {
				t.Fatalf("Re-parse failed: %v", err)
			}

			// Analyze again
			err = filter.Analyze(expr2)
			if err != nil {
				t.Fatalf("Re-analyze failed: %v", err)
			}

			t.Logf("Original: %s", tt.input)
			t.Logf("Generated: %s", sql)
		})
	}
}

// TestParseAllOperators tests parsing all supported operators
func TestParseAllOperators(t *testing.T) {
	operators := []struct {
		name     string
		input    string
		expected string
	}{
		{"Equality", "x == 'value'", "=="},
		{"Inequality", "x != 'value'", "!="},
		{"Less than", "x < 10", "<"},
		{"Less or equal", "x <= 10", "<="},
		{"Greater than", "x > 10", ">"},
		{"Greater or equal", "x >= 10", ">="},
		{"IN", "x in ('a', 'b')", "in"},
		{"LIKE", "x like '%pattern%'", "like"},
		{"AND", "a == 'x' and b == 'y'", "and"},
		{"OR", "a == 'x' or b == 'y'", "or"},
		{"NOT", "not (x == 'value')", "not"},
	}

	for _, op := range operators {
		t.Run(op.name, func(t *testing.T) {
			expr, err := filter.Parse(op.input)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			if expr == nil {
				t.Fatal("Expected non-nil expression")
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			if !strings.Contains(sql, op.expected) {
				t.Errorf("Expected SQL to contain '%s', got: %s", op.expected, sql)
			}
		})
	}
}

// TestParseWhitespace tests that whitespace is handled correctly
func TestParseWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"No extra whitespace", "name=='John'"},
		{"Normal whitespace", "name == 'John'"},
		{"Extra whitespace", "name   ==   'John'"},
		{"Tabs", "name\t==\t'John'"},
		{"Mixed whitespace", "name  \t ==  \t'John'"},
		{"Leading whitespace", "  name == 'John'"},
		{"Trailing whitespace", "name == 'John'  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := filter.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			if expr == nil {
				t.Fatal("Expected non-nil expression")
			}

			visitor := visitors.NewSQLLikeVisitor()
			visitor.Visit(expr)
			sql := visitor.SQL()

			// Should produce consistent output regardless of input whitespace
			expected := "name == 'John'"
			if sql != expected {
				t.Errorf("Expected '%s', got '%s'", expected, sql)
			}
		})
	}
}

// BenchmarkParse benchmarks the Parse function
func BenchmarkParse(b *testing.B) {
	input := "name == 'John' and age > 18"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = filter.Parse(input)
	}
}

// BenchmarkParseComplex benchmarks parsing complex expressions
func BenchmarkParseComplex(b *testing.B) {
	input := "user_type == 'individual' and (age >= 18 and name like '%tom' or verified == true) or not (status in ('suspended', 'banned'))"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = filter.Parse(input)
	}
}

// BenchmarkParseAndAnalyze benchmarks the combined operation
func BenchmarkParseAndAnalyze(b *testing.B) {
	input := "name == 'John' and age > 18 and status in ('active', 'pending')"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = filter.ParseAndAnalyze(input)
	}
}

// BenchmarkAnalyze benchmarks the Analyze function
func BenchmarkAnalyze(b *testing.B) {
	expr := filter.And(
		filter.EQ("name", "John"),
		filter.GT("age", 18),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.Analyze(expr)
	}
}

// Example demonstrates basic usage of Parse
func ExampleParse() {
	expr, err := filter.Parse("name == 'John' and age > 18")
	if err != nil {
		panic(err)
	}

	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(expr)
	println(visitor.SQL())
}

// Example demonstrates usage of ParseAndAnalyze
func ExampleParseAndAnalyze() {
	expr, err := filter.ParseAndAnalyze("status in ('active', 'pending') and verified == true")
	if err != nil {
		panic(err)
	}

	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(expr)
	println(visitor.SQL())
}

// Example demonstrates usage of Analyze
func ExampleAnalyze() {
	expr := filter.And(
		filter.EQ("name", "John"),
		filter.GT("age", 18),
	)

	err := filter.Analyze(expr)
	if err != nil {
		panic(err)
	}

	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(expr)
	println(visitor.SQL())
}

// TestParseEdgeCases tests edge cases in parsing
func TestParseEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Empty string value",
			input:   "name == ''",
			wantErr: false,
		},
		{
			name:    "Negative number",
			input:   "balance == -100",
			wantErr: false,
		},
		{
			name:    "Float number",
			input:   "price == 99.99",
			wantErr: false,
		},
		{
			name:    "Very long string",
			input:   "description == 'a very long string with many words and characters'",
			wantErr: false,
		},
		{
			name:    "Empty list",
			input:   "id in ()",
			wantErr: true,
		},
		{
			name:    "Nested parentheses",
			input:   "((a == 'x'))",
			wantErr: false,
		},
		{
			name:    "Multiple nested AND/OR",
			input:   "(a == '1' and b == '2') or (c == '3' and d == '4')",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := filter.Parse(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && expr == nil {
				t.Fatal("Expected non-nil expression")
			}
		})
	}
}

// TestAnalyzeNilExpression tests analyzing nil expression
func TestAnalyzeNilExpression(t *testing.T) {
	err := filter.Analyze(nil)
	if err == nil {
		t.Error("Expected error when analyzing nil expression")
	}
}

// TestParseAndAnalyzeEmptyString tests ParseAndAnalyze with empty string
func TestParseAndAnalyzeEmptyString(t *testing.T) {
	expr, err := filter.ParseAndAnalyze("")
	if err == nil {
		t.Error("Expected error for empty input")
	}
	if expr != nil {
		t.Error("Expected nil expression on error")
	}
}
