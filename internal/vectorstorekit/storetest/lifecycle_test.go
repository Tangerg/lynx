package storetest_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/internal/vectorstorekit/storetest"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		var result filter.Predicate
		return storetest.Compiler{
			Visit: func(predicate filter.Predicate) error {
				result = nil
				if predicate == nil {
					return errors.New("nil predicate")
				}
				result = predicate
				return nil
			},
			Snapshot: func() any { return result },
		}
	})
}
