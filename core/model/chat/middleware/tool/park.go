package tool

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

// parkCtxKey is the context key for the park conversation identifier.
type parkCtxKey struct{}

// WithParkKey returns a child context carrying the park conversation
// identifier. The engine sets it before streaming so the tool
// middleware reads it (via [parkKey]) to key ParkStore operations.
func WithParkKey(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, parkCtxKey{}, id)
}

// parkKey reads the park conversation identifier from ctx, or ""
// when the caller didn't set one via [WithParkKey].
func parkKey(ctx context.Context) string {
	id, _ := ctx.Value(parkCtxKey{}).(string)
	return id
}

type ParkReader interface {
	Read(ctx context.Context, conversationID string) (*ParkState, error)
}

type ParkWriter interface {
	Write(ctx context.Context, conversationID string, state *ParkState) error
}

type ParkClearer interface {
	Clear(ctx context.Context, conversationID string) error
}

// ParkStore is the union of [ParkReader], [ParkWriter], and
// [ParkClearer] — the complete park-state surface. Pass it to
// [Config.ParkStore]; nil means the middleware falls back to
// [buildInterruptResponse] (the conversation-based tail
// path that the engine intercepts).
type ParkStore interface {
	ParkReader
	ParkWriter
	ParkClearer
}
