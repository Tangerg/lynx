package interaction

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

type boundTool struct {
	executable tool.Tool
}

// Dispatcher executes model calls and tool batches emitted by an Interaction
// Execution. It is immutable after construction and may serve Processes
// concurrently when the supplied Client and Tools support concurrent use.
type Dispatcher struct {
	client      *chatclient.Client
	tools       map[string]boundTool
	definitions []chat.ToolDefinition
}

// NewDispatcher freezes the model-visible tool manifest and binds executable
// capabilities. Tool names must be unique, definitions must be valid, and
// typed-nil tools are rejected at construction rather than during a Process.
func NewDispatcher(config DispatcherConfig) (*Dispatcher, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("%w: Client is required", ErrInvalidDispatcherConfig)
	}
	dispatcher := &Dispatcher{
		client:      config.Client,
		tools:       make(map[string]boundTool, len(config.Tools)),
		definitions: make([]chat.ToolDefinition, 0, len(config.Tools)),
	}
	for index, executable := range config.Tools {
		if nilValue(executable) {
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
		dispatcher.tools[definition.Name] = boundTool{executable: executable}
		dispatcher.definitions = append(dispatcher.definitions, definition.Clone())
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
	_ agent.DeltaEmitter,
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
		return dispatcher.dispatchModel(ctx, request.ID(), envelope.ModelCall)
	case operationToolBatch:
		return dispatcher.dispatchToolBatch(ctx, request.ID(), envelope.ToolBatch)
	default:
		return agent.Settlement{}, errors.New("interaction: unsupported dispatcher operation")
	}
}

// ReplayPolicy is deliberately conservative. Model calls may incur cost and
// produce a different answer, while Tools may have irreversible side effects;
// neither is replayed after a crash without an explicit Process resolution.
func (*Dispatcher) ReplayPolicy(agent.Effect) agent.ReplayPolicy {
	return agent.ReplayPolicyNever
}

func (dispatcher *Dispatcher) dispatchModel(
	ctx context.Context,
	effectID agent.EffectID,
	call *modelCall,
) (agent.Settlement, error) {
	modelRequest := call.Request.Clone()
	modelRequest.Tools = cloneDefinitions(dispatcher.definitions)
	if err := modelRequest.Validate(); err != nil {
		return agent.Settlement{}, fmt.Errorf("interaction: prepare model request: %w", err)
	}
	response, err := dispatcher.client.Call(ctx, modelRequest)
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
	results := make([]chat.ToolResult, len(batch.Calls))
	for index, call := range batch.Calls {
		result, err := dispatcher.callTool(ctx, call)
		if err != nil {
			return agent.Settlement{}, fmt.Errorf("interaction: tool call %q: %w", call.ID, err)
		}
		results[index] = result
	}
	payload, err := encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationToolBatch,
		ToolResult:    &toolBatchResult{Results: results},
	})
	if err != nil {
		return agent.Settlement{}, err
	}
	return agent.NewSettlement(effectID, agent.SettlementStatusSucceeded, payload)
}

func (dispatcher *Dispatcher) callTool(ctx context.Context, call chat.ToolCall) (result chat.ToolResult, err error) {
	hosted, found := dispatcher.tools[call.Name]
	if !found {
		return chat.ToolResult{
			ID: call.ID, Name: call.Name,
			Result: fmt.Sprintf("error: tool %q is not available", call.Name), IsError: true,
		}, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			result = chat.ToolResult{}
			err = fmt.Errorf("tool panicked: %v", recovered)
		}
	}()
	output, err := hosted.executable.Call(ctx, call.Arguments)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return chat.ToolResult{}, err
		}
		return chat.ToolResult{
			ID: call.ID, Name: call.Name,
			Result: fmt.Sprintf("error: tool %q failed: %s", call.Name, boundedDiagnostic(err.Error())), IsError: true,
		}, nil
	}
	return chat.ToolResult{ID: call.ID, Name: call.Name, Result: output}, nil
}

func toolDefinition(executable tool.Tool) (definition chat.ToolDefinition, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			definition = chat.ToolDefinition{}
			err = fmt.Errorf("Definition panicked: %v", recovered)
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

func nilValue(value any) bool {
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
