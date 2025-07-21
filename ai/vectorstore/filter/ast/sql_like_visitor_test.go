package ast

import (
	"testing"
)

func TestNewSQLLikeVisitor(t *testing.T) {
	ast := OR(
		AND(
			EQ("user_type", "individual"),
			OR(
				AND(
					GE("age", 18),
					LIKE("name", "%tom"),
				),
				EQ("verified", true),
			),
		),
		AND(
			NOT(EQ("status", "suspended")),
			IN("tier", []string{"gold", "platinum"}),
		),
	)

	visitor := NewSQLLikeVisitor()

	Walk(visitor, ast)

	sql := visitor.SQL()
	t.Log(sql)
}
