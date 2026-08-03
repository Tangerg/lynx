package redis_test

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/redis"
	"github.com/Tangerg/lynx/vectorstores/storetest"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := redis.NewVisitor(map[string]redis.MetadataFieldType{
			"a": redis.FieldNumeric,
			"b": redis.FieldNumeric,
		})
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any { return visitor.Result() }}
	})
}
