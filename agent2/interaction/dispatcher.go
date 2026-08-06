package interaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

// DispatcherConfig binds external capabilities for one Deployment.
type DispatcherConfig struct {
	// Client provides complete and optional streaming model calls.
	Client *chatclient.Client

	// Tools is the frozen ordinary model-visible and executable Tool manifest.
	// Managed Delegate definitions come from the bound Definition.
	Tools []tool.Tool

	// MaxConcurrentToolCalls bounds calls that explicitly declare safe overlap.
	// Zero preserves serial execution; negative values are invalid. Undeclared
	// calls and calls with the same non-empty concurrency key remain serial.
	MaxConcurrentToolCalls int

	// StreamModelResponses selects Client.Stream and publishes each validated
	// response chunk as a best-effort ModelResponseDelta. False uses Client.Call.
	StreamModelResponses bool
}

type boundTool struct {
	executable tool.Tool
	direct     bool
	concurrent ConcurrentTool
}

// Dispatcher executes model calls and ordinary Tool segments emitted by an
// Interaction Execution. It is immutable after construction and may serve
// Processes concurrently when the supplied Client and Tools support concurrent use.
type Dispatcher struct {
	client      *chatclient.Client
	tools       map[string]boundTool
	delegates   map[string]struct{}
	definitions []chat.ToolDefinition
	stream      bool
	maxParallel int
}

// NewDispatcher binds one exact Definition's Delegate manifest alongside its
// executable Tools. All model-visible names must be unique; malformed or
// typed-nil capabilities are rejected before any Process starts.
func NewDispatcher(definition *Definition, config DispatcherConfig) (*Dispatcher, error) {
	if !definition.valid() || config.Client == nil {
		return nil, fmt.Errorf("%w: Definition and Client are required", ErrInvalidDispatcherConfig)
	}
	if config.MaxConcurrentToolCalls < 0 {
		return nil, fmt.Errorf("%w: MaxConcurrentToolCalls must not be negative", ErrInvalidDispatcherConfig)
	}
	maxParallel := max(1, config.MaxConcurrentToolCalls)
	dispatcher := &Dispatcher{
		client:      config.Client,
		tools:       make(map[string]boundTool, len(config.Tools)),
		delegates:   make(map[string]struct{}, len(definition.delegates)),
		definitions: make([]chat.ToolDefinition, 0, len(config.Tools)+len(definition.delegates)),
		stream:      config.StreamModelResponses,
		maxParallel: maxParallel,
	}
	for index, executable := range config.Tools {
		if isNilTool(executable) {
			return nil, fmt.Errorf("%w: Tools[%d] is nil", ErrInvalidDispatcherConfig, index)
		}
		definition, err := toolDefinition(executable)
		if err != nil {
			return nil, fmt.Errorf("%w: Tools[%d]: %w", ErrInvalidDispatcherConfig, index, err)
		}
		if _, duplicate := dispatcher.tools[definition.Name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate tool name %q", ErrInvalidDispatcherConfig, definition.Name)
		}
		definition = definition.Clone()
		direct, err := directResultCapability(executable)
		if err != nil {
			return nil, fmt.Errorf("%w: Tools[%d]: %w", ErrInvalidDispatcherConfig, index, err)
		}
		concurrent, err := concurrentToolCapability(executable)
		if err != nil {
			return nil, fmt.Errorf("%w: Tools[%d]: %w", ErrInvalidDispatcherConfig, index, err)
		}
		dispatcher.tools[definition.Name] = boundTool{
			executable: executable,
			direct:     direct,
			concurrent: concurrent,
		}
		dispatcher.definitions = append(dispatcher.definitions, definition.Clone())
	}
	for _, delegate := range definition.delegates {
		name := delegate.definition.Name
		if _, duplicate := dispatcher.tools[name]; duplicate {
			return nil, fmt.Errorf("%w: Delegate name %q collides with a Tool", ErrInvalidDispatcherConfig, name)
		}
		dispatcher.delegates[name] = struct{}{}
		dispatcher.definitions = append(dispatcher.definitions, delegate.definition.Clone())
	}
	return dispatcher, nil
}

// Dispatch executes one validated Interaction protocol operation and returns a
// definite owner-defined Signal payload. An error means the external outcome
// is not provable; Engine therefore records an unknown settlement instead of
// retrying the operation.
func (dispatcher *Dispatcher) Dispatch(
	ctx context.Context,
	request agent.EffectRequest,
	emit agent.DeltaEmitter,
) (agent.Settlement, error) {
	if dispatcher == nil || dispatcher.client == nil {
		return agent.Settlement{}, ErrInvalidDispatcherConfig
	}
	envelope, err := decodeEffect(request.Effect().Payload())
	if err != nil {
		return agent.Settlement{}, err
	}
	switch envelope.Operation {
	case operationModelCall:
		return dispatcher.dispatchModel(ctx, request.ID(), envelope.ModelCall, emit)
	case operationToolBatch:
		return dispatcher.dispatchToolBatch(ctx, request.ID(), envelope.ToolBatch)
	default:
		return agent.Settlement{}, errors.New("interaction: unsupported dispatcher operation")
	}
}

// ReplayPolicy is deliberately conservative. Model calls may incur cost and
// produce a different answer, while Tools may have irreversible side effects;
// neither is replayed after a crash without an explicit Process resolution.
func (*Dispatcher) ReplayPolicy(effect agent.Effect) agent.ReplayPolicy {
	return agent.ReplayPolicyNever
}

func (dispatcher *Dispatcher) dispatchModel(
	ctx context.Context,
	effectID agent.EffectID,
	call *modelCall,
	emit agent.DeltaEmitter,
) (agent.Settlement, error) {
	modelRequest := call.Request.Clone()
	modelRequest.Tools = cloneDefinitions(dispatcher.definitions)
	if err := modelRequest.Validate(); err != nil {
		return agent.Settlement{}, fmt.Errorf("interaction: prepare model request: %w", err)
	}
	response, err := dispatcher.callModel(ctx, modelRequest, emit)
	if err != nil {
		return modelFailureSettlement(effectID, err)
	}
	if response == nil {
		return modelFailureSettlement(effectID, errors.New("model returned a nil response"))
	}
	if err := response.Validate(); err != nil {
		return modelFailureSettlement(effectID, fmt.Errorf("invalid model response: %w", err))
	}
	payload, err := encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationModelCall,
		ModelResult:   &modelCallResult{Response: response.Clone()},
	})
	if err != nil {
		return agent.Settlement{}, err
	}
	return agent.NewSettlement(effectID, agent.SettlementStatusSucceeded, payload)
}

func modelFailureSettlement(effectID agent.EffectID, cause error) (agent.Settlement, error) {
	payload, err := encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationModelCall,
		ModelResult:   &modelCallResult{Error: boundedDiagnostic(cause.Error())},
	})
	if err != nil {
		return agent.Settlement{}, err
	}
	return agent.NewSettlement(effectID, agent.SettlementStatusFailed, payload)
}

func (dispatcher *Dispatcher) dispatchToolBatch(
	ctx context.Context,
	effectID agent.EffectID,
	batch *toolBatchCall,
) (agent.Settlement, error) {
	for _, call := range batch.Calls {
		if _, delegated := dispatcher.delegates[call.Name]; delegated {
			return agent.Settlement{}, fmt.Errorf(
				"interaction: Delegate %q must be executed by the managed Execution boundary",
				call.Name,
			)
		}
	}
	results := make([]chat.ToolResult, 0, len(batch.Calls))
	start := 0
	pauses := uint32(0)
	var continuation *ToolInputContinuation
	if batch.Checkpoint != nil {
		checkpoint := batch.Checkpoint
		results = append(results, checkpoint.Completed...)
		start = int(checkpoint.Next)
		pauses = checkpoint.Pauses
		if pauses == ^uint32(0) {
			return agent.Settlement{}, errors.New("interaction: tool checkpoint pause count exhausted")
		}
		continuation = &ToolInputContinuation{
			state:    append(json.RawMessage(nil), checkpoint.Input.ContinuationState...),
			response: append(json.RawMessage(nil), batch.InputResponse...),
		}
	}
	allDirect := dispatcher.allCallsDirect(batch.Calls)
	if continuation != nil {
		callContext := ctx
		callContext = withToolInputContinuation(callContext, *continuation)
		result, required, err := dispatcher.callTool(callContext, batch.Calls[start])
		if err != nil {
			return agent.Settlement{}, fmt.Errorf("interaction: tool call %q: %w", batch.Calls[start].ID, err)
		}
		if required != nil {
			checkpoint := &toolCheckpoint{
				Completed: append([]chat.ToolResult(nil), results...),
				Next:      uint32(start),
				Pauses:    pauses + 1,
				Input:     wireInputRequest(*required),
			}
			return toolCheckpointSettlement(effectID, checkpoint)
		}
		results = append(results, result)
		start++
	}

	plans, err := dispatcher.planToolCalls(batch.Calls[start:])
	if err != nil {
		return agent.Settlement{}, err
	}
	for offset := 0; offset < len(plans); {
		end := offset + 1
		if dispatcher.maxParallel > 1 {
			end = concurrentBatchEnd(plans, offset)
		}
		outcomes := dispatcher.callToolBatch(ctx, batch.Calls[start+offset:start+end])
		for index, outcome := range outcomes {
			call := batch.Calls[start+offset+index]
			if outcome.err != nil {
				return agent.Settlement{}, fmt.Errorf("interaction: tool call %q: %w", call.ID, outcome.err)
			}
			if outcome.required != nil {
				if len(outcomes) != 1 {
					return agent.Settlement{}, fmt.Errorf(
						"interaction: concurrently executed tool call %q requested external input",
						call.ID,
					)
				}
				checkpoint := &toolCheckpoint{
					Completed: append([]chat.ToolResult(nil), results...),
					Next:      uint32(start + offset),
					Pauses:    pauses + 1,
					Input:     wireInputRequest(*outcome.required),
				}
				return toolCheckpointSettlement(effectID, checkpoint)
			}
		}
		for _, outcome := range outcomes {
			results = append(results, outcome.result)
		}
		offset = end
	}
	payload, err := encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationToolBatch,
		ToolResult:    &toolBatchResult{Results: results, Direct: allDirect},
	})
	if err != nil {
		return agent.Settlement{}, err
	}
	return agent.NewSettlement(effectID, agent.SettlementStatusSucceeded, payload)
}

func toolCheckpointSettlement(
	effectID agent.EffectID,
	checkpoint *toolCheckpoint,
) (agent.Settlement, error) {
	payload, err := encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationToolBatch,
		ToolResult:    &toolBatchResult{Checkpoint: checkpoint},
	})
	if err != nil {
		return agent.Settlement{}, err
	}
	return agent.NewSettlement(effectID, agent.SettlementStatusSucceeded, payload)
}

func (dispatcher *Dispatcher) callTool(
	ctx context.Context,
	call chat.ToolCall,
) (result chat.ToolResult, required *ToolInputRequest, err error) {
	hosted, found := dispatcher.tools[call.Name]
	if !found {
		return chat.ToolResult{
			ID: call.ID, Name: call.Name,
			Result: fmt.Sprintf("error: tool %q is not available", call.Name), IsError: true,
		}, nil, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			result = chat.ToolResult{}
			required = nil
			err = fmt.Errorf("tool panicked: %v", recovered)
		}
	}()
	output, err := hosted.executable.Call(ctx, call.Arguments)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return chat.ToolResult{}, nil, err
		}
		var inputRequired *ToolInputRequiredError
		if errors.As(err, &inputRequired) {
			request, valid := inputRequired.inputRequest()
			if !valid {
				return chat.ToolResult{}, nil, ErrInvalidToolInputRequest
			}
			return chat.ToolResult{}, &request, nil
		}
		return chat.ToolResult{
			ID: call.ID, Name: call.Name,
			Result: fmt.Sprintf("error: tool %q failed: %s", call.Name, boundedDiagnostic(err.Error())), IsError: true,
		}, nil, nil
	}
	return chat.ToolResult{ID: call.ID, Name: call.Name, Result: output}, nil, nil
}

func (dispatcher *Dispatcher) allCallsDirect(calls []chat.ToolCall) bool {
	if len(calls) == 0 {
		return false
	}
	for _, call := range calls {
		hosted, found := dispatcher.tools[call.Name]
		if !found || !hosted.direct {
			return false
		}
	}
	return true
}

func directResultCapability(executable tool.Tool) (direct bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			direct = false
			err = fmt.Errorf("direct-result capability panicked: %v", recovered)
		}
	}()
	capability, found, err := tool.Capability[DirectResultTool](executable)
	if err != nil {
		return false, fmt.Errorf("direct-result capability: %w", err)
	}
	if !found {
		return false, nil
	}
	return capability.ReturnsDirectResult(), nil
}

func concurrentToolCapability(executable tool.Tool) (declared ConcurrentTool, err error) {
	capability, found, err := tool.Capability[ConcurrentTool](executable)
	if err != nil {
		return nil, fmt.Errorf("concurrency capability: %w", err)
	}
	if !found {
		return nil, nil
	}
	return capability, nil
}

func toolDefinition(executable tool.Tool) (definition chat.ToolDefinition, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			definition = chat.ToolDefinition{}
			err = fmt.Errorf("tool definition panicked: %v", recovered)
		}
	}()
	definition = executable.Definition()
	if err := definition.Validate(); err != nil {
		return chat.ToolDefinition{}, err
	}
	return definition, nil
}

func cloneDefinitions(definitions []chat.ToolDefinition) []chat.ToolDefinition {
	cloned := make([]chat.ToolDefinition, len(definitions))
	for index := range definitions {
		cloned[index] = definitions[index].Clone()
	}
	return cloned
}

func isNilTool(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

var _ agent.Dispatcher = (*Dispatcher)(nil)
