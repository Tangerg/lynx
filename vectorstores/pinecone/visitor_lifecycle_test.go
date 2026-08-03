package pinecone_test

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/pinecone"
	"github.com/Tangerg/lynx/vectorstores/storetest"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := pinecone.NewVisitor()
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any {
			result, err := visitor.Filter()
			message := ""
			if err != nil {
				message = err.Error()
			}
			return struct {
				result any
				err    string
			}{result: result, err: message}
		}}
	})
}
