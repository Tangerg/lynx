package filter_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		`category == 'tech'`,
		`year >= 2020 and published == true`,
		`tags in ('go', 'ai')`,
		`profile['author'] like 'A%'`,
		`not (deleted is null)`,
		``,
		`field ==`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		expr, err := filter.Parse(input)
		if err != nil {
			return
		}
		if expr == nil {
			t.Fatal("Parse succeeded with a nil expression")
		}
		if err := filter.Validate(expr); err != nil {
			t.Fatalf("Parse returned an invalid expression: %v", err)
		}
		if !expr.Equal(expr) {
			t.Fatal("parsed expression is not equal to itself")
		}
	})
}
