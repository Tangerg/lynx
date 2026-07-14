package pinecone_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/storetest"
	"github.com/Tangerg/lynx/vectorstores/pinecone"
)

// TestVisitor_Conformance runs the shared visitor suite. Pinecone
// metadata filters don't support LIKE — that conformance case is
// opted out via [storetest.Options.Skip].
func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t,
		func(src string) error {
			expr, err := filter.Parse(src)
			if err != nil {
				return err
			}
			v := pinecone.NewVisitor()
			return v.Visit(expr)
		},
		storetest.Options{
			// Pinecone metadata filters have no LIKE operator.
			Skip: []string{"like"},
		},
	)
}
