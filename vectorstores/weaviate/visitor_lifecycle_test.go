package weaviate_test

import (
	"testing"

	"github.com/Tangerg/lynx/internal/vectorstorekit/storetest"
	"github.com/Tangerg/lynx/vectorstores/weaviate"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := weaviate.NewVisitor()
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any { return visitor.Result() }}
	})
}
