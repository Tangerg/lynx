package couchbase_test

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/couchbase"
	"github.com/Tangerg/lynx/vectorstores/storetest"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := couchbase.NewVisitor("metadata")
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any { return visitor.Result() }}
	})
}
