package hitl

import (
	"context"
	"hash/fnv"
	"strconv"

	"github.com/Tangerg/lynx/agent/hitl"
)

// Interrupt delegates to [agent/hitl.Interrupt] so tool packages can park on
// runtime interrupts without duplicating key/hash logic.
func Interrupt[R any](ctx context.Context, key string, value any) (R, bool, error) {
	return hitl.Interrupt[R](ctx, key, value)
}

// Key derives a stable key from kind/tool/arguments using the same digest shape
// as runtime HITL keys.
func Key(kind, toolName, arguments string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(kind))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(toolName))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(arguments))
	return kind + "." + strconv.FormatUint(h.Sum64(), 16)
}
