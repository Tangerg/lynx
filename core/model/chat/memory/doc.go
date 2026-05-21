// Package memory provides conversation-history primitives for chat
// applications. It defines the [Store] interface, two reference
// implementations ([InMemoryStore] and the windowed [MessageWindowStore]),
// and a middleware that auto-loads / auto-saves messages on every chat
// turn keyed by a conversation id.
//
// Example:
//
//	store := memory.NewInMemoryStore()
//	mw, _, err := memory.NewMiddleware(store)
//	resp, err := client.Chat().
//	    WithParams(map[string]any{memory.ConversationIDKey: "session-123"}).
//	    WithMiddlewares(mw).
//	    WithText("hi").
//	    Call().Response(ctx)
package memory
