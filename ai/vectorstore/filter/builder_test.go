package filter

import (
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/visitors"
)

func TestNewBuilder(t *testing.T) {
	b := NewBuilder()
	expr, err := b.
		EQ("user_type", "individual").
		Like("name", "%tom").
		GT("age", 18).
		LT("age", 56).
		In("status", []string{"pending", "active"}).
		Not(func(builder *ExprBuilder) {
			builder.In("status", []string{"suspended"})
		}).
		EQ(Index("color", 1), "red").
		NE(Index(Index("a", 1), "b"), "red").
		EQ(Index("addr", "country"), "UK").
		In(Index("info", "email"), []string{"tom@gmail.com"}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(expr)
	t.Log(visitor.SQL())
}
