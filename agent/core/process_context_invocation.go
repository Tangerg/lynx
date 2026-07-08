package core

import "context"

// RecordUsage attributes an LLM call's cost / tokens to the running process.
// No-op when no Process is wired.
func (pc *ProcessContext) RecordUsage(ctx context.Context, cost float64, tokens int) {
	if pc.Process == nil {
		return
	}
	pc.Process.RecordUsage(contextOrBackground(ctx), cost, tokens)
}

// RecordLLMInvocation forwards a per-call LLM invocation record to the running
// process. No-op when no Process is wired.
func (pc *ProcessContext) RecordLLMInvocation(ctx context.Context, inv LLMInvocation) {
	if pc.Process == nil {
		return
	}
	pc.Process.RecordLLMInvocation(contextOrBackground(ctx), inv)
}

// RecordEmbeddingInvocation forwards a per-call embedding invocation record to
// the running process. No-op when no Process is wired.
func (pc *ProcessContext) RecordEmbeddingInvocation(ctx context.Context, inv EmbeddingInvocation) {
	if pc.Process == nil {
		return
	}
	pc.Process.RecordEmbeddingInvocation(contextOrBackground(ctx), inv)
}
