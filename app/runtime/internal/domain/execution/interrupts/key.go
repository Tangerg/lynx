package interrupts

import (
	"hash/fnv"
	"strconv"
)

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
