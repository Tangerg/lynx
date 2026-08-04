// Package maintenance implements Run-boundary LLM workers:
// history compaction, long-term fact extraction, and session titling.
//
// These workers operate OUTSIDE the normal conversation flow — they call the
// chat client directly (via askDirect), bypassing the chat history /
// tool / guardrail middleware so their own LLM calls never pollute the
// conversation history. They share the transcript-rendering and
// direct-call helpers in llm.go; each is otherwise an independent,
// single-responsibility worker (Compactor / Extractor / Titler) in its
// own file, constructible and testable without the agentexec. Suite is the
// one explicit composition point for the workers that run after a clean Run.
//
// Bootstrap owns construction. Suite owns the maintenance lifecycle policy;
// the execution controller supplies finished-Run facts and observes its result.
package maintenance
