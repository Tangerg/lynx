package vectara_test

import (
	"testing"

	"github.com/Tangerg/lynx/internal/vectorstorekit/storetest"
	"github.com/Tangerg/lynx/vectorstores/vectara"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := vectara.NewVisitor("metadata")
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any { return visitor.Result() }}
	})
}
