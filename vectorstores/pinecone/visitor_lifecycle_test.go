package pinecone_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/storetest"
	"github.com/Tangerg/lynx/vectorstores/pinecone"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := pinecone.NewVisitor()
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any { return visitor.Result() }}
	})
}
