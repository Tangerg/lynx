package chroma_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/chroma"
	"github.com/Tangerg/lynx/vectorstores/internal/storetest"
)

// TestVisitor_Conformance runs the shared visitor suite. Chroma
// metadata filters don't support a standalone logical NOT (Chroma's
// model uses != / NIN inversions instead) and don't support LIKE
// against metadata fields, so the corresponding success cases —
// including nested_logical which embeds a NOT — are opted out.
func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t,
		func(src string) error {
			expr, err := filter.Parse(src)
			if err != nil {
				return err
			}
			v := chroma.NewVisitor()
			return v.Visit(expr)
		},
		storetest.Options{
			Skip: []string{"not", "nested_logical", "like"},
		},
	)
}

func TestVisitor_RejectsLossyNumbers(t *testing.T) {
	tests := map[string]filter.Predicate{
		"integer outside int": filter.EQ("id", uint64(^uint(0))),
		"mixed list rounds integer": filter.In("id", []*filter.Literal{
			filter.NewLiteral(1<<24 + 1),
			filter.NewLiteral(1.5),
		}),
	}
	for name, predicate := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := chroma.ToFilter(predicate); err == nil {
				t.Fatal("Chroma silently accepted a lossy number")
			}
		})
	}
}
