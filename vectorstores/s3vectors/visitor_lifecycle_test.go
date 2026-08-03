package s3vectors_test

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/s3vectors"
	"github.com/Tangerg/lynx/vectorstores/storetest"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := s3vectors.NewVisitor()
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any { return visitor.Result() }}
	})
}
