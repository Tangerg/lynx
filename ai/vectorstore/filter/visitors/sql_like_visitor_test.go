package visitors

import (
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	ast2 "github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
)

func TestNewSQLLikeVisitor(t *testing.T) {
	ast := filter.Or(
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

	visitor := NewSQLLikeVisitor()

	ast2.Walk(visitor, ast)

	sql := visitor.SQL()
	t.Log(sql)
}
