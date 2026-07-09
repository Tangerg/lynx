package turn

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel/accounting"
)

// turnTracer emits the business-level span for one turn — the
// boundary every lower-layer span (agent runtime tick / action, core
// gen_ai LLM call, MCP tool) nests under once the turn's lifetime ctx
// carries the entry trace (see newTurnState's WithoutCancel derivation).
// Name follows the `lynx/lyra/...` instrumentation-scope convention;
// no-op until a TracerProvider is installed.
const turnTracerName = "lynx/lyra/turn"

var turnTracer = otel.Tracer(turnTracerName)

// Span / metric attribute keys. OTel GenAI semconv where one exists
// (a turn is the spec's `invoke_agent` operation over a keyed
// conversation), bare `run.*` domain for the lyra-runtime concepts the
// semconv has no key for. No brand prefixes — see doc/OBSERVABILITY.md.
const (
	attrGenAIOperationName  = "gen_ai.operation.name"
	attrGenAIConversationID = "gen_ai.conversation.id"
	attrGenAIRequestModel   = "gen_ai.request.model"
	attrGenAIUsageInput     = "gen_ai.usage.input_tokens"
	attrGenAIUsageOutput    = "gen_ai.usage.output_tokens"
	attrRunID               = "run.id"
	attrRunOutcome          = "run.outcome"
	attrRunInterruptKind    = "run.interrupt.kind"

	opInvokeAgent = "invoke_agent"
)

// startTurnSpan opens the turn span as a child of ctx (the entry trace,
// carried in via WithoutCancel) and returns the span-bearing ctx the
// engine call runs under so its LLM / tool / agent spans attach as
// children — full-link tracing from the HTTP entry down to each model
// round. Span kind is Internal (the turn orchestrates; the remote LLM
// call inside it is the Client span). Name uses the canonical gen_ai
// `<operation> <model>` shape.
func startTurnSpan(ctx context.Context, sessionID, runID, model string) (context.Context, trace.Span) {
	name := opInvokeAgent
	if model != "" {
		name = opInvokeAgent + " " + model
	}
	attrs := []attribute.KeyValue{
		attribute.String(attrGenAIOperationName, opInvokeAgent),
		attribute.String(attrRunID, runID),
	}
	if sessionID != "" {
		attrs = append(attrs, attribute.String(attrGenAIConversationID, sessionID))
	}
	if model != "" {
		attrs = append(attrs, attribute.String(attrGenAIRequestModel, model))
	}
	return turnTracer.Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
}

// finishTurnSpan records the terminal outcome on the turn span — the
// run outcome, the rolled-up token usage (clean / budget-stopped turns
// only), and an Error status when the turn aborted. It does NOT end the
// span; endTurn owns the single End() so the span closes exactly once
// regardless of which teardown path fired. No-op on a nil span.
func finishTurnSpan(span trace.Span, reason TurnEndReason, usage accounting.TokenUsage, withUsage bool, errMsg string) {
	if span == nil {
		return
	}
	span.SetAttributes(attribute.String(attrRunOutcome, reason.String()))
	if withUsage {
		span.SetAttributes(
			attribute.Int64(attrGenAIUsageInput, usage.PromptTokens),
			attribute.Int64(attrGenAIUsageOutput, usage.CompletionTokens),
		)
	}
	if errMsg != "" {
		span.SetStatus(codes.Error, errMsg)
	}
}
