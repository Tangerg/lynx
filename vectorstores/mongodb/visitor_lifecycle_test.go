package mongodb

import (
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/storetest"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := newVisitor("metadata")
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any { return visitor.snapshot() }}
	})
}
