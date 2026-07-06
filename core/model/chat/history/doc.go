// Package history defines conversation-history storage primitives for chat
// applications.
//
// The package owns the durable message-history port and small reference
// implementations. It deliberately does not know how a chat request is driven;
// middleware that splices history into model calls lives under
// core/model/chat/middleware/history.
package history
