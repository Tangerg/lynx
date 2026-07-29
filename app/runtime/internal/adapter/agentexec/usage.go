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
	treeModels    map[string]accounting.ModelUsage
	treeTotal     accounting.ModelUsage
	byProcess     map[string]*processUsage
	projectionErr error
}

// processUsage retains direct model usage plus immutable lineage for one Agent
// process. Subtree totals are derived at read boundaries so concurrent sibling
// calls have one accounting authority and no parent/child double counting.
type processUsage struct {
	ref           ProcessRef
	directModels  map[string]accounting.ModelUsage
	directTotal   accounting.ModelUsage
	stopReason    agent.InteractionStopReason
	hasStopReason bool
}

type subtreeUsage struct {
	snapshot   accounting.Snapshot
	total      accounting.ModelUsage
	stopReason agent.InteractionStopReason
}

func newUsageLedger(snapshot accounting.Snapshot) (*usageLedger, error) {
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	ledger := &usageLedger{
		treeModels: make(map[string]accounting.ModelUsage, len(snapshot.Models)),
		byProcess:  make(map[string]*processUsage),
	}
	for _, model := range snapshot.Models {
		nextTotal := ledger.treeTotal
		if err := addModelUsage(&nextTotal, model); err != nil {
			return nil, fmt.Errorf("agentexec: restore usage aggregate: %w", err)
		}
		if err := validateFrameworkUsageCapacity(nextTotal); err != nil {
			return nil, fmt.Errorf("agentexec: restore usage aggregate: %w", err)
		}
		ledger.treeTotal = nextTotal
		ledger.treeModels[model.Model] = model
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

func (l *usageLedger) register(process ProcessRef) error {
	if l == nil {
		return errors.New("agentexec: usage ledger is nil")
	}
	if err := validateProcessRef(process); err != nil {
		return l.reject(err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.projectionErr != nil {
		return l.projectionErr
	}
	_, err := l.registerLocked(process)
	if err != nil {
		return l.rejectLocked(err)
	}
	return nil
}

func validateProcessRef(process ProcessRef) error {
	if process.ID == "" {
		return errors.New("agentexec: usage process id is required")
	}
	if process.ParentID == process.ID {
		return fmt.Errorf("agentexec: usage process %q cannot parent itself", process.ID)
	}
	if process.ParentID == "" && process.SpawnCallID != "" {
		return fmt.Errorf(
			"agentexec: root usage process %q cannot have spawn call %q",
			process.ID,
			process.SpawnCallID,
		)
	}
	return nil
}

func (l *usageLedger) registerLocked(process ProcessRef) (*processUsage, error) {
	if current, ok := l.byProcess[process.ID]; ok {
		if current.ref != process {
			return nil, fmt.Errorf(
				"agentexec: usage process %q changed immutable identity from %+v to %+v",
				process.ID,
				current.ref,
				process,
			)
		}
		return current, nil
	}
	if process.ParentID != "" {
		if _, ok := l.byProcess[process.ParentID]; !ok {
			return nil, fmt.Errorf(
				"agentexec: usage process %q references unregistered parent %q",
				process.ID,
				process.ParentID,
			)
		}
	}
	current := &processUsage{
		ref:          process,
		directModels: make(map[string]accounting.ModelUsage),
	}
	l.byProcess[process.ID] = current
	return current, nil
}

func (l *usageLedger) record(process ProcessRef, response *chat.Response, cost float64) error {
	if l == nil {
		return errors.New("agentexec: usage ledger is nil")
	}
	if err := validateProcessRef(process); err != nil {
		return l.reject(err)
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
	processState, err := l.registerLocked(process)
	if err != nil {
		return l.rejectLocked(err)
	}
	current, exists := l.treeModels[model]
	nextModel := delta
	if exists {
		nextModel = current
		if err := addModelUsage(&nextModel, delta); err != nil {
			return l.rejectLocked(fmt.Errorf("agentexec: record model usage: %w", err))
		}
	}
	nextTotal := l.treeTotal
	if err := addModelUsage(&nextTotal, delta); err != nil {
		return l.rejectLocked(fmt.Errorf("agentexec: record total usage: %w", err))
	}
	if err := validateFrameworkUsageCapacity(nextTotal); err != nil {
		return l.rejectLocked(fmt.Errorf("agentexec: record total usage: %w", err))
	}

	processModel, processModelExists := processState.directModels[model]
	nextProcessModel := delta
	if processModelExists {
		nextProcessModel = processModel
		if err := addModelUsage(&nextProcessModel, delta); err != nil {
			return l.rejectLocked(fmt.Errorf(
				"agentexec: record usage for process %q: %w",
				process.ID,
				err,
			))
		}
	}
	nextProcessTotal := processState.directTotal
	if err := addModelUsage(&nextProcessTotal, delta); err != nil {
		return l.rejectLocked(fmt.Errorf(
			"agentexec: record total usage for process %q: %w",
			process.ID,
			err,
		))
	}
	if err := validateFrameworkUsageCapacity(nextProcessTotal); err != nil {
		return l.rejectLocked(fmt.Errorf(
			"agentexec: record total usage for process %q: %w",
			process.ID,
			err,
		))
	}

	l.treeModels[model] = nextModel
	l.treeTotal = nextTotal
	processState.directModels[model] = nextProcessModel
	processState.directTotal = nextProcessTotal
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

func (l *usageLedger) output(process ProcessRef, reply string, stopReason agent.InteractionStopReason) (TurnOutput, error) {
	if !stopReason.Valid() {
		return TurnOutput{}, fmt.Errorf("agentexec: invalid interaction stop reason %q", stopReason)
	}
	if err := l.recordStopReason(process, stopReason); err != nil {
		return TurnOutput{}, err
	}
	subtree, err := l.subtree(process)
	if err != nil {
		return TurnOutput{}, err
	}
	return TurnOutput{
		Reply:        reply,
		Usage:        subtree.total.TokenUsage,
		UsageByModel: slices.Clone(subtree.snapshot.Models),
		CostUSD:      subtree.total.CostUSD,
		Steps:        subtree.total.Calls,
		StopReason:   stopReason,
	}, nil
}

func (l *usageLedger) recordStopReason(process ProcessRef, stopReason agent.InteractionStopReason) error {
	if l == nil {
		return errors.New("agentexec: usage ledger is nil")
	}
	if err := validateProcessRef(process); err != nil {
		return l.reject(err)
	}
	if !stopReason.Valid() {
		return l.reject(fmt.Errorf(
			"agentexec: process %q produced invalid interaction stop reason %q",
			process.ID,
			stopReason,
		))
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.projectionErr != nil {
		return l.projectionErr
	}
	current, err := l.registerLocked(process)
	if err != nil {
		return l.rejectLocked(err)
	}
	if current.hasStopReason && current.stopReason != stopReason {
		return l.rejectLocked(fmt.Errorf(
			"agentexec: process %q changed terminal interaction stop reason from %q to %q",
			process.ID,
			current.stopReason,
			stopReason,
		))
	}
	current.stopReason = stopReason
	current.hasStopReason = true
	return nil
}

func (l *usageLedger) state() (accounting.Snapshot, accounting.ModelUsage, error) {
	if l == nil {
		return accounting.Snapshot{}, accounting.ModelUsage{}, errors.New("agentexec: usage ledger is nil")
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return usageSnapshot(l.treeModels), l.treeTotal, l.projectionErr
}

func (l *usageLedger) subtree(process ProcessRef) (subtreeUsage, error) {
	if l == nil {
		return subtreeUsage{}, errors.New("agentexec: usage ledger is nil")
	}
	if err := validateProcessRef(process); err != nil {
		return subtreeUsage{}, err
	}

	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.projectionErr != nil {
		return subtreeUsage{}, l.projectionErr
	}
	current, ok := l.byProcess[process.ID]
	if !ok {
		return subtreeUsage{}, fmt.Errorf(
			"agentexec: usage process %q is not registered",
			process.ID,
		)
	}
	if current.ref != process {
		return subtreeUsage{}, fmt.Errorf(
			"agentexec: usage process %q identity mismatch: have %+v, requested %+v",
			process.ID,
			current.ref,
			process,
		)
	}

	// A restored checkpoint retains one exact root aggregate but intentionally
	// does not invent historical per-child attribution. Root state therefore
	// reads the canonical aggregate directly; first-class children are not
	// restorable until the tree-barrier/recovery phases persist that detail.
	if !process.Child() {
		return subtreeUsage{
			snapshot:   usageSnapshot(l.treeModels),
			total:      l.treeTotal,
			stopReason: current.stopReason,
		}, nil
	}

	models := make(map[string]accounting.ModelUsage)
	var total accounting.ModelUsage
	for processID, candidate := range l.byProcess {
		descendant, err := l.descendsFromLocked(processID, process.ID)
		if err != nil {
			return subtreeUsage{}, err
		}
		if !descendant {
			continue
		}
		for model, usage := range candidate.directModels {
			next := models[model]
			if next.Model == "" {
				next.Model = model
			}
			if err := addModelUsage(&next, usage); err != nil {
				return subtreeUsage{}, fmt.Errorf(
					"agentexec: aggregate subtree %q model %q: %w",
					process.ID,
					model,
					err,
				)
			}
			models[model] = next
		}
		if err := addModelUsage(&total, candidate.directTotal); err != nil {
			return subtreeUsage{}, fmt.Errorf(
				"agentexec: aggregate subtree %q total: %w",
				process.ID,
				err,
			)
		}
	}
	return subtreeUsage{
		snapshot:   usageSnapshot(models),
		total:      total,
		stopReason: current.stopReason,
	}, nil
}

func (l *usageLedger) descendsFromLocked(processID, ancestorID string) (bool, error) {
	visited := make(map[string]struct{})
	for processID != "" {
		if processID == ancestorID {
			return true, nil
		}
		if _, duplicate := visited[processID]; duplicate {
			return false, fmt.Errorf("agentexec: usage lineage contains a cycle at process %q", processID)
		}
		visited[processID] = struct{}{}
		current, ok := l.byProcess[processID]
		if !ok {
			return false, fmt.Errorf(
				"agentexec: usage lineage references unregistered process %q",
				processID,
			)
		}
		processID = current.ref.ParentID
	}
	return false, nil
}

func usageSnapshot(models map[string]accounting.ModelUsage) accounting.Snapshot {
	names := make([]string, 0, len(models))
	for model := range models {
		names = append(names, model)
	}
	slices.Sort(names)
	snapshot := accounting.Snapshot{Models: make([]accounting.ModelUsage, 0, len(names))}
	for _, model := range names {
		snapshot.Models = append(snapshot.Models, models[model])
	}
	return snapshot
}

type processProjection struct {
	engine      *Engine
	provider    string
	usage       *usageLedger
	observation *toolObservation
	observer    executionObserver
}

var (
	_ core.InteractionCostProjector = (*processProjection)(nil)
	_ agentruntime.EventListener    = (*processProjection)(nil)
)

func (*processProjection) Name() string { return "lyra:process-projection" }

func (p *processProjection) ProjectInteractionCost(
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

func (p *processProjection) OnEvent(_ context.Context, published event.Event) {
	if p == nil || published == nil || p.usage == nil {
		return
	}
	process, err := p.resolveProcess(published.ProcessID())
	if err != nil {
		_ = p.usage.reject(err)
		return
	}
	ref := processRef(process)
	if err := p.usage.register(ref); err != nil {
		return
	}

	interactionBoundary, ok := published.(event.InteractionBoundary)
	if ok {
		p.projectInteraction(ref, interactionBoundary.Boundary)
		return
	}
	status, terminalErr, terminal := terminalProcessStatus(published)
	if !terminal || !ref.AgentToolChild() || p.observer == nil {
		return
	}

	subtree, usageErr := p.usage.subtree(ref)
	p.observer.OnChildProcessEnd(ChildCompletion{
		Process: ChildProcess{
			ProcessRef: ref,
			StartedAt:  process.StartedAt().UTC(),
		},
		Status:       status,
		StopReason:   subtree.stopReason,
		Usage:        subtree.total.TokenUsage,
		UsageByModel: slices.Clone(subtree.snapshot.Models),
		CostUSD:      subtree.total.CostUSD,
		Steps:        subtree.total.Calls,
		Err:          errors.Join(terminalErr, usageErr),
		CompletedAt:  published.Timestamp().UTC(),
	})
}

func (p *processProjection) projectInteraction(process ProcessRef, boundary interaction.Event) {
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
	if err := p.usage.record(process, boundary.Response, boundary.Cost); err != nil {
		return
	}
	if p.observer == nil {
		return
	}
	cumulative, err := p.usage.subtree(process)
	if err != nil {
		return
	}
	p.observer.OnUsage(process, UsageProgress{
		Usage:         cumulative.total.TokenUsage,
		UsageByModel:  slices.Clone(cumulative.snapshot.Models),
		CostUSD:       cumulative.total.CostUSD,
		Steps:         cumulative.total.Calls,
		ContextTokens: boundary.Response.Usage.InputTokens,
	})
}

func (p *processProjection) resolveProcess(processID string) (core.ProcessView, error) {
	if p.engine == nil || p.engine.runtime == nil {
		return nil, errors.New("agentexec: process projection requires Agent Runtime")
	}
	process, ok := p.engine.runtime.Process(processID)
	if !ok {
		return nil, fmt.Errorf(
			"agentexec: process projection cannot resolve process %q",
			processID,
		)
	}
	return process, nil
}

func terminalProcessStatus(published event.Event) (core.ProcessStatus, error, bool) {
	switch value := published.(type) {
	case event.ProcessCompleted:
		return core.StatusCompleted, nil, true
	case event.ProcessFailed:
		return core.StatusFailed, value.Err, true
	case event.ProcessStuck:
		return core.StatusStuck, nil, true
	case event.ProcessKilled:
		return core.StatusKilled, nil, true
	case event.ProcessTerminated:
		return core.StatusTerminated, nil, true
	default:
		return core.StatusNotStarted, nil, false
	}
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

	application, err := snapshot.Total()
	if err != nil {
		return fmt.Errorf("%w: application usage aggregate: %w", core.ErrInvalidSnapshot, err)
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
