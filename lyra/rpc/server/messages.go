package server

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// ListMessages — needs a paginated message store backing; the
// current internal/storage.MessageStore is a per-session chat memory
// adapter, not the message-table the wire surface expects. Stub
// until the schema work lands.
func (i *Server) ListMessages(_ context.Context, _ protocol.ListMessagesRequest) (*protocol.Page[protocol.Message], error) {
	return nil, notImpl("messages.list")
}

// EditMessage — depends on checkpoints / fork semantics that aren't
// wired through the engine yet.
func (i *Server) EditMessage(_ context.Context, _ protocol.EditMessageRequest) (*protocol.EditMessageResponse, error) {
	return nil, notImpl("messages.edit")
}
