package azurecosmos_test

import (
	"testing"

	"github.com/Tangerg/lynx/internal/vectorstorekit/storetest"
	"github.com/Tangerg/lynx/vectorstores/azurecosmos"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := azurecosmos.NewVisitor("c", "metadata")
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any {
			query, args := visitor.Result()
			return struct {
				query string
				args  any
			}{query: query, args: args}
		}}
	})
}
