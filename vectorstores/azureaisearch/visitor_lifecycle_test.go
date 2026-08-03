package azureaisearch_test

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/azureaisearch"
	"github.com/Tangerg/lynx/vectorstores/storetest"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := azureaisearch.NewVisitor()
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any { return visitor.Result() }}
	})
}
