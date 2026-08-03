package tidb_test

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/storetest"
	"github.com/Tangerg/lynx/vectorstores/tidb"
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
