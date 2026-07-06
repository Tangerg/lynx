package toolloop

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"
)

// ParkState is the resumable state of an interrupted tool round:
// the assistant message that triggered the tool call(s) and any
// results already produced before the interrupt. On resume the
// conversation tail (assistant + Done) is injected into the request
// so [parseResumePoint] detects it and continues at the pending call.
type ParkState struct {
	Assistant *chat.AssistantMessage `json:"assistant"`
	Done      []*chat.ToolReturn     `json:"done,omitempty"`
}

// ParkConsumer atomically loads AND removes the parked round for a
// conversation, returning (nil, nil) when nothing is parked. Read-and-remove
// is ONE operation by design: a separate clear that failed after a successful
// read would leave a stale round that the next turn — possibly a brand-new
// one on the same conversation — re-injects, wrongly resuming onto a dead
// tail. Atomic consumption makes resume idempotent.
type ParkConsumer interface {
	Consume(ctx context.Context, conversationID string) (*ParkState, error)
}

type ParkWriter interface {
	Write(ctx context.Context, conversationID string, state *ParkState) error
}

// ParkStore is the resumable-round persistence surface: atomically consume a
// parked round on resume ([ParkConsumer]), write one on interrupt
// ([ParkWriter]). Pass it to [Config.ParkStore]; nil means the middleware
// falls back to [buildInterruptResponse] (the conversation-based tail path
// that the engine intercepts).
type ParkStore interface {
	ParkConsumer
	ParkWriter
}
