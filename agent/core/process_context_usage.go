package core

import "context"

// RecordUsage attributes an LLM call's cost / tokens to the running process.
// No-op when no UsageRecorder is wired.
func (pc *ProcessContext) RecordUsage(ctx context.Context, cost float64, tokens int) {
	if pc == nil || pc.usage == nil {
		return
	}
	pc.usage.RecordUsage(contextOrBackground(ctx), cost, tokens)
}

// RecordModelCall forwards a model-call record to the running
// process. No-op when no UsageRecorder is wired.
func (pc *ProcessContext) RecordModelCall(ctx context.Context, call ModelCall) {
	if pc == nil || pc.usage == nil {
		return
	}
	pc.usage.RecordModelCall(contextOrBackground(ctx), call)
}

// RecordEmbeddingCall forwards an embedding-call record to
// the running process. No-op when no UsageRecorder is wired.
func (pc *ProcessContext) RecordEmbeddingCall(ctx context.Context, call EmbeddingCall) {
	if pc == nil || pc.usage == nil {
		return
	}
	pc.usage.RecordEmbeddingCall(contextOrBackground(ctx), call)
}
