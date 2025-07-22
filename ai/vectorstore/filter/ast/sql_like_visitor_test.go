package ast

import (
	"testing"
)

func TestNewSQLLikeVisitor(t *testing.T) {
	ast := Or(
		And(
			EQ("user_type", "individual"),
			Or(
				And(
					GE("age", 18),
					Like("name", "%tom"),
				),
				EQ("verified", true),
			),
		),
		And(
			Not(EQ("status", "suspended")),
			In("tier", []string{"gold", "platinum"}),
		),
	)

	visitor := NewSQLLikeVisitor()

	Walk(visitor, ast)

	sql := visitor.SQL()
	t.Log(sql)
}
