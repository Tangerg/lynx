// Package maintenance holds the turn-boundary domain operations the
// engine triggers autonomously after a chat turn ends: history
// compaction and long-term fact extraction.
//
// Both work OUTSIDE the normal conversation flow — they call the
// chat client directly (via askDirect), bypassing the chat-memory /
// tool / guardrail middleware so their own LLM calls never pollute the
// conversation history. They share the transcript-rendering and
// direct-call helpers in llm.go; each is otherwise an independent,
// single-responsibility worker (Compactor / Extractor) in its
// own file, constructible and testable without the engine.
//
// The engine owns construction (any worker may be nil when its feature
// is disabled by config) and orchestration — it decides when to call
// which; this package decides only how each operation works.
package maintenance
