package core

import "context"

type contextUsageRecorder interface {
	RecordUsageContext(context.Context, float64, int)
}

type contextLLMInvocationRecorder interface {
	RecordLLMInvocationContext(context.Context, LLMInvocation)
}

type contextEmbeddingInvocationRecorder interface {
	RecordEmbeddingInvocationContext(context.Context, EmbeddingInvocation)
}

// RecordUsage attributes an LLM call's cost / tokens to the running process.
// When called inside ExecuteSafely, the emitted invocation event inherits the
// current action context. No-op when no Process is wired.
func (pc *ProcessContext) RecordUsage(cost float64, tokens int) {
	pc.RecordUsageContext(pc.eventContext, cost, tokens)
}

// RecordUsageContext is the context-aware companion to [ProcessContext.RecordUsage].
func (pc *ProcessContext) RecordUsageContext(ctx context.Context, cost float64, tokens int) {
	if pc.Process == nil {
		return
	}
	if recorder, ok := pc.Process.(contextUsageRecorder); ok {
		recorder.RecordUsageContext(ctx, cost, tokens)
		return
	}
	pc.Process.RecordUsage(cost, tokens)
}

// RecordLLMInvocation forwards a per-call LLM invocation record to the running
// process. When called inside ExecuteSafely, the emitted invocation event
// inherits the current action context. No-op when no Process is wired.
func (pc *ProcessContext) RecordLLMInvocation(inv LLMInvocation) {
	pc.RecordLLMInvocationContext(pc.eventContext, inv)
}

// RecordLLMInvocationContext is the context-aware companion to
// [ProcessContext.RecordLLMInvocation].
func (pc *ProcessContext) RecordLLMInvocationContext(ctx context.Context, inv LLMInvocation) {
	if pc.Process == nil {
		return
	}
	if recorder, ok := pc.Process.(contextLLMInvocationRecorder); ok {
		recorder.RecordLLMInvocationContext(ctx, inv)
		return
	}
	pc.Process.RecordLLMInvocation(inv)
}

// RecordEmbeddingInvocation forwards a per-call embedding invocation record to
// the running process. When called inside ExecuteSafely, the emitted invocation
// event inherits the current action context. No-op when no Process is wired.
func (pc *ProcessContext) RecordEmbeddingInvocation(inv EmbeddingInvocation) {
	pc.RecordEmbeddingInvocationContext(pc.eventContext, inv)
}

// RecordEmbeddingInvocationContext is the context-aware companion to
// [ProcessContext.RecordEmbeddingInvocation].
func (pc *ProcessContext) RecordEmbeddingInvocationContext(ctx context.Context, inv EmbeddingInvocation) {
	if pc.Process == nil {
		return
	}
	if recorder, ok := pc.Process.(contextEmbeddingInvocationRecorder); ok {
		recorder.RecordEmbeddingInvocationContext(ctx, inv)
		return
	}
	pc.Process.RecordEmbeddingInvocation(inv)
}
