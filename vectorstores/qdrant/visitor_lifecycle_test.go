package qdrant_test

import (
	"testing"

	"github.com/Tangerg/lynx/internal/vectorstorekit/storetest"
	"github.com/Tangerg/lynx/vectorstores/qdrant"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := qdrant.NewVisitor()
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any { return visitor.Filter() }}
	})
}
