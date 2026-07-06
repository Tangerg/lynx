// Package maintenance implements turn-boundary LLM workers for kernel ports:
// history compaction, long-term fact extraction, and session titling.
//
// These workers operate OUTSIDE the normal conversation flow — they call the
// chat client directly (via askDirect), bypassing the chat history /
// tool / guardrail middleware so their own LLM calls never pollute the
// conversation history. They share the transcript-rendering and
// direct-call helpers in llm.go; each is otherwise an independent,
// single-responsibility worker (Compactor / Extractor / Titler) in its
// own file, constructible and testable without the kernel.
//
// The kernel owns construction (any worker may be nil when its feature is
// disabled by config) and orchestration — it decides when to call which; this
// adapter decides only how each operation is performed against chat and memory
// dependencies.
package maintenance
