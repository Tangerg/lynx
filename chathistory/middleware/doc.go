// Package middleware binds a context-scoped conversation ID to chat model
// calls through a chathistory Reader and Writer.
//
// Live system messages remain request-scoped and are never persisted. Stored
// non-system messages are replayed before the fresh non-system messages. A
// successful final assistant response is persisted with the fresh messages;
// tool-call responses are deferred so a following tool-loop request can store
// the complete assistant-call/tool-result exchange atomically.
package middleware
