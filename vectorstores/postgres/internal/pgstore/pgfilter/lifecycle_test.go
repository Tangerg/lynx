package pgfilter_test

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/postgres/internal/pgstore/pgfilter"
	"github.com/Tangerg/lynx/vectorstores/storetest"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		compiler := pgfilter.NewCompiler("metadata")
		return storetest.Compiler{Visit: compiler.Visit, Snapshot: func() any {
			query, args := compiler.Result()
			return struct {
				query string
				args  any
			}{query: query, args: args}
		}}
	})
}
