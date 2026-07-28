package agentexec

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/interaction"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/core/chat"
)

var usageLedgerKey = core.MustDependencyKey[*usageLedger]("lyra.usage")

// usageLedger is the application-owned model/accounting projection for one
// complete process tree. Agent tracks only execution counters; this ledger
// retains the model breakdown required by Runtime output and persistence.
type usageLedger struct {
	mu            sync.RWMutex
	models        map[string]accounting.ModelUsage
	total         accounting.ModelUsage
	projectionErr error
}

func newUsageLedger(snapshot accounting.Snapshot) (*usageLedger, error) {
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	ledger := &usageLedger{models: make(map[string]accounting.ModelUsage, len(snapshot.Models))}
	for _, model := range snapshot.Models {
		nextTotal := ledger.total
		if err := addModelUsage(&nextTotal, model); err != nil {
			return nil, fmt.Errorf("agentexec: restore usage aggregate: %w", err)
		}
		if err := validateFrameworkUsageCapacity(nextTotal); err != nil {
			return nil, fmt.Errorf("agentexec: restore usage aggregate: %w", err)
		}
		ledger.total = nextTotal
		ledger.models[model.Model] = model
	}
	return ledger, nil
}

func emptyUsageLedger() *usageLedger {
	ledger, err := newUsageLedger(accounting.Snapshot{})
	if err != nil {
		panic(err)
	}
	return ledger
}

func usageLedgerFrom(dependencies *core.Dependencies) (*usageLedger, error) {
	ledger, err := core.LookupDependency(dependencies, usageLedgerKey)
	if err != nil {
		return nil, fmt.Errorf("agentexec: resolve usage ledger: %w", err)
	}
	return ledger, nil
}

func (l *usageLedger) record(response *chat.Response, cost float64) error {
	if l == nil {
		return errors.New("agentexec: usage ledger is nil")
	}
	if response == nil {
		return l.reject(errors.New("agentexec: record usage from nil model response"))
	}
	model := cmp.Or(response.Model, "unknown")
	delta := accounting.ModelUsage{
		Model:      model,
		TokenUsage: tokenUsageOf(response.Usage),
		CostUSD:    cost,
		Calls:      1,
	}
	if err := delta.Validate(); err != nil {
		return l.reject(fmt.Errorf("agentexec: record model usage: %w", err))
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.projectionErr != nil {
		return l.projectionErr
	}
	current, exists := l.models[model]
	nextModel := delta
	if exists {
		nextModel = current
		if err := addModelUsage(&nextModel, delta); err != nil {
			return l.rejectLocked(fmt.Errorf("agentexec: record model usage: %w", err))
		}
	}
	nextTotal := l.total
	if err := addModelUsage(&nextTotal, delta); err != nil {
		return l.rejectLocked(fmt.Errorf("agentexec: record total usage: %w", err))
	}
	if err := validateFrameworkUsageCapacity(nextTotal); err != nil {
		return l.rejectLocked(fmt.Errorf("agentexec: record total usage: %w", err))
	}
	l.models[model] = nextModel
	l.total = nextTotal
	return nil
}

func (l *usageLedger) reject(err error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rejectLocked(err)
}

func (l *usageLedger) rejectLocked(err error) error {
	if l.projectionErr == nil {
		l.projectionErr = err
	}
	return l.projectionErr
}

func addModelUsage(total *accounting.ModelUsage, delta accounting.ModelUsage) error {
	if total == nil {
		return errors.New("model usage total is nil")
	}
	prompt, ok := addInt64(total.PromptTokens, delta.PromptTokens)
	if !ok {
		return errors.New("prompt-token total exceeds int64 capacity")
	}
	completion, ok := addInt64(total.CompletionTokens, delta.CompletionTokens)
	if !ok {
		return errors.New("completion-token total exceeds int64 capacity")
	}
	reasoning, ok := addInt64(total.ReasoningTokens, delta.ReasoningTokens)
	if !ok {
		return errors.New("reasoning-token total exceeds int64 capacity")
	}
	cacheRead, ok := addInt64(total.CacheReadTokens, delta.CacheReadTokens)
	if !ok {
		return errors.New("cache-read-token total exceeds int64 capacity")
	}
	cacheWrite, ok := addInt64(total.CacheWriteTokens, delta.CacheWriteTokens)
	if !ok {
		return errors.New("cache-write-token total exceeds int64 capacity")
	}
	if delta.CostUSD > math.MaxFloat64-total.CostUSD {
		return errors.New("cost total exceeds float64 capacity")
	}
	if delta.Calls > math.MaxInt-total.Calls {
		return errors.New("model-call total exceeds int capacity")
	}
	total.PromptTokens = prompt
	total.CompletionTokens = completion
	total.ReasoningTokens = reasoning
	total.CacheReadTokens = cacheRead
	total.CacheWriteTokens = cacheWrite
	total.CostUSD += delta.CostUSD
	total.Calls += delta.Calls
	return nil
}

func addInt64(left, right int64) (int64, bool) {
	if right < 0 || left > math.MaxInt64-right {
		return 0, false
	}
	return left + right, true
}

func validateFrameworkUsageCapacity(usage accounting.ModelUsage) error {
	if _, ok := addInt64(usage.PromptTokens, usage.CompletionTokens); !ok {
		return errors.New("token total exceeds int64 capacity")
	}
	return nil
}

func (l *usageLedger) snapshot() (accounting.Snapshot, error) {
	snapshot, _, err := l.state()
	return snapshot, err
}

func (l *usageLedger) output(reply string, stopReason agent.InteractionStopReason) (TurnOutput, error) {
	snapshot, total, err := l.state()
	if err != nil {
		return TurnOutput{}, err
	}
	return TurnOutput{
		Reply:        reply,
		Usage:        total.TokenUsage,
		UsageByModel: slices.Clone(snapshot.Models),
		CostUSD:      total.CostUSD,
		StopReason:   stopReason,
	}, nil
}

func (l *usageLedger) totals() (accounting.TokenUsage, float64, error) {
	_, total, err := l.state()
	return total.TokenUsage, total.CostUSD, err
}

func (l *usageLedger) state() (accounting.Snapshot, accounting.ModelUsage, error) {
	if l == nil {
		return accounting.Snapshot{}, accounting.ModelUsage{}, errors.New("agentexec: usage ledger is nil")
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	models := make([]string, 0, len(l.models))
	for model := range l.models {
		models = append(models, model)
	}
	slices.Sort(models)
	snapshot := accounting.Snapshot{Models: make([]accounting.ModelUsage, 0, len(models))}
	for _, model := range models {
		snapshot.Models = append(snapshot.Models, l.models[model])
	}
	return snapshot, l.total, l.projectionErr
}

type interactionProjection struct {
	engine      *Engine
	provider    string
	usage       *usageLedger
	observation *toolObservation
}

var (
	_ core.InteractionCostProjector = (*interactionProjection)(nil)
	_ agentruntime.EventListener    = (*interactionProjection)(nil)
)

func (*interactionProjection) Name() string { return "lyra:interaction-projection" }

func (p *interactionProjection) ProjectInteractionCost(
	_ context.Context,
	_ core.ProcessView,
	response *chat.Response,
) (float64, error) {
	if p == nil || p.engine == nil || p.engine.pricing == nil || response == nil {
		return 0, nil
	}
	resolvedProvider := cmp.Or(p.provider, p.engine.defaultProvider)
	model := cmp.Or(response.Model, "unknown")
	return p.engine.pricing(resolvedProvider, model, &response.Usage), nil
}

func (p *interactionProjection) OnEvent(_ context.Context, published event.Event) {
	if p == nil {
		return
	}
	interactionBoundary, ok := published.(event.InteractionBoundary)
	if !ok {
		return
	}
	process := p.processRef(interactionBoundary.ProcessID())
	boundary := interactionBoundary.Boundary
	if p.observation != nil {
		switch boundary.Kind {
		case interaction.EventToolCall:
			if boundary.ToolCall != nil {
				p.observation.beginRef(process, boundary.Round, *boundary.ToolCall)
			}
		case interaction.EventToolResult:
			if boundary.ToolResult != nil {
				p.observation.resultRef(process, boundary.Round, *boundary.ToolResult)
			}
		}
	}
	if boundary.Kind != interaction.EventModelResponse {
		return
	}
	if err := p.usage.record(boundary.Response, boundary.Cost); err != nil {
		return
	}
	if p.observation == nil || p.observation.target == nil ||
		(boundary.Response.Usage.TotalTokens() == 0 && boundary.Response.Model == "") {
		return
	}
	cumulative, cumulativeCost, err := p.usage.totals()
	if err != nil {
		return
	}
	p.observation.target.OnUsage(
		process,
		cumulative,
		cumulativeCost,
		boundary.Response.Usage.InputTokens,
	)
}

func (p *interactionProjection) processRef(processID string) ProcessRef {
	if p.engine != nil && p.engine.runtime != nil {
		if process, ok := p.engine.runtime.Process(processID); ok {
			return processRef(process)
		}
	}
	return ProcessRef{ID: processID}
}

func tokenUsageOf(usage chat.Usage) accounting.TokenUsage {
	result := accounting.TokenUsage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
	}
	if usage.ReasoningTokens != nil {
		result.ReasoningTokens = *usage.ReasoningTokens
	}
	if usage.CacheReadInputTokens != nil {
		result.CacheReadTokens = *usage.CacheReadInputTokens
	}
	if usage.CacheWriteInputTokens != nil {
		result.CacheWriteTokens = *usage.CacheWriteInputTokens
	}
	return result
}

func validateCheckpointUsage(tree core.ProcessSnapshotTree, snapshot accounting.Snapshot) error {
	if err := tree.Validate(); err != nil {
		return err
	}
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("%w: application usage: %w", core.ErrInvalidSnapshot, err)
	}

	framework, err := tree.Usage()
	if err != nil {
		return err
	}

	var application accounting.ModelUsage
	for _, model := range snapshot.Models {
		if err := addModelUsage(&application, model); err != nil {
			return fmt.Errorf("%w: application usage aggregate: %w", core.ErrInvalidSnapshot, err)
		}
	}
	tokens, ok := addInt64(application.PromptTokens, application.CompletionTokens)
	if !ok {
		return fmt.Errorf("%w: application token aggregate overflows", core.ErrInvalidSnapshot)
	}
	if framework.Tokens != tokens || framework.ModelCalls != application.Calls ||
		!sameCost(framework.Cost, application.CostUSD) {
		return fmt.Errorf(
			"%w: framework usage %+v does not match application usage {cost:%g tokens:%d model_calls:%d}",
			core.ErrInvalidSnapshot,
			framework,
			application.CostUSD,
			tokens,
			application.Calls,
		)
	}
	return nil
}

func sameCost(left, right float64) bool {
	scale := max(1, math.Abs(left), math.Abs(right))
	return math.Abs(left-right) <= 1e-12*scale
}
