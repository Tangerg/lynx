package agentexec

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

// interactionAccounting owns per-Process model usage, usage restored from
// retired Processes, and the root's Tool-call count. These facts share the
// accounting snapshot/checkpoint invariant but not the Process-tree lock.
type interactionAccounting struct {
	mu             sync.Mutex
	usageByProcess map[agent.ProcessID]map[string]accounting.ModelUsage
	carriedUsage   map[string]accounting.ModelUsage
	provider       string
	fallbackModel  string
	pricing        accounting.Pricing
	toolCalls      int
}

func newInteractionAccounting(
	provider string,
	fallbackModel string,
	pricing accounting.Pricing,
) interactionAccounting {
	return interactionAccounting{
		usageByProcess: make(map[agent.ProcessID]map[string]accounting.ModelUsage),
		carriedUsage:   make(map[string]accounting.ModelUsage),
		provider:       provider,
		fallbackModel:  fallbackModel,
		pricing:        pricing,
	}
}

func (meter *interactionAccounting) providerName() string { return meter.provider }

func (meter *interactionAccounting) recordToolCall() {
	meter.mu.Lock()
	meter.toolCalls++
	meter.mu.Unlock()
}

func (meter *interactionAccounting) toolCallCount() int {
	meter.mu.Lock()
	defer meter.mu.Unlock()
	return meter.toolCalls
}

func (meter *interactionAccounting) snapshot() accounting.Snapshot {
	meter.mu.Lock()
	defer meter.mu.Unlock()
	byModel := make(map[string]accounting.ModelUsage)
	mergeInteractionUsage(byModel, meter.carriedUsage)
	for _, processUsage := range meter.usageByProcess {
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

func (meter *interactionAccounting) restore(
	usageByProcess map[agent.ProcessID]map[string]accounting.ModelUsage,
	carriedUsage map[string]accounting.ModelUsage,
) {
	meter.mu.Lock()
	meter.usageByProcess = usageByProcess
	meter.carriedUsage = carriedUsage
	meter.mu.Unlock()
}

func (meter *interactionAccounting) checkpointLocked() (
	map[agent.ProcessID]map[string]accounting.ModelUsage,
	map[string]accounting.ModelUsage,
) {
	usageByProcess := make(map[agent.ProcessID]map[string]accounting.ModelUsage, len(meter.usageByProcess))
	for processID, byModel := range meter.usageByProcess {
		usageByProcess[processID] = maps.Clone(byModel)
	}
	return usageByProcess, maps.Clone(meter.carriedUsage)
}

func (session *interactionSession) interactionCheckpointPayload(
	tree agent.TreeSnapshot,
) ([]byte, error) {
	// Accounting and pending steers were one lock domain before P113. Hold both
	// owners while copying so the checkpoint retains the same atomic snapshot,
	// without making every model call contend with Process lifecycle transitions.
	session.accounting.mu.Lock()
	session.state.mu.Lock()
	usageByProcess, carried := session.accounting.checkpointLocked()
	pendingSteers := make(map[agent.SignalID]pendingInteractionSteer, len(session.state.pendingSteers))
	for signalID, pending := range session.state.pendingSteers {
		pendingSteers[signalID] = pendingInteractionSteer{content: transcript.CloneContent(pending.content)}
	}
	session.state.mu.Unlock()
	session.accounting.mu.Unlock()

	instructions, err := interactionInstructionContext(session.start.WorkingContext)
	if err != nil {
		return nil, err
	}
	return encodeInteractionCheckpointPayload(tree, usageByProcess, carried, instructions, pendingSteers)
}

func (meter *interactionAccounting) accountModelCall(
	invocation interaction.ModelInvocation,
	callID string,
	response *corechat.Response,
) (runs.ModelCallCompleted, error) {
	delta := modelUsage(response, meter.provider, meter.fallbackModel, meter.pricing)
	if err := delta.Validate(); err != nil {
		return runs.ModelCallCompleted{}, fmt.Errorf("agentexec: account model call: %w", err)
	}
	meter.mu.Lock()
	defer meter.mu.Unlock()
	processID := invocation.Relation().ProcessID()
	usageByModel := meter.usageByProcess[processID]
	if usageByModel == nil {
		usageByModel = make(map[string]accounting.ModelUsage)
		meter.usageByProcess[processID] = usageByModel
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
	choice := response.First()
	if choice == nil || choice.Message == nil {
		return runs.ModelCallCompleted{}, errors.New("agentexec: account model call without an assistant message")
	}
	return runs.ModelCallCompleted{
		CallID: callID, Message: choice.Message.Clone(), TokenUsage: total.TokenUsage,
		ByModel: slices.Clone(models), CostUSD: total.CostUSD, Steps: total.Calls,
		ContextTokens: response.Usage.InputTokens,
	}, nil
}

func (meter *interactionAccounting) segmentUsage(processID agent.ProcessID) *runs.SegmentUsage {
	meter.mu.Lock()
	usageByModel := meter.usageByProcess[processID]
	models := make([]accounting.ModelUsage, 0, len(usageByModel))
	for _, usage := range usageByModel {
		models = append(models, usage)
	}
	meter.mu.Unlock()
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
