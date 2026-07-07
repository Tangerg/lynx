package interrupts

import (
	"context"
	"errors"
	"hash/fnv"
	"strconv"
)

// Interruption is the narrow contract for resumable control-flow in runtime tools.
type Interruption func(ctx context.Context, key string, value any) (Resolution, bool, error)

// NoInterruption is the explicit failure path when the runtime doesn't wire any
// resume-aware interrupt implementation.
func NoInterruption(context.Context, string, any) (Resolution, bool, error) {
	return Resolution{}, false, errors.New("interrupt contract is unavailable")
}

// InterruptKey is the stable identity used by HITL questions and approvals.
//
// It is namespace-sensitive so mixed kinds do not collide (for example
// "approval|write|{...}" vs. "question|write|{...}"). The stable hash keeps
// serialized keys compact for transport and durable stores.
func InterruptKey(kind, toolName, arguments string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(kind))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(toolName))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(arguments))
	return kind + "." + strconv.FormatUint(h.Sum64(), 16)
}
