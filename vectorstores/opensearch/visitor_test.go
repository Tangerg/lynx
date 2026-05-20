package opensearch_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/storetest"
	"github.com/Tangerg/lynx/vectorstores/opensearch"
)

func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t, func(src string) error {
		expr, err := filter.ParseAndAnalyze(src)
		if err != nil {
			return err
		}
		v := opensearch.NewVisitor("metadata")
		v.Visit(expr)
		return v.Error()
	})
}
