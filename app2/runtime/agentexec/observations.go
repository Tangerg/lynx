package agentexec

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

// ExecutableTool binds one executable capability to the Lyra safety class
// used by transcript and approval projections.
type ExecutableTool struct {
	Tool        tool.Tool
	SafetyClass protocol.SafetyClass
}

type ModelObservation struct {
	Sequence   int
	OccurredAt time.Time
	Response   *chat.Response
}

type ToolObservation struct {
	ItemID, CallID, Name       string
	Arguments                  map[string]any
	SafetyClass                protocol.SafetyClass
	CommittedPlan              *protocol.Plan
	ModelCallSequence          int
	ToolCallIndex              int
	StartedAt, FinishedAt      time.Time
	Result                     string
	IsError, Waiting, Unknown  bool
	Failure                    string
}

// ToolItemID derives the stable transcript identity for one provider ToolCall.
// It survives an Interaction checkpoint/restore without persisting a second
// identity mapping.
func ToolItemID(runID, callID string) string {
	digest := sha256.Sum256([]byte(runID + "\x00" + callID))
	return "itm_" + hex.EncodeToString(digest[:16])
}

type executionObserver struct {
	mu             sync.Mutex
	runID          string
	defaultModel   string
	safetyByName   map[string]protocol.SafetyClass
	models         []ModelObservation
	tools          map[string]ToolObservation
	plans          map[string]protocol.Plan
}

func newExecutionObserver(runID, model string) *executionObserver {
	return &executionObserver{
		runID: runID, defaultModel: model, safetyByName: make(map[string]protocol.SafetyClass),
		tools: make(map[string]ToolObservation), plans: make(map[string]protocol.Plan),
	}
}

func (observer *executionObserver) bindTools(executables []ExecutableTool) {
	for _, executable := range executables {
		observer.safetyByName[executable.Tool.Definition().Name] = executable.SafetyClass
	}
}

func (observer *executionObserver) RecordCommittedPlan(callID string, plan protocol.Plan) {
	observer.mu.Lock()
	observer.plans[callID] = clonePlan(plan)
	observer.mu.Unlock()
}

func (observer *executionObserver) OnModelResponse(_ context.Context, invocation interaction.ModelInvocation, response *chat.Response) {
	observer.mu.Lock()
	observer.models = append(observer.models, ModelObservation{
		Sequence: int(invocation.ModelCallSequence()), OccurredAt: time.Now().UTC(), Response: response.Clone(),
	})
	observer.mu.Unlock()
}

func (observer *executionObserver) OnToolStarted(_ context.Context, invocation interaction.ToolInvocation) {
	call := invocation.ToolCall()
	arguments := decodeArguments(call.Arguments)
	observer.mu.Lock()
	observer.tools[call.ID] = ToolObservation{
		ItemID: ToolItemID(observer.runID, call.ID), CallID: call.ID, Name: call.Name,
		Arguments: arguments, SafetyClass: observer.safetyByName[call.Name],
		ModelCallSequence: int(invocation.ModelCallSequence()), ToolCallIndex: int(invocation.ToolCallIndex()),
		StartedAt: time.Now().UTC(),
	}
	observer.mu.Unlock()
}

func (observer *executionObserver) OnToolSettled(_ context.Context, invocation interaction.ToolInvocation, settlement interaction.ToolSettlement) {
	call := invocation.ToolCall()
	observer.mu.Lock()
	value, found := observer.tools[call.ID]
	if !found {
		value = ToolObservation{
			ItemID: ToolItemID(observer.runID, call.ID), CallID: call.ID, Name: call.Name,
			Arguments: decodeArguments(call.Arguments), SafetyClass: observer.safetyByName[call.Name],
			ModelCallSequence: int(invocation.ModelCallSequence()), ToolCallIndex: int(invocation.ToolCallIndex()),
			StartedAt: time.Now().UTC(),
		}
	}
	value.Waiting = settlement.InputRequired
	value.Unknown = settlement.Unknown
	value.Failure = settlement.Failure
	if settlement.Result != nil {
		value.Result = settlement.Result.Result
		value.IsError = settlement.Result.IsError
	}
	if !value.Waiting {
		value.FinishedAt = time.Now().UTC()
	}
	observer.tools[call.ID] = value
	observer.mu.Unlock()
}

func (observer *executionObserver) snapshot() ([]ModelObservation, []ToolObservation, protocol.Usage) {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	models := make([]ModelObservation, len(observer.models))
	usage := protocol.Usage{ByModel: make(map[string]protocol.ModelUsage)}
	for index, value := range observer.models {
		models[index] = ModelObservation{Sequence: value.Sequence, OccurredAt: value.OccurredAt, Response: value.Response.Clone()}
		model := value.Response.Model
		if model == "" {
			model = observer.defaultModel
		}
		addUsage(&usage, model, presentUsage(value.Response.Usage).ModelUsage)
	}
	sort.SliceStable(models, func(left, right int) bool { return models[left].Sequence < models[right].Sequence })
	tools := make([]ToolObservation, 0, len(observer.tools))
	for _, value := range observer.tools {
		value.Arguments = cloneObject(value.Arguments)
		if plan, found := observer.plans[value.CallID]; found {
			clone := clonePlan(plan)
			value.CommittedPlan = &clone
		}
		tools = append(tools, value)
	}
	sort.SliceStable(tools, func(left, right int) bool {
		if tools[left].ModelCallSequence != tools[right].ModelCallSequence {
			return tools[left].ModelCallSequence < tools[right].ModelCallSequence
		}
		return tools[left].ToolCallIndex < tools[right].ToolCallIndex
	})
	if len(usage.ByModel) == 0 {
		usage.ByModel = nil
	}
	return models, tools, usage
}

func clonePlan(value protocol.Plan) protocol.Plan {
	value.Steps = append(make([]protocol.PlanStep, 0, len(value.Steps)), value.Steps...)
	return value
}

func decodeArguments(raw string) map[string]any {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return map[string]any{}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return map[string]any{}
	}
	return value
}

func cloneObject(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var clone map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&clone); err != nil {
		return map[string]any{}
	}
	return clone
}

func addUsage(total *protocol.Usage, model string, value protocol.ModelUsage) {
	total.InputTokens += value.InputTokens
	total.OutputTokens += value.OutputTokens
	total.CacheReadTokens += value.CacheReadTokens
	total.CacheWriteTokens += value.CacheWriteTokens
	total.ReasoningTokens += value.ReasoningTokens
	byModel := total.ByModel[model]
	byModel.InputTokens += value.InputTokens
	byModel.OutputTokens += value.OutputTokens
	byModel.CacheReadTokens += value.CacheReadTokens
	byModel.CacheWriteTokens += value.CacheWriteTokens
	byModel.ReasoningTokens += value.ReasoningTokens
	total.ByModel[model] = byModel
}

var _ interaction.ExecutionObserver = (*executionObserver)(nil)
var _ ToolFactSink = (*executionObserver)(nil)
