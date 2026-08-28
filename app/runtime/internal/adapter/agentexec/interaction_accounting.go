package agentexec

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

// interactionAccounting owns per-Process model usage, usage restored from
// retired Processes, and the root's Tool-call count. These facts share the
// accounting snapshot/checkpoint invariant but not the Process-tree lock.
type interactionAccounting struct {
	mu                       sync.Mutex
	usageByProcess           map[agent.ProcessID]map[string]accounting.ModelUsage
	carriedUsage             map[string]accounting.ModelUsage
	contextByProcess         map[agent.ProcessID]ModelContextTokenCalibration
	preparedContextByProcess map[agent.ProcessID]preparedModelContext
	provider                 string
	fallbackModel            string
	pricing                  accounting.Pricing
	toolCalls                int
}

type preparedModelContext struct {
	effectID  agent.EffectID
	sequence  uint32
	estimated int
}

func newInteractionAccounting(
	provider string,
	fallbackModel string,
	pricing accounting.Pricing,
) interactionAccounting {
	return interactionAccounting{
		usageByProcess:           make(map[agent.ProcessID]map[string]accounting.ModelUsage),
		carriedUsage:             make(map[string]accounting.ModelUsage),
		contextByProcess:         make(map[agent.ProcessID]ModelContextTokenCalibration),
		preparedContextByProcess: make(map[agent.ProcessID]preparedModelContext),
		provider:                 provider,
		fallbackModel:            fallbackModel,
		pricing:                  pricing,
	}
}

func (i *interactionAccounting) modelContextCalibration(
	invocation interaction.ModelInvocation,
) ModelContextTokenCalibration {
	if i == nil || !invocation.Valid() {
		return ModelContextTokenCalibration{}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.contextByProcess[invocation.Relation().ProcessID()]
}

func (i *interactionAccounting) prepareModelContext(
	invocation interaction.ModelInvocation,
	estimated int,
) error {
	if i == nil || !invocation.Valid() || estimated <= 0 {
		return errors.New("agentexec: prepare model context requires valid attribution and estimate")
	}
	processID := invocation.Relation().ProcessID()
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, exists := i.preparedContextByProcess[processID]; exists {
		return fmt.Errorf("agentexec: Process %s already has a prepared model context", processID)
	}
	i.preparedContextByProcess[processID] = preparedModelContext{
		effectID: invocation.EffectID(), sequence: invocation.ModelCallSequence(), estimated: estimated,
	}
	return nil
}

func (i *interactionAccounting) providerName() string { return i.provider }

func (i *interactionAccounting) recordToolCall() {
	i.mu.Lock()
	i.toolCalls++
	i.mu.Unlock()
}

func (i *interactionAccounting) toolCallCount() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.toolCalls
}

func (i *interactionAccounting) snapshot() accounting.Snapshot {
	i.mu.Lock()
	defer i.mu.Unlock()
	byModel := make(map[string]accounting.ModelUsage)
	mergeInteractionUsage(byModel, i.carriedUsage)
	for _, processUsage := range i.usageByProcess {
		mergeInteractionUsage(byModel, processUsage)
	}
	models := make([]accounting.ModelUsage, 0, len(byModel))
	for _, usage := range byModel {
		models = append(models, usage)
	}
	slices.SortFunc(models, func(left, right accounting.ModelUsage) int {
		return strings.Compare(left.Model, right.Model)
	})
	return accounting.Snapshot{Models: models}
}

func mergeInteractionUsage(
	target map[string]accounting.ModelUsage,
	source map[string]accounting.ModelUsage,
) {
	for model, usage := range source {
		current := target[model]
		if current.Model == "" {
			current.Model = model
		}
		current.Add(usage.TokenUsage)
		current.CostUSD += usage.CostUSD
		current.Calls += usage.Calls
		target[model] = current
	}
}

func (i *interactionAccounting) restore(
	usageByProcess map[agent.ProcessID]map[string]accounting.ModelUsage,
	carriedUsage map[string]accounting.ModelUsage,
	contextByProcess map[agent.ProcessID]ModelContextTokenCalibration,
) {
	i.mu.Lock()
	i.usageByProcess = usageByProcess
	i.carriedUsage = carriedUsage
	i.contextByProcess = contextByProcess
	i.preparedContextByProcess = make(map[agent.ProcessID]preparedModelContext)
	i.mu.Unlock()
}

func (i *interactionAccounting) checkpointLocked() (
	map[agent.ProcessID]map[string]accounting.ModelUsage,
	map[string]accounting.ModelUsage,
	map[agent.ProcessID]ModelContextTokenCalibration,
) {
	usageByProcess := make(map[agent.ProcessID]map[string]accounting.ModelUsage, len(i.usageByProcess))
	for processID, byModel := range i.usageByProcess {
		usageByProcess[processID] = maps.Clone(byModel)
	}
	return usageByProcess, maps.Clone(i.carriedUsage), maps.Clone(i.contextByProcess)
}

func (i *interactionSession) interactionCheckpointPayload(
	tree agent.TreeSnapshot,
) ([]byte, error) {
	// Accounting and pending steers were one lock domain before P113. Hold both
	// owners while copying so the checkpoint retains the same atomic snapshot,
	// without making every model call contend with Process lifecycle transitions.
	i.accounting.mu.Lock()
	i.state.mu.Lock()
	usageByProcess, carried, contexts := i.accounting.checkpointLocked()
	pendingSteers := make(map[agent.SignalID]pendingInteractionSteer, len(i.state.pendingSteers))
	for signalID, pending := range i.state.pendingSteers {
		pendingSteers[signalID] = pendingInteractionSteer{content: transcript.CloneContent(pending.content)}
	}
	i.state.mu.Unlock()
	i.accounting.mu.Unlock()

	instructions, err := interactionInstructionContext(i.start.WorkingContext)
	if err != nil {
		return nil, err
	}
	return encodeInteractionCheckpointPayload(
		tree,
		usageByProcess,
		carried,
		contexts,
		instructions,
		pendingSteers,
	)
}

func (i *interactionAccounting) accountModelCall(
	invocation interaction.ModelInvocation,
	callID string,
	response *corechat.Response,
) (runs.ModelCallCompleted, error) {
	delta := modelUsage(response, i.provider, i.fallbackModel, i.pricing)
	if err := delta.Validate(); err != nil {
		return runs.ModelCallCompleted{}, fmt.Errorf("agentexec: account model call: %w", err)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	processID := invocation.Relation().ProcessID()
	prepared, preparedFound := i.preparedContextByProcess[processID]
	if preparedFound && (prepared.effectID != invocation.EffectID() ||
		prepared.sequence != invocation.ModelCallSequence()) {
		return runs.ModelCallCompleted{}, errors.New("agentexec: model response has no matching prepared context")
	}
	usageByModel := i.usageByProcess[processID]
	if usageByModel == nil {
		usageByModel = make(map[string]accounting.ModelUsage)
		i.usageByProcess[processID] = usageByModel
	}
	current := usageByModel[delta.Model]
	if current.Model == "" {
		current.Model = delta.Model
	}
	current.Add(delta.TokenUsage)
	current.CostUSD += delta.CostUSD
	current.Calls += delta.Calls
	if err := current.Validate(); err != nil {
		return runs.ModelCallCompleted{}, fmt.Errorf("agentexec: aggregate model call: %w", err)
	}
	usageByModel[delta.Model] = current
	models := make([]accounting.ModelUsage, 0, len(usageByModel))
	for _, usage := range usageByModel {
		models = append(models, usage)
	}
	slices.SortFunc(models, func(left, right accounting.ModelUsage) int {
		return strings.Compare(left.Model, right.Model)
	})
	total, err := (accounting.Snapshot{Models: models}).Total()
	if err != nil {
		return runs.ModelCallCompleted{}, fmt.Errorf("agentexec: total model usage: %w", err)
	}
	if total.Calls != int(invocation.ModelCallSequence()) {
		return runs.ModelCallCompleted{}, fmt.Errorf(
			"agentexec: model call sequence %d differs from accounted calls %d",
			invocation.ModelCallSequence(), total.Calls,
		)
	}
	modelOutput := response.Output
	if modelOutput == nil || modelOutput.Message == nil {
		return runs.ModelCallCompleted{}, errors.New("agentexec: account model call without an assistant message")
	}
	var usage corechat.Usage
	if response.Metadata != nil {
		usage = response.Metadata.Usage
	}
	if preparedFound {
		delete(i.preparedContextByProcess, processID)
	}
	if preparedFound && usage.InputTokens > 0 {
		calibration, err := NewModelContextTokenCalibration(usage.InputTokens, prepared.estimated)
		if err != nil {
			return runs.ModelCallCompleted{}, err
		}
		i.contextByProcess[processID] = calibration
	}
	return runs.ModelCallCompleted{
		CallID: callID, Message: modelOutput.Message.Clone(), TokenUsage: total.TokenUsage,
		ByModel: slices.Clone(models), CostUSD: total.CostUSD, Steps: total.Calls,
		ContextTokens: usage.InputTokens,
	}, nil
}

func (i *interactionAccounting) segmentUsage(processID agent.ProcessID) *runs.SegmentUsage {
	i.mu.Lock()
	usageByModel := i.usageByProcess[processID]
	models := make([]accounting.ModelUsage, 0, len(usageByModel))
	for _, usage := range usageByModel {
		models = append(models, usage)
	}
	i.mu.Unlock()
	slices.SortFunc(models, func(left, right accounting.ModelUsage) int {
		return strings.Compare(left.Model, right.Model)
	})
	if len(models) == 0 {
		return nil
	}
	total, err := (accounting.Snapshot{Models: models}).Total()
	if err != nil {
		return nil
	}
	return &runs.SegmentUsage{
		Tokens: total.TokenUsage, ByModel: models,
		CostUSD: total.CostUSD, Steps: total.Calls,
	}
}
