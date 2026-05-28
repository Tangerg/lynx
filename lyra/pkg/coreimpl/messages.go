package coreimpl

import (
	"context"

	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
)

// ListMessages — needs a paginated message store backing; the
// current internal/storage.MessageStore is a per-session chat memory
// adapter, not the message-table the wire surface expects. Stub
// until the schema work lands.
func (i *Impl) ListMessages(_ context.Context, _ coreapi.ListMessagesIn) (*coreapi.Page[coreapi.Message], error) {
	return nil, notImpl("messages.list")
}

// EditMessage — depends on checkpoints / fork semantics that aren't
// wired through the engine yet.
func (i *Impl) EditMessage(_ context.Context, _ coreapi.EditMessageIn) (*coreapi.EditMessageOut, error) {
	return nil, notImpl("messages.edit")
}
