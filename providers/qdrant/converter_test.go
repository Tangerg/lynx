package qdrant

import (
	"testing"

	"github.com/qdrant/go-client/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// Test: Basic Converter Creation and Methods

func TestNewConverter(t *testing.T) {
	conv := NewConverter()

	assert.NotNil(t, conv)
	assert.NotNil(t, conv.Filter())
	assert.Nil(t, conv.Error())
	assert.Nil(t, conv.currentFieldValue)
	assert.Empty(t, conv.currentFieldKey)
}

func TestConverter_Filter(t *testing.T) {
	conv := NewConverter()
	result := conv.Filter()

	assert.NotNil(t, result)
	assert.Empty(t, result.Must)
	assert.Empty(t, result.Should)
	assert.Empty(t, result.MustNot)
}

func TestConverter_Error(t *testing.T) {
	conv := NewConverter()
	assert.Nil(t, conv.Error())

	// Trigger an error
	conv.Visit(nil)
	assert.Error(t, conv.Error())
	assert.Contains(t, conv.Error().Error(), "nil expression")
}

// Test: Visit Methods for Atomic Expressions

func TestConverter_visitIdent(t *testing.T) {
	tests := []struct {
		name     string
		ident    *ast.Ident
		expected string
	}{
		{
			name:     "simple identifier",
			ident:    filter.NewIdent("age"),
			expected: "age",
		},
		{
			name:     "complex identifier",
			ident:    filter.NewIdent("user_status"),
			expected: "user_status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewConverter()
			err := conv.visitIdent(tt.ident)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, conv.currentFieldKey)
		})
	}
}

func TestConverter_visitLiteral(t *testing.T) {
	tests := []struct {
		name     string
		literal  *ast.Literal
		expected any
	}{
		{
			name:     "string literal",
			literal:  filter.NewLiteral("active"),
			expected: "active",
		},
		{
			name:     "number literal - int",
			literal:  filter.NewLiteral(42),
			expected: 42.0,
		},
		{
			name:     "number literal - float",
			literal:  filter.NewLiteral(42.5),
			expected: 42.5,
		},
		{
			name:     "true literal",
			literal:  filter.NewLiteral(true),
			expected: true,
		},
		{
			name:     "false literal",
			literal:  filter.NewLiteral(false),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewConverter()
			err := conv.visitLiteral(tt.literal)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, conv.currentFieldValue)
		})
	}
}

func TestConverter_visitListLiteral(t *testing.T) {
	tests := []struct {
		name     string
		list     *ast.ListLiteral
		expected []any
	}{
		{
			name:     "string list",
			list:     filter.NewListLiteral([]string{"active", "pending"}),
			expected: []any{"active", "pending"},
		},
		{
			name:     "int list",
			list:     filter.NewListLiteral([]int{1, 2, 3}),
			expected: []any{1.0, 2.0, 3.0},
		},
		{
			name:     "float list",
			list:     filter.NewListLiteral([]float64{1.5, 2.5, 3.5}),
			expected: []any{1.5, 2.5, 3.5},
		},
		{
			name:     "bool list",
			list:     filter.NewListLiteral([]bool{true, false}),
			expected: []any{true, false},
		},
		{
			name: "literal list",
			list: filter.NewListLiteral([]*ast.Literal{
				filter.NewLiteral("a"),
				filter.NewLiteral("b"),
			}),
			expected: []any{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewConverter()
			err := conv.visitListLiteral(tt.list)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, conv.currentFieldValue)
		})
	}
}

func TestConverter_visitListLiteral_EmptyList(t *testing.T) {
	list := filter.NewListLiteral([]string{})
	conv := NewConverter()
	err := conv.visitListLiteral(list)

	require.NoError(t, err)
	assert.Equal(t, []any{}, conv.currentFieldValue)
}

// Test: Index Expressions

func TestConverter_visitIndexExpr_Simple(t *testing.T) {
	// user["name"]
	expr := filter.Index("user", "name")

	conv := NewConverter()
	err := conv.visitIndexExpr(expr)

	require.NoError(t, err)
	assert.Equal(t, "user.name", conv.currentFieldKey)
}

func TestConverter_visitIndexExpr_Nested(t *testing.T) {
	// metadata["user"]["age"]
	expr := filter.Index(
		filter.Index("metadata", "user"),
		"age",
	)

	conv := NewConverter()
	err := conv.visitIndexExpr(expr)

	require.NoError(t, err)
	assert.Equal(t, "metadata.user.age", conv.currentFieldKey)
}

func TestConverter_visitIndexExpr_NumericIndex(t *testing.T) {
	// tags[0]
	expr := filter.Index("tags", 0)

	conv := NewConverter()
	err := conv.visitIndexExpr(expr)

	require.NoError(t, err)
	assert.Equal(t, "tags.0", conv.currentFieldKey)
}

func TestConverter_visitIndexExpr_DeepNesting(t *testing.T) {
	// data["level1"]["level2"]["level3"]
	expr := filter.Index(
		filter.Index(
			filter.Index("data", "level1"),
			"level2",
		),
		"level3",
	)

	conv := NewConverter()
	err := conv.visitIndexExpr(expr)

	require.NoError(t, err)
	assert.Equal(t, "data.level1.level2.level3", conv.currentFieldKey)
}

func TestConverter_buildIndexedFieldKey_MixedIndices(t *testing.T) {
	// arr[0]["key"][1]
	expr := filter.Index(
		filter.Index(
			filter.Index("arr", 0),
			"key",
		),
		1,
	)

	conv := NewConverter()
	fieldKey, err := conv.buildIndexedFieldKey(expr)

	require.NoError(t, err)
	assert.Equal(t, "arr.0.key.1", fieldKey)
}

// Test: Equality Operators

func TestConverter_visitEqualityExpr_Equal(t *testing.T) {
	tests := []struct {
		name      string
		expr      *ast.BinaryExpr
		checkCond func(t *testing.T, filter *qdrant.Filter)
	}{
		{
			name: "string equality",
			expr: filter.EQ("status", "active"),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				assert.NotNil(t, cond.GetField())
				assert.NotNil(t, cond.GetField().GetMatch())
				assert.Equal(t, "active", cond.GetField().GetMatch().GetKeyword())
			},
		},
		{
			name: "number equality - int",
			expr: filter.EQ("age", 25),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				assert.NotNil(t, cond.GetField())
				assert.NotNil(t, cond.GetField().GetMatch())
				assert.Equal(t, int64(25), cond.GetField().GetMatch().GetInteger())
			},
		},
		{
			name: "number equality - float",
			expr: filter.EQ("score", 98.5),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				assert.NotNil(t, cond.GetField())
				assert.NotNil(t, cond.GetField().GetMatch())
				// Float is cast to int64
				assert.Equal(t, int64(98), cond.GetField().GetMatch().GetInteger())
			},
		},
		{
			name: "bool equality - true",
			expr: filter.EQ("active", true),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				assert.NotNil(t, cond.GetField())
				assert.NotNil(t, cond.GetField().GetMatch())
				assert.True(t, cond.GetField().GetMatch().GetBoolean())
			},
		},
		{
			name: "bool equality - false",
			expr: filter.EQ("deleted", false),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				assert.NotNil(t, cond.GetField())
				assert.NotNil(t, cond.GetField().GetMatch())
				assert.False(t, cond.GetField().GetMatch().GetBoolean())
			},
		},
		{
			name: "indexed field equality",
			expr: filter.EQ(filter.Index("user", "status"), "active"),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				assert.NotNil(t, cond.GetField())
				assert.NotNil(t, cond.GetField().GetMatch())
				assert.Equal(t, "active", cond.GetField().GetMatch().GetKeyword())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewConverter()
			err := conv.visitEqualityExpr(tt.expr)

			require.NoError(t, err)
			tt.checkCond(t, conv.filter)
		})
	}
}

func TestConverter_visitEqualityExpr_NotEqual(t *testing.T) {
	tests := []struct {
		name      string
		expr      *ast.BinaryExpr
		checkCond func(t *testing.T, filter *qdrant.Filter)
	}{
		{
			name: "string inequality",
			expr: filter.NE("status", "inactive"),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.MustNot, 1)
				cond := f.MustNot[0]
				assert.NotNil(t, cond.GetField())
				assert.NotNil(t, cond.GetField().GetMatch())
				assert.Equal(t, "inactive", cond.GetField().GetMatch().GetKeyword())
			},
		},
		{
			name: "number inequality",
			expr: filter.NE("age", 0),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.MustNot, 1)
				cond := f.MustNot[0]
				assert.NotNil(t, cond.GetField())
				assert.NotNil(t, cond.GetField().GetMatch())
				assert.Equal(t, int64(0), cond.GetField().GetMatch().GetInteger())
			},
		},
		{
			name: "bool inequality",
			expr: filter.NE("active", false),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.MustNot, 1)
				cond := f.MustNot[0]
				assert.NotNil(t, cond.GetField())
				assert.NotNil(t, cond.GetField().GetMatch())
				assert.False(t, cond.GetField().GetMatch().GetBoolean())
			},
		},
		{
			name: "indexed field inequality",
			expr: filter.NE(filter.Index("metadata", "deleted"), true),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.MustNot, 1)
				cond := f.MustNot[0]
				assert.NotNil(t, cond.GetField())
				assert.NotNil(t, cond.GetField().GetMatch())
				assert.True(t, cond.GetField().GetMatch().GetBoolean())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewConverter()
			err := conv.visitEqualityExpr(tt.expr)

			require.NoError(t, err)
			tt.checkCond(t, conv.filter)
		})
	}
}

// Test: Ordering Operators

func TestConverter_visitOrderingExpr(t *testing.T) {
	tests := []struct {
		name      string
		expr      *ast.BinaryExpr
		checkCond func(t *testing.T, filter *qdrant.Filter)
	}{
		{
			name: "less than - int",
			expr: filter.LT("age", 30),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				rng := cond.GetField().GetRange()
				assert.NotNil(t, rng)
				assert.NotNil(t, rng.Lt)
				assert.Equal(t, 30.0, *rng.Lt)
			},
		},
		{
			name: "less than - float",
			expr: filter.LT("price", 99.99),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				rng := cond.GetField().GetRange()
				assert.NotNil(t, rng)
				assert.NotNil(t, rng.Lt)
				assert.Equal(t, 99.99, *rng.Lt)
			},
		},
		{
			name: "less than or equal",
			expr: filter.LE("score", 100),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				rng := cond.GetField().GetRange()
				assert.NotNil(t, rng)
				assert.NotNil(t, rng.Lte)
				assert.Equal(t, 100.0, *rng.Lte)
			},
		},
		{
			name: "greater than",
			expr: filter.GT("age", 18),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				rng := cond.GetField().GetRange()
				assert.NotNil(t, rng)
				assert.NotNil(t, rng.Gt)
				assert.Equal(t, 18.0, *rng.Gt)
			},
		},
		{
			name: "greater than or equal",
			expr: filter.GE("count", 5),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				rng := cond.GetField().GetRange()
				assert.NotNil(t, rng)
				assert.NotNil(t, rng.Gte)
				assert.Equal(t, 5.0, *rng.Gte)
			},
		},
		{
			name: "indexed field comparison",
			expr: filter.GT(filter.Index("stats", "views"), 1000),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				rng := cond.GetField().GetRange()
				assert.NotNil(t, rng)
				assert.NotNil(t, rng.Gt)
				assert.Equal(t, 1000.0, *rng.Gt)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewConverter()
			err := conv.visitOrderingExpr(tt.expr)

			require.NoError(t, err)
			tt.checkCond(t, conv.filter)
		})
	}
}

func TestConverter_visitOrderingExpr_InvalidValue(t *testing.T) {
	// Create an expression with string literal (should fail for ordering)
	expr := &ast.BinaryExpr{
		Left: filter.NewIdent("age"),
		Op: token.Token{
			Kind:    token.GT,
			Literal: ">",
			Start:   token.NoPosition,
			End:     token.NoPosition,
		},
		Right: filter.NewLiteral("invalid"),
	}

	conv := NewConverter()
	err := conv.visitOrderingExpr(expr)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot convert value to number")
}

// Test: IN Operator

func TestConverter_visitInExpr(t *testing.T) {
	tests := []struct {
		name      string
		expr      *ast.BinaryExpr
		checkCond func(t *testing.T, filter *qdrant.Filter)
	}{
		{
			name: "string IN",
			expr: filter.In("status", []string{"active", "pending", "approved"}),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				match := cond.GetField().GetMatch()
				assert.NotNil(t, match)
				keywords := match.GetKeywords().GetStrings()
				assert.Len(t, keywords, 3)
				assert.Contains(t, keywords, "active")
				assert.Contains(t, keywords, "pending")
				assert.Contains(t, keywords, "approved")
			},
		},
		{
			name: "int IN",
			expr: filter.In("id", []int{1, 2, 3, 4, 5}),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				match := cond.GetField().GetMatch()
				assert.NotNil(t, match)
				integers := match.GetIntegers().GetIntegers()
				assert.Len(t, integers, 5)
				assert.Contains(t, integers, int64(1))
				assert.Contains(t, integers, int64(3))
				assert.Contains(t, integers, int64(5))
			},
		},
		{
			name: "int64 IN",
			expr: filter.In("id", []int64{100, 200, 300}),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				match := cond.GetField().GetMatch()
				assert.NotNil(t, match)
				integers := match.GetIntegers().GetIntegers()
				assert.Len(t, integers, 3)
				assert.Contains(t, integers, int64(100))
				assert.Contains(t, integers, int64(200))
			},
		},
		{
			name: "float IN",
			expr: filter.In("score", []float64{1.5, 2.5, 3.5}),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				match := cond.GetField().GetMatch()
				assert.NotNil(t, match)
				integers := match.GetIntegers().GetIntegers()
				assert.Len(t, integers, 3)
				// Floats are cast to int64
				assert.Contains(t, integers, int64(1))
				assert.Contains(t, integers, int64(2))
				assert.Contains(t, integers, int64(3))
			},
		},
		{
			name: "bool IN",
			expr: filter.In("flag", []bool{true, false}),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				nestedFilter := cond.GetFilter()
				assert.NotNil(t, nestedFilter)
				assert.Len(t, nestedFilter.Should, 2)
			},
		},
		{
			name: "indexed field IN",
			expr: filter.In(filter.Index("user", "role"), []string{"admin", "owner"}),
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				match := cond.GetField().GetMatch()
				assert.NotNil(t, match)
				keywords := match.GetKeywords().GetStrings()
				assert.Len(t, keywords, 2)
				assert.Contains(t, keywords, "admin")
				assert.Contains(t, keywords, "owner")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewConverter()
			err := conv.visitInExpr(tt.expr)

			require.NoError(t, err)
			tt.checkCond(t, conv.filter)
		})
	}
}

func TestConverter_visitInExpr_EmptyList(t *testing.T) {
	expr := filter.In("status", []string{})

	conv := NewConverter()
	err := conv.visitInExpr(expr)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty list")
}

func TestConverter_visitInExpr_NotAList(t *testing.T) {
	// Manually create invalid expression (not a list)
	expr := &ast.BinaryExpr{
		Left: filter.NewIdent("status"),
		Op: token.Token{
			Kind:    token.IN,
			Literal: "in",
			Start:   token.NoPosition,
			End:     token.NoPosition,
		},
		Right: filter.NewLiteral("active"),
	}

	conv := NewConverter()
	err := conv.visitInExpr(expr)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires a list")
}

// Test: LIKE Operator

func TestConverter_visitLikeExpr(t *testing.T) {
	tests := []struct {
		name      string
		expr      *ast.BinaryExpr
		pattern   string
		checkCond func(t *testing.T, filter *qdrant.Filter)
	}{
		{
			name:    "prefix pattern",
			expr:    filter.Like("name", "John%"),
			pattern: "John%",
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				match := cond.GetField().GetMatch()
				assert.NotNil(t, match)
				assert.Equal(t, "John%", match.GetText())
			},
		},
		{
			name:    "suffix pattern",
			expr:    filter.Like("email", "%@gmail.com"),
			pattern: "%@gmail.com",
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				match := cond.GetField().GetMatch()
				assert.NotNil(t, match)
				assert.Equal(t, "%@gmail.com", match.GetText())
			},
		},
		{
			name:    "contains pattern",
			expr:    filter.Like("description", "%keyword%"),
			pattern: "%keyword%",
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				match := cond.GetField().GetMatch()
				assert.NotNil(t, match)
				assert.Equal(t, "%keyword%", match.GetText())
			},
		},
		{
			name:    "indexed field pattern",
			expr:    filter.Like(filter.Index("profile", "bio"), "%developer%"),
			pattern: "%developer%",
			checkCond: func(t *testing.T, f *qdrant.Filter) {
				require.Len(t, f.Must, 1)
				cond := f.Must[0]
				match := cond.GetField().GetMatch()
				assert.NotNil(t, match)
				assert.Equal(t, "%developer%", match.GetText())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewConverter()
			err := conv.visitLikeExpr(tt.expr)

			require.NoError(t, err)
			tt.checkCond(t, conv.filter)
		})
	}
}

func TestConverter_visitLikeExpr_NotString(t *testing.T) {
	// Manually create invalid expression (not a string)
	expr := &ast.BinaryExpr{
		Left: filter.NewIdent("name"),
		Op: token.Token{
			Kind:    token.LIKE,
			Literal: "like",
			Start:   token.NoPosition,
			End:     token.NoPosition,
		},
		Right: filter.NewLiteral(123),
	}

	conv := NewConverter()
	err := conv.visitLikeExpr(expr)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires a string")
}

// Test: Logical Operators

func TestConverter_visitLogicalExpr_AND(t *testing.T) {
	// age > 18 AND status == "active"
	expr := filter.And(
		filter.GT("age", 18),
		filter.EQ("status", "active"),
	)

	conv := NewConverter()
	err := conv.visitLogicalExpr(expr)

	require.NoError(t, err)
	assert.Len(t, conv.filter.Must, 2)
	assert.Empty(t, conv.filter.Should)
	assert.Empty(t, conv.filter.MustNot)
}

func TestConverter_visitLogicalExpr_OR(t *testing.T) {
	// status == "active" OR status == "pending"
	expr := filter.Or(
		filter.EQ("status", "active"),
		filter.EQ("status", "pending"),
	)

	conv := NewConverter()
	err := conv.visitLogicalExpr(expr)

	require.NoError(t, err)
	assert.Empty(t, conv.filter.Must)
	assert.Len(t, conv.filter.Should, 2)
	assert.Empty(t, conv.filter.MustNot)
}

func TestConverter_visitLogicalExpr_MultipleAND(t *testing.T) {
	// age > 18 AND status == "active" AND score >= 80
	expr := filter.And(
		filter.And(
			filter.GT("age", 18),
			filter.EQ("status", "active"),
		),
		filter.GE("score", 80),
	)

	conv := NewConverter()
	err := conv.visit(expr)

	require.NoError(t, err)
	// The structure might be nested, so we check that conditions exist
	assert.NotEmpty(t, conv.filter.Must)
}

func TestConverter_visitLogicalExpr_MultipleOR(t *testing.T) {
	// role == "admin" OR role == "owner" OR role == "moderator"
	expr := filter.Or(
		filter.Or(
			filter.EQ("role", "admin"),
			filter.EQ("role", "owner"),
		),
		filter.EQ("role", "moderator"),
	)

	conv := NewConverter()
	err := conv.visit(expr)

	require.NoError(t, err)
	assert.NotEmpty(t, conv.filter.Should)
}

// Test: NOT Operator

func TestConverter_visitNotExpr(t *testing.T) {
	tests := []struct {
		name string
		expr *ast.UnaryExpr
	}{
		{
			name: "NOT equality",
			expr: filter.Not(filter.EQ("deleted", true)),
		},
		{
			name: "NOT comparison",
			expr: filter.Not(filter.GT("age", 65)),
		},
		{
			name: "NOT IN",
			expr: filter.Not(filter.In("status", []string{"blocked", "suspended"})),
		},
		{
			name: "NOT indexed field",
			expr: filter.Not(filter.EQ(filter.Index("flags", "hidden"), true)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewConverter()
			err := conv.visitNotExpr(tt.expr)

			require.NoError(t, err)
			assert.Len(t, conv.filter.MustNot, 1)
			assert.Empty(t, conv.filter.Must)
			assert.Empty(t, conv.filter.Should)
		})
	}
}

func TestConverter_visitNotExpr_Nested(t *testing.T) {
	// NOT (age > 18 AND status == "active")
	expr := filter.Not(
		filter.And(
			filter.GT("age", 18),
			filter.EQ("status", "active"),
		),
	)

	conv := NewConverter()
	err := conv.visitNotExpr(expr)

	require.NoError(t, err)
	assert.Len(t, conv.filter.MustNot, 1)

	// Check that the negated condition contains a nested filter
	notCond := conv.filter.MustNot[0]
	assert.NotNil(t, notCond.GetFilter())
}

// Test: Complex Expressions

func TestConverter_ComplexExpression_AndOr(t *testing.T) {
	// (age > 18 AND status == "active") OR (vip == true)
	expr := filter.Or(
		filter.And(
			filter.GT("age", 18),
			filter.EQ("status", "active"),
		),
		filter.EQ("vip", true),
	)

	conv := NewConverter()
	err := conv.visit(expr)

	require.NoError(t, err)
	assert.Len(t, conv.filter.Should, 2)
}

func TestConverter_ComplexExpression_NestedLogical(t *testing.T) {
	// age > 18 AND (status == "active" OR status == "pending")
	expr := filter.And(
		filter.GT("age", 18),
		filter.Or(
			filter.EQ("status", "active"),
			filter.EQ("status", "pending"),
		),
	)

	conv := NewConverter()
	err := conv.visit(expr)

	require.NoError(t, err)
	assert.Len(t, conv.filter.Must, 2)

	// Check that one of the Must conditions is a nested OR filter
	hasNestedFilter := false
	for _, cond := range conv.filter.Must {
		if cond.GetFilter() != nil {
			hasNestedFilter = true
			nestedFilter := cond.GetFilter()
			if len(nestedFilter.GetShould()) == 0 {
				continue
			}
			assert.Len(t, nestedFilter.Should, 2)
		}
	}
	assert.True(t, hasNestedFilter, "Expected nested OR filter in Must conditions")
}

func TestConverter_ComplexExpression_WithIndexedFields(t *testing.T) {
	// user["age"] > 18 AND user["status"] IN ["active", "pending"]
	expr := filter.And(
		filter.GT(filter.Index("user", "age"), 18),
		filter.In(filter.Index("user", "status"), []string{"active", "pending"}),
	)

	conv := NewConverter()
	err := conv.visit(expr)

	require.NoError(t, err)
	assert.Len(t, conv.filter.Must, 2)
}

func TestConverter_ComplexExpression_NotWithLogical(t *testing.T) {
	// NOT (age < 18 OR score < 60)
	expr := filter.Not(
		filter.Or(
			filter.LT("age", 18),
			filter.LT("score", 60),
		),
	)

	conv := NewConverter()
	err := conv.visit(expr)

	require.NoError(t, err)
	assert.Len(t, conv.filter.MustNot, 1)

	notCond := conv.filter.MustNot[0]
	nestedFilter := notCond.GetFilter()
	assert.NotNil(t, nestedFilter)
	assert.Len(t, nestedFilter.Should, 2)
}

func TestConverter_ComplexExpression_DeepNesting(t *testing.T) {
	// ((a AND b) OR (c AND d)) AND (e OR f)
	expr := filter.And(
		filter.Or(
			filter.And(
				filter.EQ("a", 1),
				filter.EQ("b", 2),
			),
			filter.And(
				filter.EQ("c", 3),
				filter.EQ("d", 4),
			),
		),
		filter.Or(
			filter.EQ("e", 5),
			filter.EQ("f", 6),
		),
	)

	conv := NewConverter()
	err := conv.visit(expr)

	require.NoError(t, err)
	assert.Len(t, conv.filter.Must, 2)
}

// Test: ToFilter Function

func TestToFilter_Success(t *testing.T) {
	tests := []struct {
		name  string
		expr  ast.Expr
		check func(t *testing.T, filter *qdrant.Filter)
	}{
		{
			name: "simple equality",
			expr: filter.EQ("status", "active"),
			check: func(t *testing.T, f *qdrant.Filter) {
				assert.NotNil(t, f)
				assert.Len(t, f.Must, 1)
			},
		},
		{
			name: "AND expression",
			expr: filter.And(
				filter.GT("age", 18),
				filter.EQ("status", "active"),
			),
			check: func(t *testing.T, f *qdrant.Filter) {
				assert.NotNil(t, f)
				assert.Len(t, f.Must, 2)
			},
		},
		{
			name: "OR expression",
			expr: filter.Or(
				filter.EQ("role", "admin"),
				filter.EQ("role", "owner"),
			),
			check: func(t *testing.T, f *qdrant.Filter) {
				assert.NotNil(t, f)
				assert.Len(t, f.Should, 2)
			},
		},
		{
			name: "NOT expression",
			expr: filter.Not(filter.EQ("deleted", true)),
			check: func(t *testing.T, f *qdrant.Filter) {
				assert.NotNil(t, f)
				assert.Len(t, f.MustNot, 1)
			},
		},
		{
			name: "IN expression",
			expr: filter.In("status", []string{"active", "pending", "approved"}),
			check: func(t *testing.T, f *qdrant.Filter) {
				assert.NotNil(t, f)
				assert.Len(t, f.Must, 1)
			},
		},
		{
			name: "LIKE expression",
			expr: filter.Like("name", "John%"),
			check: func(t *testing.T, f *qdrant.Filter) {
				assert.NotNil(t, f)
				assert.Len(t, f.Must, 1)
			},
		},
		{
			name: "indexed field",
			expr: filter.EQ(filter.Index("user", "status"), "active"),
			check: func(t *testing.T, f *qdrant.Filter) {
				assert.NotNil(t, f)
				assert.Len(t, f.Must, 1)
			},
		},
		{
			name: "complex nested",
			expr: filter.And(
				filter.GT("age", 18),
				filter.Or(
					filter.EQ("status", "active"),
					filter.EQ("status", "pending"),
				),
			),
			check: func(t *testing.T, f *qdrant.Filter) {
				assert.NotNil(t, f)
				assert.Len(t, f.Must, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ToFilter(tt.expr)

			require.NoError(t, err)
			require.NotNil(t, result)
			tt.check(t, result)
		})
	}
}

func TestToFilter_NilExpression(t *testing.T) {
	result, err := ToFilter(nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil expression")
}

func TestToFilter_UnsupportedExpression(t *testing.T) {
	// Create an unsupported expression type
	type UnsupportedExpr struct {
		ast.Expr
	}

	unsupported := &UnsupportedExpr{}
	result, err := ToFilter(unsupported)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unsupported expression type")
}

// Test: Helper Methods

func TestConverter_extractFieldKey(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		expected string
	}{
		{
			name:     "simple identifier",
			expr:     filter.NewIdent("age"),
			expected: "age",
		},
		{
			name:     "single index",
			expr:     filter.Index("user", "name"),
			expected: "user.name",
		},
		{
			name:     "nested index",
			expr:     filter.Index(filter.Index("data", "user"), "email"),
			expected: "data.user.email",
		},
		{
			name:     "numeric index",
			expr:     filter.Index("items", 0),
			expected: "items.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewConverter()
			key, err := conv.extractFieldKey(tt.expr)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, key)
		})
	}
}

func TestConverter_extractFieldValue(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		expected any
	}{
		{
			name:     "string literal",
			expr:     filter.NewLiteral("test"),
			expected: "test",
		},
		{
			name:     "int literal",
			expr:     filter.NewLiteral(42),
			expected: 42.0,
		},
		{
			name:     "float literal",
			expr:     filter.NewLiteral(3.14),
			expected: 3.14,
		},
		{
			name:     "bool literal - true",
			expr:     filter.NewLiteral(true),
			expected: true,
		},
		{
			name:     "bool literal - false",
			expr:     filter.NewLiteral(false),
			expected: false,
		},
		{
			name:     "string list",
			expr:     filter.NewListLiteral([]string{"a", "b", "c"}),
			expected: []any{"a", "b", "c"},
		},
		{
			name:     "int list",
			expr:     filter.NewListLiteral([]int{1, 2, 3}),
			expected: []any{1.0, 2.0, 3.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewConverter()
			value, err := conv.extractFieldValue(tt.expr)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, value)
		})
	}
}

func TestConverter_buildNestedCondition(t *testing.T) {
	tests := []struct {
		name  string
		expr  ast.Expr
		check func(t *testing.T, cond *qdrant.Condition)
	}{
		{
			name: "simple comparison",
			expr: filter.GT("age", 18),
			check: func(t *testing.T, cond *qdrant.Condition) {
				assert.NotNil(t, cond)
				nestedFilter := cond.GetFilter()
				assert.NotNil(t, nestedFilter)
				assert.Len(t, nestedFilter.Must, 1)
			},
		},
		{
			name: "nested AND",
			expr: filter.And(
				filter.EQ("status", "active"),
				filter.GT("age", 18),
			),
			check: func(t *testing.T, cond *qdrant.Condition) {
				assert.NotNil(t, cond)
				nestedFilter := cond.GetFilter()
				assert.NotNil(t, nestedFilter)
				assert.Len(t, nestedFilter.Must, 2)
			},
		},
		{
			name: "nested OR",
			expr: filter.Or(
				filter.EQ("role", "admin"),
				filter.EQ("role", "owner"),
			),
			check: func(t *testing.T, cond *qdrant.Condition) {
				assert.NotNil(t, cond)
				nestedFilter := cond.GetFilter()
				assert.NotNil(t, nestedFilter)
				assert.Len(t, nestedFilter.Should, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewConverter()
			cond, err := conv.buildNestedCondition(tt.expr)

			require.NoError(t, err)
			require.NotNil(t, cond)
			tt.check(t, cond)
		})
	}
}

func TestConverter_literalToValue(t *testing.T) {
	tests := []struct {
		name     string
		literal  *ast.Literal
		expected any
		wantErr  bool
	}{
		{
			name:     "string",
			literal:  filter.NewLiteral("test"),
			expected: "test",
		},
		{
			name:     "int",
			literal:  filter.NewLiteral(123),
			expected: 123.0,
		},
		{
			name:     "float",
			literal:  filter.NewLiteral(3.14),
			expected: 3.14,
		},
		{
			name:     "true",
			literal:  filter.NewLiteral(true),
			expected: true,
		},
		{
			name:     "false",
			literal:  filter.NewLiteral(false),
			expected: false,
		},
		{
			name: "invalid token kind",
			literal: &ast.Literal{
				Token: token.Token{Kind: token.ERROR},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewConverter()
			value, err := conv.literalToValue(tt.literal)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, value)
			}
		})
	}
}

// Test: Edge Cases

func TestConverter_StatePreservation(t *testing.T) {
	conv := NewConverter()

	// Set initial state
	conv.currentFieldKey = "initial_key"
	conv.currentFieldValue = "initial_value"

	// Extract a field key (should preserve original state)
	_, err := conv.extractFieldKey(filter.NewIdent("temp_key"))
	require.NoError(t, err)

	// Verify original state is restored
	assert.Equal(t, "initial_key", conv.currentFieldKey)
	assert.Equal(t, "initial_value", conv.currentFieldValue)
}

func TestConverter_MultipleConditions(t *testing.T) {
	// Test building multiple conditions sequentially
	expr := filter.And(
		filter.And(
			filter.GT("age", 18),
			filter.EQ("status", "active"),
		),
		filter.And(
			filter.LT("score", 100),
			filter.NE("role", "guest"),
		),
	)

	result, err := ToFilter(expr)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Must)
}

func TestConverter_AllOperatorTypes(t *testing.T) {
	// Test expression using all operator types
	expr := filter.And(
		filter.And(
			filter.And(
				filter.EQ("field1", "value1"),
				filter.NE("field2", "value2"),
			),
			filter.And(
				filter.LT("field3", 10),
				filter.LE("field4", 20),
			),
		),
		filter.And(
			filter.And(
				filter.GT("field5", 30),
				filter.GE("field6", 40),
			),
			filter.And(
				filter.In("field7", []string{"a", "b"}),
				filter.Like("field8", "%pattern%"),
			),
		),
	)

	result, err := ToFilter(expr)

	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestConverter_DeepIndexNesting(t *testing.T) {
	// Test deeply nested index expressions
	expr := filter.EQ(
		filter.Index(
			filter.Index(
				filter.Index(
					filter.Index("root", "level1"),
					"level2",
				),
				"level3",
			),
			"level4",
		),
		"value",
	)

	result, err := ToFilter(expr)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Must, 1)
}
