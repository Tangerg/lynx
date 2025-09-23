package visitors

import (
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter"
)

func TestNewAnalyzer(t *testing.T) {
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
	analyzer := NewAnalyzer()
	analyzer.Visit(expr)
	t.Log(analyzer.Error())
}
