package tidb_test

import (
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/storetest"
	"github.com/Tangerg/scope/vectorstores/tidb"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := tidb.NewVisitor("metadata")
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any {
			query, args := visitor.Result()
			return struct {
				query string
				args  any
			}{query: query, args: args}
		}}
	})
}
