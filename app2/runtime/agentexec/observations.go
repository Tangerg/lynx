package agentexec

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	agent "github.com/Tangerg/lynx/agent"
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
	// IntrinsicInput marks a capability whose visible identity is a Question,
	// not a provisional ToolCall. It is projected only after its typed input
	// request exists, avoiding a wire-level Item type mutation.
	IntrinsicInput bool
}

type ModelObservation struct {
	RunID, SegmentID string
	EffectID   string
	Sequence   int
	OccurredAt time.Time
	Response   *chat.Response
}

// ModelDeltaKind is the provider-neutral live material that Lyra can project
// without exposing Agent Framework identities or provider chunk shapes.
type ModelDeltaKind string

const (
	ModelDeltaContent   ModelDeltaKind = "content"
	ModelDeltaReasoning ModelDeltaKind = "reasoning"
)

// ModelDelta is a best-effort append to one stable model-produced Item. Index
// addresses a ContentBlock for content deltas and a reasoning Item for
// reasoning deltas.
type ModelDelta struct {
	RunID, SegmentID string
	EffectID       string
	EffectSequence uint64
	OccurredAt     time.Time
	Kind           ModelDeltaKind
	Index          int
	Text           string
}

// LiveObservationSink must return in bounded time. Delivery is observational:
// a dropped update never changes the settled executor Output, which remains
// the authoritative fallback at the segment boundary.
type LiveObservationSink interface {
	OfferModelDelta(ModelDelta)
	OfferModelProgress(ModelProgress)
	OfferToolStarted(ToolObservation)
	OfferToolSettled(ToolObservation)
}

// ModelProgress is emitted after one complete model call. Usage belongs to that
// call; the Run owner combines it with the durable run-cumulative baseline.
type ModelProgress struct {
	RunID, SegmentID string
	Sequence      int
	OccurredAt    time.Time
	Usage         protocol.Usage
	ContextTokens int64
	Model         string
}

type ToolObservation struct {
	RunID, SegmentID          string
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
	IntrinsicInput             bool
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
	segmentID      string
	defaultModel   string
	safetyByName   map[string]protocol.SafetyClass
	intrinsicInput map[string]bool
	models         []ModelObservation
	tools          map[observationIdentity]ToolObservation
	plans          map[observationIdentity]protocol.Plan
	live           LiveObservationSink
	streams        map[string]*modelStream
	delegation     *delegationBridge
}

type modelStream struct {
	accumulator chat.ResponseAccumulator
	startedAt   time.Time
}

type observationIdentity struct {
	runID, sourceID string
}

func newExecutionObserver(runID, segmentID, model string, live LiveObservationSink, delegation *delegationBridge) *executionObserver {
	return &executionObserver{
		runID: runID, segmentID: segmentID, defaultModel: model, safetyByName: make(map[string]protocol.SafetyClass),
		intrinsicInput: make(map[string]bool),
		tools: make(map[observationIdentity]ToolObservation), plans: make(map[observationIdentity]protocol.Plan), live: live,
		streams: make(map[string]*modelStream), delegation: delegation,
	}
}

func (observer *executionObserver) bindTools(executables []ExecutableTool) {
	for _, executable := range executables {
		name := executable.Tool.Definition().Name
		observer.safetyByName[name] = executable.SafetyClass
		observer.intrinsicInput[name] = executable.IntrinsicInput
	}
}

func (observer *executionObserver) RecordCommittedPlan(callID string, plan protocol.Plan) {
	observer.recordCommittedPlanFor(observer.runID, callID, plan)
}

func (observer *executionObserver) recordCommittedPlanFor(runID, callID string, plan protocol.Plan) {
	observer.mu.Lock()
	observer.plans[observationIdentity{runID: runID, sourceID: callID}] = clonePlan(plan)
	observer.mu.Unlock()
}

func (observer *executionObserver) OnModelResponse(_ context.Context, invocation interaction.ModelInvocation, response *chat.Response) {
	scope, found := observer.scope(invocation.Relation())
	if !found {
		return
	}
	effectID := invocation.EffectID().String()
	streamID := observationIdentity{runID: scope.runID, sourceID: effectID}
	occurredAt := time.Now().UTC()
	observer.mu.Lock()
	if stream := observer.streams[streamID.String()]; stream != nil && !stream.startedAt.IsZero() {
		occurredAt = stream.startedAt
	}
	if observer.delegation != nil {
		observer.delegation.register(invocation, response, occurredAt)
	}
	observer.models = append(observer.models, ModelObservation{
		RunID: scope.runID, SegmentID: scope.segmentID,
		EffectID: effectID, Sequence: int(invocation.ModelCallSequence()), OccurredAt: occurredAt, Response: response.Clone(),
	})
	observer.mu.Unlock()
	if observer.live != nil {
		model := response.Model
		if model == "" {
			model = observer.defaultModel
		}
		usage := usageForModel(model, response.Usage)
		observer.live.OfferModelProgress(ModelProgress{
			RunID: scope.runID, SegmentID: scope.segmentID,
			Sequence: int(invocation.ModelCallSequence()), OccurredAt: time.Now().UTC(),
			Usage: usage, ContextTokens: response.Usage.InputTokens, Model: model,
		})
	}
}

func (observer *executionObserver) OnDelta(_ context.Context, delta agent.Delta) {
	if observer.live == nil {
		return
	}
	decoded, err := interaction.ParseModelResponseDelta(delta.Payload())
	if err != nil {
		return
	}
	effectID := delta.EffectID().String()
	scope, found := observer.scopeProcess(delta.ProcessID())
	if !found {
		return
	}
	streamID := observationIdentity{runID: scope.runID, sourceID: effectID}
	observer.mu.Lock()
	stream := observer.streams[streamID.String()]
	if stream == nil {
		stream = &modelStream{startedAt: delta.EmittedAt()}
		observer.streams[streamID.String()] = stream
	}
	beforeContent, beforeReasoning := streamProjection(stream.accumulator.Response())
	if err := stream.accumulator.Add(decoded.Response()); err != nil {
		observer.mu.Unlock()
		return
	}
	afterContent, afterReasoning := streamProjection(stream.accumulator.Response())
	updates := appendedModelDeltas(effectID, delta.EffectSequence(), delta.EmittedAt(), beforeContent, afterContent, ModelDeltaContent)
	updates = append(updates, appendedModelDeltas(effectID, delta.EffectSequence(), delta.EmittedAt(), beforeReasoning, afterReasoning, ModelDeltaReasoning)...)
	observer.mu.Unlock()
	for index := range updates {
		updates[index].RunID = scope.runID
		updates[index].SegmentID = scope.segmentID
	}
	for _, update := range updates {
		observer.live.OfferModelDelta(update)
	}
}

func (observer *executionObserver) OnToolStarted(_ context.Context, invocation interaction.ToolInvocation) {
	scope, found := observer.scope(invocation.Relation())
	if !found {
		return
	}
	call := invocation.ToolCall()
	arguments := decodeArguments(call.Arguments)
	observer.mu.Lock()
	value := ToolObservation{
		RunID: scope.runID, SegmentID: scope.segmentID,
		ItemID: ToolItemID(scope.runID, call.ID), CallID: call.ID, Name: call.Name,
		Arguments: arguments, SafetyClass: observer.safetyByName[call.Name],
		IntrinsicInput: observer.intrinsicInput[call.Name],
		ModelCallSequence: int(invocation.ModelCallSequence()), ToolCallIndex: int(invocation.ToolCallIndex()),
		StartedAt: time.Now().UTC(),
	}
	observer.tools[observationIdentity{runID: scope.runID, sourceID: call.ID}] = value
	observer.mu.Unlock()
	if observer.live != nil {
		observer.live.OfferToolStarted(cloneToolObservation(value))
	}
}

func (observer *executionObserver) OnToolSettled(_ context.Context, invocation interaction.ToolInvocation, settlement interaction.ToolSettlement) {
	scope, scoped := observer.scope(invocation.Relation())
	if !scoped {
		return
	}
	call := invocation.ToolCall()
	identity := observationIdentity{runID: scope.runID, sourceID: call.ID}
	observer.mu.Lock()
	value, found := observer.tools[identity]
	if !found {
		value = ToolObservation{
			RunID: scope.runID, SegmentID: scope.segmentID,
			ItemID: ToolItemID(scope.runID, call.ID), CallID: call.ID, Name: call.Name,
			Arguments: decodeArguments(call.Arguments), SafetyClass: observer.safetyByName[call.Name],
			IntrinsicInput: observer.intrinsicInput[call.Name],
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
	if plan, found := observer.plans[identity]; found {
		clone := clonePlan(plan)
		value.CommittedPlan = &clone
	}
	observer.tools[identity] = value
	observer.mu.Unlock()
	if observer.live != nil {
		observer.live.OfferToolSettled(cloneToolObservation(value))
	}
}

func (observer *executionObserver) snapshot(runID string) ([]ModelObservation, []ToolObservation, protocol.Usage, int64) {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	models := make([]ModelObservation, 0, len(observer.models))
	usage := protocol.Usage{ByModel: make(map[string]protocol.ModelUsage)}
	contextTokens := int64(0)
	contextSequence := 0
	for _, value := range observer.models {
		if value.RunID != runID {
			continue
		}
		models = append(models, ModelObservation{
			RunID: value.RunID, SegmentID: value.SegmentID,
			EffectID: value.EffectID, Sequence: value.Sequence, OccurredAt: value.OccurredAt, Response: value.Response.Clone(),
		})
		model := value.Response.Model
		if model == "" {
			model = observer.defaultModel
		}
		addUsage(&usage, model, presentUsage(value.Response.Usage).ModelUsage)
		if value.Sequence >= contextSequence && value.Response.Usage.InputTokens > 0 {
			contextSequence = value.Sequence
			contextTokens = value.Response.Usage.InputTokens
		}
	}
	sort.SliceStable(models, func(left, right int) bool { return models[left].Sequence < models[right].Sequence })
	tools := make([]ToolObservation, 0, len(observer.tools))
	for _, value := range observer.tools {
		if value.RunID != runID {
			continue
		}
		value = cloneToolObservation(value)
		if plan, found := observer.plans[observationIdentity{runID: value.RunID, sourceID: value.CallID}]; found {
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
	return models, tools, usage, contextTokens
}

func (observer *executionObserver) scope(relation agent.ProcessRelation) (processBinding, bool) {
	if observer.delegation != nil {
		return observer.delegation.binding(relation)
	}
	if relation.IsRoot() {
		return processBinding{runID: observer.runID, segmentID: observer.segmentID, rootRunID: observer.runID}, true
	}
	return processBinding{}, false
}

func (observer *executionObserver) scopeProcess(processID agent.ProcessID) (processBinding, bool) {
	if observer.delegation != nil {
		return observer.delegation.bindingProcess(processID)
	}
	return processBinding{runID: observer.runID, segmentID: observer.segmentID, rootRunID: observer.runID}, true
}

func (identity observationIdentity) String() string { return identity.runID + "\x00" + identity.sourceID }

func cloneToolObservation(value ToolObservation) ToolObservation {
	value.Arguments = cloneObject(value.Arguments)
	if value.CommittedPlan != nil {
		clone := clonePlan(*value.CommittedPlan)
		value.CommittedPlan = &clone
	}
	return value
}

func streamProjection(response *chat.Response) (content, reasoning []string) {
	choice := response.First()
	if choice == nil || choice.Message == nil {
		return nil, nil
	}
	for _, part := range choice.Message.Parts {
		switch part.Kind {
		case chat.PartText:
			if part.Text != "" {
				content = append(content, part.Text)
			}
		case chat.PartMedia:
			// Reserve the terminal ContentBlock index. Media is delivered by the
			// authoritative completion because ItemDelta has no media variant.
			if part.Media != nil && part.Media.Source.Kind == "bytes" && strings.HasPrefix(part.Media.MIME, "image/") {
				content = append(content, "")
			}
		case chat.PartReasoning:
			reasoning = append(reasoning, part.Text)
		}
	}
	return content, reasoning
}

func appendedModelDeltas(
	effectID string,
	effectSequence uint64,
	occurredAt time.Time,
	before, after []string,
	kind ModelDeltaKind,
) []ModelDelta {
	updates := make([]ModelDelta, 0, len(after))
	for index, value := range after {
		previous := ""
		if index < len(before) {
			previous = before[index]
		}
		if value == previous || !strings.HasPrefix(value, previous) {
			continue
		}
		appendix := strings.TrimPrefix(value, previous)
		if appendix == "" {
			continue
		}
		updates = append(updates, ModelDelta{
			EffectID: effectID, EffectSequence: effectSequence, OccurredAt: occurredAt,
			Kind: kind, Index: index, Text: appendix,
		})
	}
	return updates
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
	addObservedCost(&total.CostUSD, value.CostUSD)
	byModel := total.ByModel[model]
	byModel.InputTokens += value.InputTokens
	byModel.OutputTokens += value.OutputTokens
	byModel.CacheReadTokens += value.CacheReadTokens
	byModel.CacheWriteTokens += value.CacheWriteTokens
	byModel.ReasoningTokens += value.ReasoningTokens
	addObservedCost(&byModel.CostUSD, value.CostUSD)
	total.ByModel[model] = byModel
}

func addObservedCost(total **float64, value *float64) {
	if value == nil {
		return
	}
	merged := *value
	if *total != nil {
		merged += **total
	}
	*total = &merged
}

func usageForModel(model string, value chat.Usage) protocol.Usage {
	usage := presentUsage(value)
	if model != "" {
		usage.ByModel = map[string]protocol.ModelUsage{model: usage.ModelUsage}
	}
	return usage
}

var _ interaction.ExecutionObserver = (*executionObserver)(nil)
var _ agent.DeltaListener = (*executionObserver)(nil)
var _ ToolFactSink = (*executionObserver)(nil)
