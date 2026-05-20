package elasticsearch_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/elasticsearch"
	"github.com/Tangerg/lynx/vectorstores/internal/storetest"
)

func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t, func(src string) error {
		expr, err := filter.ParseAndAnalyze(src)
		if err != nil {
			return err
		}
		v := elasticsearch.NewVisitor("metadata")
		v.Visit(expr)
		return v.Error()
	})
}
