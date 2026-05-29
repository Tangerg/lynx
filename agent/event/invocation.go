package event

import "github.com/Tangerg/lynx/agent/core"

// LLMInvocationRecorded fires when an LLM call is attributed to a process
// via [Process.RecordLLMInvocation] or [Process.RecordUsage]. It carries the
// full invocation so listeners can do per-call cost auditing, billing
// reconciliation, or live token dashboards without polling
// [Process.LLMInvocations]. Mirrors embabel's LlmInvocationEvent.
type LLMInvocationRecorded struct {
	BaseEvent
	Invocation core.LLMInvocation `json:"-"`
}

func (LLMInvocationRecorded) EventName() string { return "llm_invocation" }

// EmbeddingInvocationRecorded mirrors [LLMInvocationRecorded] for the
// embeddings path.
type EmbeddingInvocationRecorded struct {
	BaseEvent
	Invocation core.EmbeddingInvocation `json:"-"`
}

func (EmbeddingInvocationRecorded) EventName() string { return "embedding_invocation" }
