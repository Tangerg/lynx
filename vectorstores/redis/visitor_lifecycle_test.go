package redis

import (
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/storetest"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := newVisitor(map[string]MetadataFieldType{
			"a": FieldNumeric,
			"b": FieldNumeric,
		})
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any { return visitor.snapshot() }}
	})
}
