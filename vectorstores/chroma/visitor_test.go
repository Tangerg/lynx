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
			expr, err := filter.ParseAndAnalyze(src)
			if err != nil {
				return err
			}
			v := chroma.NewVisitor()
			v.Visit(expr)
			return v.Error()
		},
		storetest.Options{
			Skip: []string{"not", "nested_logical", "like"},
		},
	)
}
