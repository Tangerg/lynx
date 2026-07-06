// Package history provides the chat middleware that auto-loads and auto-saves
// conversation history on every chat turn keyed by a conversation id.
//
// Example:
//
//	store := conversationhistory.NewInMemoryStore()
//	mw, _, err := history.NewMiddleware(store)
//	resp, err := client.Chat().
//	    WithParams(map[string]any{conversation.IDKey: "session-123"}).
//	    WithMiddlewares(mw).
//	    WithUserPrompt("hi").
//	    Call().Response(ctx)
package history
