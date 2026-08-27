package neo4j_test

import (
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/storetest"
	"github.com/Tangerg/scope/vectorstores/neo4j"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := neo4j.NewVisitor("n", "metadata")
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any {
			query, args := visitor.Result()
			return struct {
				query string
				args  any
			}{query: query, args: args}
		}}
	})
}
