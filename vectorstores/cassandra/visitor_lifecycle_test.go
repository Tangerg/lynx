package cassandra_test

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/cassandra"
	"github.com/Tangerg/lynx/vectorstores/storetest"
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
