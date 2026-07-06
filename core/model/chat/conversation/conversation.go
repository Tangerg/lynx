// Package conversation owns the request-scoped conversation identity shared by
// chat middleware and agent runtimes.
package conversation

import (
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
)

// IDKey is the [chat.Request.Params] key identifying the conversation a
// request belongs to.
//
// The producer that owns the session/process id stamps this key once; consumers
// that persist per-conversation state, such as history splicing or parked tool
// rounds, read it through [ID].
const IDKey = "lynx:ai:model:chat:conversation_id"

// ID returns the conversation id stamped under [IDKey], or "" when the request
// is not bound to a durable conversation.
func ID(req *chat.Request) (string, error) {
	raw, exists := req.Get(IDKey)
	if !exists {
		return "", nil
	}
	id, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("chat conversation: IDKey value must be a string, got %T", raw)
	}
	return id, nil
}

// Stamp stores id on req under [IDKey]. An empty id deliberately clears the
// durable conversation binding for callers that reuse a request value.
func Stamp(req *chat.Request, id string) {
	req.Set(IDKey, id)
}
