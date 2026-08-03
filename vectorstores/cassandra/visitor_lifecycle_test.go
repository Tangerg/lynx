package cassandra_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/storetest"
	"github.com/Tangerg/lynx/vectorstores/cassandra"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := cassandra.NewVisitor()
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any {
			query, args := visitor.Result()
			return struct {
				query string
				args  any
			}{query: query, args: args}
		}}
	})
}
