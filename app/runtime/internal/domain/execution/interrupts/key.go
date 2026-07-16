package interrupts

import (
	"context"
	"errors"
	"hash/fnv"
	"strconv"
)

// Func is the narrow contract for resumable control flow in runtime tools.
type Func func(ctx context.Context, key string, value any) (Resolution, error)

// Unavailable is the explicit failure path when the runtime does not wire a
// resume-aware interrupt function.
func Unavailable(context.Context, string, any) (Resolution, error) {
	return Resolution{}, errors.New("interrupt contract is unavailable")
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
