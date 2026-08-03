package elasticsearch_test

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/elasticsearch"
	"github.com/Tangerg/lynx/vectorstores/storetest"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := elasticsearch.NewVisitor("metadata")
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any { return visitor.Result() }}
	})
}
