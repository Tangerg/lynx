package filter_test

import (
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/visitors"
)

func TestNewBuilder(t *testing.T) {
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
		t.Fatal(err)
	}
	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(expr)
	t.Log(visitor.SQL())
}
