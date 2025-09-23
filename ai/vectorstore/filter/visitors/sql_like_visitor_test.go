package visitors_test

import (
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/visitors"
)

func TestNewSQLLikeVisitor(t *testing.T) {
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

	visitor := visitors.NewSQLLikeVisitor()

	visitor.Visit(expr)

	sql := visitor.SQL()
	t.Log(sql)
}
