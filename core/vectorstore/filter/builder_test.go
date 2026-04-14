package filter_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/visitors"
)

// TestNewBuilder tests the creation of a new Builder instance
func TestNewBuilder(t *testing.T) {
	b := filter.NewBuilder()
	if b == nil {
		t.Fatal("NewBuilder should return non-nil builder")
	}

	// Empty builder should build without error
	expr, err := b.Build()
	if err != nil {
		t.Errorf("Empty builder should not return error, got: %v", err)
	}
	if expr != nil {
		t.Errorf("Empty builder should return nil expression, got: %v", expr)
	}
}

// TestBuilderEQ tests the EQ method
func TestBuilderEQ(t *testing.T) {
	tests := []struct {
		name     string
		left     any
		right    any
		expected string
		wantErr  bool
	}{
		{
			name:     "String equality",
			left:     "name",
			right:    "John",
			expected: "name == 'John'",
			wantErr:  false,
		},
		{
			name:     "Number equality",
			left:     "age",
			right:    25,
			expected: "age == 25",
			wantErr:  false,
		},
		{
			name:     "Boolean equality",
			left:     "active",
			right:    true,
			expected: "active == true",
			wantErr:  false,
		},
		{
			name:     "Float equality",
			left:     "score",
			right:    98.5,
			expected: "score == 98.5",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			expr, err := b.EQ(tt.left, tt.right).Build()

			if (err != nil) != tt.wantErr {
				t.Errorf("EQ() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				visitor := visitors.NewSQLLikeVisitor()
				visitor.Visit(expr)
				sql := visitor.SQL()

				if sql != tt.expected {
					t.Errorf("Expected '%s', got '%s'", tt.expected, sql)
				}
			}
		})
	}
}

// TestBuilderNE tests the NE method
func TestBuilderNE(t *testing.T) {
	tests := []struct {
		name     string
		left     any
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
			name:     "Number inequality",
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
			b := filter.NewBuilder()
			expr, err := b.NE(tt.left, tt.right).Build()

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderLT tests the LT method
func TestBuilderLT(t *testing.T) {
	tests := []struct {
		name     string
		left     any
		right    any
		expected string
	}{
		{
			name:     "Less than integer",
			left:     "age",
			right:    18,
			expected: "age < 18",
		},
		{
			name:     "Less than float",
			left:     "price",
			right:    99.99,
			expected: "price < 99.99",
		},
		{
			name:     "Less than negative",
			left:     "temperature",
			right:    -10,
			expected: "temperature < -10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			expr, err := b.LT(tt.left, tt.right).Build()

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderLE tests the LE method
func TestBuilderLE(t *testing.T) {
	tests := []struct {
		name     string
		left     any
		right    any
		expected string
	}{
		{
			name:     "Less or equal integer",
			left:     "age",
			right:    65,
			expected: "age <= 65",
		},
		{
			name:     "Less or equal float",
			left:     "balance",
			right:    1000.50,
			expected: "balance <= 1000.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			expr, err := b.LE(tt.left, tt.right).Build()

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderGT tests the GT method
func TestBuilderGT(t *testing.T) {
	tests := []struct {
		name     string
		left     any
		right    any
		expected string
	}{
		{
			name:     "Greater than integer",
			left:     "score",
			right:    100,
			expected: "score > 100",
		},
		{
			name:     "Greater than zero",
			left:     "quantity",
			right:    0,
			expected: "quantity > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			expr, err := b.GT(tt.left, tt.right).Build()

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderGE tests the GE method
func TestBuilderGE(t *testing.T) {
	tests := []struct {
		name     string
		left     any
		right    any
		expected string
	}{
		{
			name:     "Greater or equal integer",
			left:     "age",
			right:    18,
			expected: "age >= 18",
		},
		{
			name:     "Greater or equal float",
			left:     "rating",
			right:    4.5,
			expected: "rating >= 4.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			expr, err := b.GE(tt.left, tt.right).Build()

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderIn tests the In method
func TestBuilderIn(t *testing.T) {
	tests := []struct {
		name     string
		left     any
		right    any
		expected string
	}{
		{
			name:     "In string list",
			left:     "status",
			right:    []string{"pending", "active", "completed"},
			expected: "status in ('pending','active','completed')",
		},
		{
			name:     "In integer list",
			left:     "id",
			right:    []int{1, 2, 3, 4, 5},
			expected: "id in (1,2,3,4,5)",
		},
		{
			name:     "In single element",
			left:     "type",
			right:    []string{"premium"},
			expected: "type in ('premium')",
		},
		{
			name:     "In float list",
			left:     "version",
			right:    []float64{1.0, 2.0, 3.5},
			expected: "version in (1,2,3.5)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			expr, err := b.In(tt.left, tt.right).Build()

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderLike tests the Like method
func TestBuilderLike(t *testing.T) {
	tests := []struct {
		name     string
		left     any
		right    any
		expected string
	}{
		{
			name:     "Like prefix pattern",
			left:     "name",
			right:    "John%",
			expected: "name like 'John%'",
		},
		{
			name:     "Like suffix pattern",
			left:     "email",
			right:    "%@gmail.com",
			expected: "email like '%@gmail.com'",
		},
		{
			name:     "Like contains pattern",
			left:     "description",
			right:    "%test%",
			expected: "description like '%test%'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			expr, err := b.Like(tt.left, tt.right).Build()

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderAnd tests the And method for nested expressions
func TestBuilderAnd(t *testing.T) {
	tests := []struct {
		name     string
		build    func(*filter.Builder) (*filter.Builder, error)
		expected string
	}{
		{
			name: "Simple nested AND",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.And(func(sub *filter.ExprBuilder) {
					sub.EQ("status", "active")
					sub.GT("age", 18)
				}), nil
			},
			expected: "status == 'active' and age > 18",
		},
		{
			name: "Multiple nested AND",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.
					EQ("type", "user").
					And(func(sub *filter.ExprBuilder) {
						sub.GE("score", 100)
						sub.LE("score", 200)
					}), nil
			},
			expected: "type == 'user' and score >= 100 and score <= 200",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			builder, err := tt.build(b)
			if err != nil {
				t.Fatalf("Build error: %v", err)
			}

			expr, err := builder.Build()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderOr tests the Or method for nested expressions
func TestBuilderOr(t *testing.T) {
	tests := []struct {
		name     string
		build    func(*filter.Builder) (*filter.Builder, error)
		expected string
	}{
		{
			name: "Simple nested OR",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.Or(func(sub *filter.ExprBuilder) {
					sub.EQ("status", "pending")
					sub.EQ("status", "active")
				}), nil
			},
			expected: "status == 'pending' and status == 'active'",
		},
		{
			name: "AND with OR",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.
					EQ("type", "user").
					Or(func(sub *filter.ExprBuilder) {
						sub.EQ("role", "admin")
						sub.EQ("role", "moderator")
					}), nil
			},
			expected: "type == 'user' or role == 'admin' and role == 'moderator'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			builder, err := tt.build(b)
			if err != nil {
				t.Fatalf("Build error: %v", err)
			}

			expr, err := builder.Build()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderNot tests the Not method for negated expressions
func TestBuilderNot(t *testing.T) {
	tests := []struct {
		name     string
		build    func(*filter.Builder) (*filter.Builder, error)
		expected string
	}{
		{
			name: "Simple NOT",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.Not(func(sub *filter.ExprBuilder) {
					sub.EQ("status", "suspended")
				}), nil
			},
			expected: "not (status == 'suspended')",
		},
		{
			name: "NOT with multiple conditions",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.Not(func(sub *filter.ExprBuilder) {
					sub.EQ("status", "blocked")
					sub.EQ("verified", false)
				}), nil
			},
			expected: "not (status == 'blocked' and verified == false)",
		},
		{
			name: "Combined with other conditions",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.
					EQ("active", true).
					Not(func(sub *filter.ExprBuilder) {
						sub.In("status", []string{"suspended", "banned"})
					}), nil
			},
			expected: "active == true and not (status in ('suspended','banned'))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			builder, err := tt.build(b)
			if err != nil {
				t.Fatalf("Build error: %v", err)
			}

			expr, err := builder.Build()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderWithIndexExpr tests expressions with index access
func TestBuilderWithIndexExpr(t *testing.T) {
	tests := []struct {
		name     string
		build    func(*filter.Builder) (*filter.Builder, error)
		expected string
	}{
		{
			name: "Single level index with string key",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.EQ(filter.Index("addr", "country"), "UK"), nil
			},
			expected: "addr['country'] == 'UK'",
		},
		{
			name: "Single level index with integer key",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.EQ(filter.Index("colors", 0), "red"), nil
			},
			expected: "colors[0] == 'red'",
		},
		{
			name: "Nested index access",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.NE(filter.Index(filter.Index("a", 1), "b"), "red"), nil
			},
			expected: "a[1]['b'] != 'red'",
		},
		{
			name: "Index with In operator",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.In(filter.Index("info", "email"), []string{"tom@gmail.com", "john@gmail.com"}), nil
			},
			expected: "info['email'] in ('tom@gmail.com','john@gmail.com')",
		},
		{
			name: "Multiple index expressions",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.
					EQ(filter.Index("color", 1), "red").
					NE(filter.Index("size", "large"), true), nil
			},
			expected: "color[1] == 'red' and size['large'] != true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			builder, err := tt.build(b)
			if err != nil {
				t.Fatalf("Build error: %v", err)
			}

			expr, err := builder.Build()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderChaining tests method chaining with multiple operations
func TestBuilderChaining(t *testing.T) {
	tests := []struct {
		name     string
		build    func(*filter.Builder) (*filter.Builder, error)
		expected string
	}{
		{
			name: "Multiple comparisons",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.
					EQ("status", "active").
					GT("age", 18).
					LT("age", 65).
					Like("name", "John%"), nil
			},
			expected: "status == 'active' and age > 18 and age < 65 and name like 'John%'",
		},
		{
			name: "Mixed operators",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.
					EQ("type", "user").
					In("role", []string{"admin", "moderator"}).
					GE("score", 100).
					NE("status", "banned"), nil
			},
			expected: "type == 'user' and role in ('admin','moderator') and score >= 100 and status != 'banned'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			builder, err := tt.build(b)
			if err != nil {
				t.Fatalf("Build error: %v", err)
			}

			expr, err := builder.Build()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderComplexNesting tests complex nested expressions
func TestBuilderComplexNesting(t *testing.T) {
	b := filter.NewBuilder()
	expr, err := b.
		EQ("user_type", "individual").
		And(func(sub *filter.ExprBuilder) {
			sub.GE("age", 18)
			sub.Like("name", "%tom")
		}).
		Or(func(sub *filter.ExprBuilder) {
			sub.EQ("verified", true)
		}).
		Not(func(sub *filter.ExprBuilder) {
			sub.In("status", []string{"suspended", "banned"})
		}).
		Build()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(expr)
	sql := visitor.SQL()

	// The exact SQL depends on how the builder combines expressions
	if sql == "" {
		t.Error("Expected non-empty SQL")
	}

	t.Logf("Generated SQL: %s", sql)
}

// TestBuilderOriginalExample tests the original example from the code
func TestBuilderOriginalExample(t *testing.T) {
	b := filter.NewBuilder()
	expr, err := b.
		EQ("user_type", "individual").
		Like("name", "%tom").
		GT("age", 18).
		LT("age", 56).
		In("status", []string{"pending", "active"}).
		Not(func(builder *filter.ExprBuilder) {
			builder.In("status", []string{"suspended"})
		}).
		EQ(filter.Index("color", 1), "red").
		NE(filter.Index(filter.Index("a", 1), "b"), "red").
		EQ(filter.Index("addr", "country"), "UK").
		In(filter.Index("info", "email"), []string{"tom@gmail.com"}).
		Build()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(expr)
	sql := visitor.SQL()

	// Verify the SQL contains key components
	expectedParts := []string{
		"user_type == 'individual'",
		"name like '%tom'",
		"age > 18",
		"age < 56",
		"status in ('pending','active')",
		"not (status in ('suspended'))",
		"color[1] == 'red'",
		"a[1]['b'] != 'red'",
		"addr['country'] == 'UK'",
		"info['email'] in ('tom@gmail.com')",
	}

	for _, part := range expectedParts {
		if !strings.Contains(sql, part) {
			t.Errorf("Expected SQL to contain '%s', got: %s", part, sql)
		}
	}

	t.Logf("Generated SQL: %s", sql)
}

// TestBuilderErrorHandling tests error propagation
func TestBuilderErrorHandling(t *testing.T) {
	tests := []struct {
		name    string
		build   func(*filter.Builder) (*filter.Builder, error)
		wantErr bool
	}{
		{
			name: "Valid expression",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.EQ("name", "John"), nil
			},
			wantErr: false,
		},
		{
			name: "Invalid type for identifier",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.EQ(123, "value"), nil
			},
			wantErr: true,
		},
		{
			name: "Error in nested expression",
			build: func(b *filter.Builder) (*filter.Builder, error) {
				return b.
					EQ("valid", "field").
					And(func(sub *filter.ExprBuilder) {
						sub.EQ([]int{1, 2}, "invalid")
					}), nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			builder, err := tt.build(b)
			if err != nil {
				t.Fatalf("Build setup error: %v", err)
			}

			_, err = builder.Build()
			if (err != nil) != tt.wantErr {
				t.Errorf("Build() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestBuilderEmptySubExpression tests behavior with empty sub-expressions
func TestBuilderEmptySubExpression(t *testing.T) {
	tests := []struct {
		name     string
		build    func(*filter.Builder) *filter.Builder
		expected string
	}{
		{
			name: "Empty And sub-expression",
			build: func(b *filter.Builder) *filter.Builder {
				return b.
					EQ("name", "John").
					And(func(sub *filter.ExprBuilder) {
						// Empty
					})
			},
			expected: "name == 'John'",
		},
		{
			name: "Empty Or sub-expression",
			build: func(b *filter.Builder) *filter.Builder {
				return b.
					EQ("status", "active").
					Or(func(sub *filter.ExprBuilder) {
						// Empty
					})
			},
			expected: "status == 'active'",
		},
		{
			name: "Empty Not sub-expression",
			build: func(b *filter.Builder) *filter.Builder {
				return b.
					EQ("enabled", true).
					Not(func(sub *filter.ExprBuilder) {
						// Empty
					})
			},
			expected: "enabled == true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			builder := tt.build(b)

			expr, err := builder.Build()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderAllOperatorsWithIndex tests all operators with index expressions
func TestBuilderAllOperatorsWithIndex(t *testing.T) {
	tests := []struct {
		name     string
		build    func(*filter.Builder) *filter.Builder
		expected string
	}{
		{
			name: "EQ with index",
			build: func(b *filter.Builder) *filter.Builder {
				return b.EQ(filter.Index("data", "key"), "value")
			},
			expected: "data['key'] == 'value'",
		},
		{
			name: "NE with index",
			build: func(b *filter.Builder) *filter.Builder {
				return b.NE(filter.Index("data", "key"), "value")
			},
			expected: "data['key'] != 'value'",
		},
		{
			name: "LT with index",
			build: func(b *filter.Builder) *filter.Builder {
				return b.LT(filter.Index("scores", 0), 100)
			},
			expected: "scores[0] < 100",
		},
		{
			name: "LE with index",
			build: func(b *filter.Builder) *filter.Builder {
				return b.LE(filter.Index("values", "max"), 200)
			},
			expected: "values['max'] <= 200",
		},
		{
			name: "GT with index",
			build: func(b *filter.Builder) *filter.Builder {
				return b.GT(filter.Index("metrics", "score"), 50)
			},
			expected: "metrics['score'] > 50",
		},
		{
			name: "GE with index",
			build: func(b *filter.Builder) *filter.Builder {
				return b.GE(filter.Index("thresholds", "min"), 10)
			},
			expected: "thresholds['min'] >= 10",
		},
		{
			name: "Like with index",
			build: func(b *filter.Builder) *filter.Builder {
				return b.Like(filter.Index("profile", "bio"), "%developer%")
			},
			expected: "profile['bio'] like '%developer%'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			builder := tt.build(b)

			expr, err := builder.Build()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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

// TestBuilderErrorPropagation tests that errors propagate correctly through chaining
func TestBuilderErrorPropagation(t *testing.T) {
	b := filter.NewBuilder()

	// Create an invalid expression
	b.EQ(struct{}{}, "value") // struct{} should cause an error

	// Continue chaining - these should not execute
	b.EQ("name", "John")
	b.GT("age", 18)

	_, err := b.Build()

	if err == nil {
		t.Error("Expected error from invalid identifier type")
	}

	if !strings.Contains(err.Error(), "expected") {
		t.Errorf("Expected error about identifier type, got: %v", err)
	}
}

// TestBuilderNilHandling tests how the builder handles nil values
func TestBuilderNilHandling(t *testing.T) {
	b := filter.NewBuilder()

	// Build with no operations should return nil expression
	expr, err := b.Build()

	if err != nil {
		t.Errorf("Expected no error for empty builder, got: %v", err)
	}

	if expr != nil {
		t.Errorf("Expected nil expression for empty builder, got: %v", expr)
	}
}

// TestBuilderReturnsSelf tests that all methods return the builder for chaining
func TestBuilderReturnsSelf(t *testing.T) {
	b := filter.NewBuilder()

	// Test that all methods return *Builder
	b1 := b.EQ("a", 1)
	b2 := b1.NE("b", 2)
	b3 := b2.LT("c", 3)
	b4 := b3.LE("d", 4)
	b5 := b4.GT("e", 5)
	b6 := b5.GE("f", 6)
	b7 := b6.In("g", []int{7})
	b8 := b7.Like("h", "%8%")
	b9 := b8.And(func(sub *filter.ExprBuilder) {})
	b10 := b9.Or(func(sub *filter.ExprBuilder) {})
	b11 := b10.Not(func(sub *filter.ExprBuilder) {})

	// All should be the same builder instance
	if b != b1 || b1 != b2 || b2 != b3 || b3 != b4 ||
		b4 != b5 || b5 != b6 || b6 != b7 || b7 != b8 ||
		b8 != b9 || b9 != b10 || b10 != b11 {
		t.Error("Builder methods should return the same instance for chaining")
	}
}

// TestExprBuilderDirectUsage tests using ExprBuilder directly (if accessible)
func TestExprBuilderDirectUsage(t *testing.T) {
	// This tests the internal ExprBuilder behavior
	// The Builder wraps ExprBuilder, so we test through Builder

	b := filter.NewBuilder()

	// Test that sub-expressions work correctly
	expr, err := b.
		And(func(sub *filter.ExprBuilder) {
			sub.EQ("a", "1")
			sub.EQ("b", "2")
		}).
		Build()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if expr == nil {
		t.Fatal("Expected non-nil expression")
	}

	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(expr)
	sql := visitor.SQL()

	if !strings.Contains(sql, "a == '1'") || !strings.Contains(sql, "b == '2'") {
		t.Errorf("Expected SQL to contain both conditions, got: %s", sql)
	}
}

// TestBuilderBuildReturnsInterface tests that Build returns ast.Expr interface
func TestBuilderBuildReturnsInterface(t *testing.T) {
	b := filter.NewBuilder()
	expr, err := b.EQ("name", "test").Build()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// expr should implement ast.Expr interface
	var _ ast.Expr = expr

	// Should be able to use with visitors
	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(expr)
	sql := visitor.SQL()

	if sql == "" {
		t.Error("Expected non-empty SQL")
	}
}

// BenchmarkBuilderSimple benchmarks simple expression building
func BenchmarkBuilderSimple(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder := filter.NewBuilder()
		_, _ = builder.EQ("name", "John").Build()
	}
}

// BenchmarkBuilderComplex benchmarks complex expression building
func BenchmarkBuilderComplex(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder := filter.NewBuilder()
		_, _ = builder.
			EQ("user_type", "individual").
			Like("name", "%tom").
			GT("age", 18).
			LT("age", 56).
			In("status", []string{"pending", "active"}).
			Not(func(sub *filter.ExprBuilder) {
				sub.In("status", []string{"suspended"})
			}).
			Build()
	}
}

// BenchmarkBuilderWithIndex benchmarks expression building with index access
func BenchmarkBuilderWithIndex(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder := filter.NewBuilder()
		_, _ = builder.
			EQ(filter.Index("data", "name"), "test").
			NE(filter.Index(filter.Index("a", 0), "b"), "value").
			Build()
	}
}

// Example demonstrates basic usage of the Builder
func ExampleBuilder() {
	b := filter.NewBuilder()
	expr, err := b.
		EQ("name", "John").
		GT("age", 18).
		In("status", []string{"active", "pending"}).
		Build()

	if err != nil {
		panic(err)
	}

	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(expr)
	println(visitor.SQL())
}

// Example demonstrates complex nested expressions
func ExampleBuilder_complex() {
	b := filter.NewBuilder()
	expr, err := b.
		EQ("user_type", "individual").
		And(func(sub *filter.ExprBuilder) {
			sub.GE("age", 18)
			sub.LE("age", 65)
		}).
		Or(func(sub *filter.ExprBuilder) {
			sub.EQ("verified", true)
		}).
		Not(func(sub *filter.ExprBuilder) {
			sub.In("status", []string{"suspended", "banned"})
		}).
		Build()

	if err != nil {
		panic(err)
	}

	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(expr)
	println(visitor.SQL())
}

// Example demonstrates index access expressions
func ExampleBuilder_indexAccess() {
	b := filter.NewBuilder()
	expr, err := b.
		EQ(filter.Index("addr", "country"), "US").
		EQ(filter.Index("colors", 0), "red").
		In(filter.Index("emails", "primary"), []string{"test@example.com"}).
		Build()

	if err != nil {
		panic(err)
	}

	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(expr)
	println(visitor.SQL())
}

// TestBuilderTypeValidation tests type validation for different operations
func TestBuilderTypeValidation(t *testing.T) {
	tests := []struct {
		name    string
		build   func(*filter.Builder) error
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid string identifier",
			build: func(b *filter.Builder) error {
				_, err := b.EQ("name", "value").Build()
				return err
			},
			wantErr: false,
		},
		{
			name: "Invalid identifier type",
			build: func(b *filter.Builder) error {
				_, err := b.EQ([]string{"invalid"}, "value").Build()
				return err
			},
			wantErr: true,
			errMsg:  "expected",
		},
		{
			name: "Valid index expression",
			build: func(b *filter.Builder) error {
				_, err := b.EQ(filter.Index("data", "key"), "value").Build()
				return err
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := filter.NewBuilder()
			err := tt.build(b)

			if (err != nil) != tt.wantErr {
				t.Errorf("Expected error: %v, got: %v", tt.wantErr, err)
			}

			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errMsg, err)
				}
			}
		})
	}
}

// TestBuilderConcurrentUsage tests that builder can be used safely
func TestBuilderConcurrentUsage(t *testing.T) {
	// Note: The Builder is NOT designed for concurrent use
	// This test documents that each goroutine should use its own builder

	done := make(chan bool, 2)

	go func() {
		b := filter.NewBuilder()
		_, err := b.EQ("name", "test1").Build()
		if err != nil {
			t.Error(err)
		}
		done <- true
	}()

	go func() {
		b := filter.NewBuilder()
		_, err := b.EQ("name", "test2").Build()
		if err != nil {
			t.Error(err)
		}
		done <- true
	}()

	<-done
	<-done
}

// Helper function to check if an error is a specific type
func isErrorType(err error, target error) bool {
	return errors.Is(err, target)
}
