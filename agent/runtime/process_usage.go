package runtime

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// Usage returns the subtree-aggregated cost / token / action totals.
// Cost and tokens come from [core.ProcessContext.RecordUsage] calls (zero
// unless integration code wires them up). The action count is the
// recursive sum of every History across this process and its children.
// [core.BudgetPolicy]
// uses the result so a parent's budget governs its entire delegation
// tree.
func (p *Process) Usage() (cost float64, tokens int, actions int) {
	return p.budget.usage(p.state.historyLen())
}

type processUsage struct{ process *Process }

var _ core.UsageRecorder = processUsage{}

func (recorder processUsage) RecordUsage(ctx context.Context, cost float64, tokens int) {
	recorder.RecordModelCall(ctx, core.ModelCall{CostUSD: cost, PromptTokens: int64(tokens)})
}

func (recorder processUsage) RecordModelCall(ctx context.Context, call core.ModelCall) {
	if ctx == nil {
		ctx = context.Background()
	}
	if call.Timestamp.IsZero() {
		call.Timestamp = time.Now()
	}
	process := recorder.process
	process.budget.recordModelCall(call)
	process.publishEvent(ctx, event.ModelCallRecorded{
		Header: process.eventHeader(),
		Call:   call,
	})
}

func (recorder processUsage) RecordEmbeddingCall(ctx context.Context, call core.EmbeddingCall) {
	if ctx == nil {
		ctx = context.Background()
	}
	if call.Timestamp.IsZero() {
		call.Timestamp = time.Now()
	}
	process := recorder.process
	process.budget.recordEmbeddingCall(call)
	process.publishEvent(ctx, event.EmbeddingCallRecorded{
		Header: process.eventHeader(),
		Call:   call,
	})
}

// ModelCalls returns the subtree-aggregated model-call history in
// process order.
func (p *Process) ModelCalls() []core.ModelCall {
	return p.budget.modelCallHistory()
}

// EmbeddingCalls returns the subtree-aggregated embedding-call history.
func (p *Process) EmbeddingCalls() []core.EmbeddingCall {
	return p.budget.embeddingCallHistory()
}
