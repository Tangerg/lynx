package interaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/samber/lo"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/Tangerg/scope/core/tool"
)

// DispatcherConfig binds external capabilities for one Deployment.
type DispatcherConfig struct {
	// Client provides complete and optional streaming model calls.
	Client *chatclient.Client

	// Tools is the frozen ordinary model-visible and executable Tool manifest.
	// Managed Delegate definitions come from the bound Definition.
	Tools []tool.Tool

	// DeferredTools is the frozen ordinary executable Tool set omitted from the
	// initial model manifest. An executing Tool may make exact names visible on
	// later model calls through AdvertiseTools; authority never changes.
	DeferredTools []tool.Tool

	// MaxConcurrentToolCalls bounds calls that explicitly declare safe overlap.
	// Zero preserves serial execution; negative values are invalid. Undeclared
	// calls and calls with the same non-empty concurrency key remain serial.
	MaxConcurrentToolCalls int

	// StreamModelResponses selects Client.Stream and publishes each validated
	// response chunk as a best-effort ModelResponseDelta. False uses Client.Call.
	StreamModelResponses bool

	// Observer receives exact settled Interaction facts. It is intentionally
	// separate from Engine Events/Deltas: those describe execution mechanics,
	// while this boundary exposes typed model and Tool semantics.
	Observer ExecutionObserver

	// ModelContextReducer optionally replaces only the provider-neutral message
	// context at the last safe boundary before each model call. The Dispatcher
	// installs the effective messages back into Interaction recovery state when
	// the call settles, so later calls and checkpoints cannot regrow a reduced
	// context from the pre-reduction Effect payload.
	ModelContextReducer ModelContextReducer
}

type boundTool struct {
	executable tool.Tool
	definition chat.ToolDefinition
	deferred   bool
	direct     bool
	concurrent ConcurrentTool
}

// Dispatcher executes model calls and ordinary Tool segments emitted by an
// Interaction Execution. It is immutable after construction and may serve
// Processes concurrently when the supplied Client and Tools support concurrent use.
type Dispatcher struct {
	client             *chatclient.Client
	tools              map[string]boundTool
	delegates          map[string]struct{}
	initialDefinitions []chat.ToolDefinition
	deferredToolNames  map[string]struct{}
	stream             bool
	maxParallel        int
	observer           ExecutionObserver
	contextReducer     ModelContextReducer
}

func NewDispatcher(definition *Definition, config DispatcherConfig) (*Dispatcher, error) {
	if !definition.valid() || config.Client == nil {
		return nil, fmt.Errorf("%w: Definition and Client are required", ErrInvalidDispatcherConfig)
	}
	if config.MaxConcurrentToolCalls < 0 {
		return nil, fmt.Errorf("%w: MaxConcurrentToolCalls must not be negative", ErrInvalidDispatcherConfig)
	}
	if config.ModelContextReducer != nil && lo.IsNil(config.ModelContextReducer) {
		return nil, fmt.Errorf("%w: ModelContextReducer is typed nil", ErrInvalidDispatcherConfig)
	}
	maxParallel := max(1, config.MaxConcurrentToolCalls)
	dispatcher := &Dispatcher{
		client:    config.Client,
		tools:     make(map[string]boundTool, len(config.Tools)+len(config.DeferredTools)),
		delegates: make(map[string]struct{}, len(definition.delegates)),
		initialDefinitions: make(
			[]chat.ToolDefinition, 0, len(config.Tools)+len(definition.delegates),
		),
		deferredToolNames: make(map[string]struct{}, len(config.DeferredTools)),
		stream:            config.StreamModelResponses,
		maxParallel:       maxParallel,
		observer:          config.Observer,
		contextReducer:    config.ModelContextReducer,
	}
	for index, executable := range config.Tools {
		if err := dispatcher.bindTool(executable, false); err != nil {
			return nil, fmt.Errorf("%w: Tools[%d]: %w", ErrInvalidDispatcherConfig, index, err)
		}
	}
	for index, executable := range config.DeferredTools {
		if err := dispatcher.bindTool(executable, true); err != nil {
			return nil, fmt.Errorf("%w: DeferredTools[%d]: %w", ErrInvalidDispatcherConfig, index, err)
		}
	}
	for _, delegate := range definition.delegates {
		name := delegate.definition.Name
		if _, duplicate := dispatcher.tools[name]; duplicate {
			return nil, fmt.Errorf("%w: Delegate name %q collides with a Tool", ErrInvalidDispatcherConfig, name)
		}
		dispatcher.delegates[name] = struct{}{}
		dispatcher.initialDefinitions = append(
			dispatcher.initialDefinitions, delegate.definition.Clone(),
		)
	}
	return dispatcher, nil
}

func (d *Dispatcher) bindTool(executable tool.Tool, deferred bool) error {
	if lo.IsNil(executable) {
		return errors.New("tool is nil")
	}
	definition, err := toolDefinition(executable)
	if err != nil {
		return err
	}
	if _, duplicate := d.tools[definition.Name]; duplicate {
		return fmt.Errorf("duplicate tool name %q", definition.Name)
	}
	direct, err := directResultCapability(executable)
	if err != nil {
		return err
	}
	concurrent, err := concurrentToolCapability(executable)
	if err != nil {
		return err
	}
	definition = definition.Clone()
	d.tools[definition.Name] = boundTool{
		executable: executable, definition: definition.Clone(), deferred: deferred,
		direct: direct, concurrent: concurrent,
	}
	if deferred {
		d.deferredToolNames[definition.Name] = struct{}{}
	} else {
		d.initialDefinitions = append(d.initialDefinitions, definition.Clone())
	}
	return nil
}

// Dispatch executes one validated Interaction protocol operation and returns a
// definite owner-defined Signal payload. An error means the external outcome
// is not provable; Engine therefore records an unknown settlement instead of
// retrying the operation.
func (d *Dispatcher) Dispatch(
	ctx context.Context,
	request agent.EffectRequest,
	emit agent.DeltaEmitter,
) (agent.Settlement, error) {
	if d == nil || d.client == nil {
		return agent.Settlement{}, ErrInvalidDispatcherConfig
	}
	envelope, err := decodeEffect(request.Effect().Payload())
	if err != nil {
		return agent.Settlement{}, err
	}
	switch envelope.Operation {
	case operationModelCall:
		return d.dispatchModel(ctx, request, envelope.ModelCall, emit)
	case operationToolBatch:
		return d.dispatchToolBatch(ctx, request, envelope.ToolBatch)
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

func (d *Dispatcher) dispatchModel(
	ctx context.Context,
	request agent.EffectRequest,
	call *modelCall,
	emit agent.DeltaEmitter,
) (agent.Settlement, error) {
	modelRequest := call.Request.Clone()
	definitions, err := d.modelDefinitions(call.AdvertisedToolNames)
	if err != nil {
		return agent.Settlement{}, err
	}
	modelRequest.Tools = definitions
	if validateErr := modelRequest.Validate(); validateErr != nil {
		return agent.Settlement{}, fmt.Errorf("interaction: prepare model request: %w", validateErr)
	}
	invocation := modelInvocationFromRequest(
		request,
		call.ModelCallSequence,
		call.AppliedSteerSignalIDs,
	)
	ctx = withModelInvocation(ctx, invocation)
	if d.contextReducer != nil {
		effectiveMessages, reduceErr := d.contextReducer.ReduceModelContext(
			ctx, invocation, modelRequest.Clone(),
		)
		if reduceErr != nil {
			return modelHostFailureSettlement(
				request.ID(),
				fmt.Errorf("interaction: reduce model context: %w", reduceErr),
			)
		}
		modelRequest.Messages = cloneMessages(effectiveMessages)
		if validateErr := modelRequest.Validate(); validateErr != nil {
			return modelHostFailureSettlement(
				request.ID(),
				fmt.Errorf("interaction: reduced model context: %w", validateErr),
			)
		}
	}
	response, err := d.callModel(ctx, modelRequest, emit)
	if err != nil {
		if errors.Is(err, ErrHostFailure) {
			return modelHostFailureSettlement(request.ID(), err)
		}
		return modelFailureSettlement(request.ID(), err)
	}
	if response == nil {
		return modelFailureSettlement(request.ID(), errors.New("model returned a nil response"))
	}
	if validateErr := response.Validate(); validateErr != nil {
		return modelFailureSettlement(request.ID(), fmt.Errorf("invalid model response: %w", validateErr))
	}
	d.observeModel(ctx, invocation, response)
	payload, err := encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationModelCall,
		ModelResult: &modelCallResult{
			Response:          response.Clone(),
			EffectiveMessages: cloneMessages(modelRequest.Messages),
		},
	})
	if err != nil {
		return agent.Settlement{}, err
	}
	return agent.NewSettlement(request.ID(), agent.SettlementStatusSucceeded, payload)
}

func (d *Dispatcher) modelDefinitions(advertisedToolNames []string) ([]chat.ToolDefinition, error) {
	if err := validateAdvertisedToolNames(advertisedToolNames); err != nil {
		return nil, fmt.Errorf("interaction: advertised Tools: %w", err)
	}
	definitions := cloneDefinitions(d.initialDefinitions)
	for _, name := range advertisedToolNames {
		hosted, found := d.tools[name]
		if !found || !hosted.deferred {
			return nil, fmt.Errorf("interaction: tool %q is not a bound deferred Tool", name)
		}
		definitions = append(definitions, hosted.definition.Clone())
	}
	return definitions, nil
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

func modelHostFailureSettlement(effectID agent.EffectID, cause error) (agent.Settlement, error) {
	payload, err := encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationModelCall,
		ModelResult:   &modelCallResult{HostError: boundedDiagnostic(cause.Error())},
	})
	if err != nil {
		return agent.Settlement{}, err
	}
	return agent.NewSettlement(effectID, agent.SettlementStatusFailed, payload)
}

func (d *Dispatcher) dispatchToolBatch(
	ctx context.Context,
	request agent.EffectRequest,
	batch *toolBatchCall,
) (agent.Settlement, error) {
	dispatch, err := newToolBatchDispatch(ctx, d, request, batch)
	if err != nil {
		return agent.Settlement{}, err
	}
	return dispatch.run()
}

type toolBatchDispatch struct {
	dispatcher          *Dispatcher
	ctx                 context.Context
	request             agent.EffectRequest
	batch               *toolBatchCall
	results             []chat.ToolResult
	advertisedToolNames []string
	start               int
	pauseCount          uint32
	continuation        *ToolInputContinuation
	allDirect           bool
}

func newToolBatchDispatch(
	ctx context.Context,
	dispatcher *Dispatcher,
	request agent.EffectRequest,
	batch *toolBatchCall,
) (*toolBatchDispatch, error) {
	for _, call := range batch.Calls {
		if _, delegated := dispatcher.delegates[call.Name]; delegated {
			return nil, fmt.Errorf(
				"interaction: Delegate %q must be executed by the managed Execution boundary",
				call.Name,
			)
		}
	}
	dispatch := &toolBatchDispatch{
		dispatcher: dispatcher,
		ctx:        ctx,
		request:    request,
		batch:      batch,
		results:    make([]chat.ToolResult, 0, len(batch.Calls)),
		allDirect:  dispatcher.allCallsDirect(batch.Calls),
	}
	if batch.Checkpoint != nil {
		checkpoint := batch.Checkpoint
		dispatch.results = append(dispatch.results, checkpoint.CompletedResults...)
		dispatch.advertisedToolNames = append(dispatch.advertisedToolNames, checkpoint.AdvertisedToolNames...)
		dispatch.start = int(checkpoint.NextToolCallIndex)
		dispatch.pauseCount = checkpoint.PauseCount
		if dispatch.pauseCount == ^uint32(0) {
			return nil, errors.New("interaction: tool checkpoint pause count exhausted")
		}
		dispatch.continuation = &ToolInputContinuation{
			state:    bytes.Clone(checkpoint.InputRequest.ContinuationState),
			response: bytes.Clone(batch.InputResponse),
		}
	}
	return dispatch, nil
}

func (t *toolBatchDispatch) run() (agent.Settlement, error) {
	if settlement, paused, err := t.resume(); err != nil || paused {
		if errors.Is(err, ErrHostFailure) {
			return toolHostFailureSettlement(t.request.ID(), err)
		}
		return settlement, err
	}
	if settlement, paused, err := t.dispatchRemaining(); err != nil || paused {
		if errors.Is(err, ErrHostFailure) {
			return toolHostFailureSettlement(t.request.ID(), err)
		}
		return settlement, err
	}
	return t.complete()
}

func toolHostFailureSettlement(effectID agent.EffectID, cause error) (agent.Settlement, error) {
	payload, err := encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationToolBatch,
		ToolResult:    &toolBatchResult{HostError: boundedDiagnostic(cause.Error())},
	})
	if err != nil {
		return agent.Settlement{}, err
	}
	return agent.NewSettlement(effectID, agent.SettlementStatusFailed, payload)
}

func (t *toolBatchDispatch) resume() (agent.Settlement, bool, error) {
	if t.continuation == nil {
		return agent.Settlement{}, false, nil
	}
	callContext := withToolInputContinuation(t.ctx, *t.continuation)
	result, newlyAdvertised, required, err := t.dispatcher.callTool(
		callContext,
		t.request,
		t.batch.ModelCallSequence,
		t.batch.FirstToolCallIndex+uint32(t.start),
		t.batch.Calls[t.start],
	)
	if err != nil {
		return agent.Settlement{}, false, fmt.Errorf(
			"interaction: tool call %q: %w", t.batch.Calls[t.start].ID, err,
		)
	}
	if required != nil {
		settlement, pauseErr := t.pause(uint32(t.start), *required)
		return settlement, true, pauseErr
	}
	t.results = append(t.results, result)
	t.advertisedToolNames, err = mergeAdvertisedToolNames(
		t.advertisedToolNames, newlyAdvertised,
	)
	if err != nil {
		return agent.Settlement{}, false, err
	}
	t.start++
	return agent.Settlement{}, false, nil
}

func (t *toolBatchDispatch) dispatchRemaining() (agent.Settlement, bool, error) {
	plans, err := t.dispatcher.planToolCalls(t.batch.Calls[t.start:])
	if err != nil {
		return agent.Settlement{}, false, err
	}
	for offset := 0; offset < len(plans); {
		end := offset + 1
		if t.dispatcher.maxParallel > 1 {
			end = concurrentBatchEnd(plans, offset)
		}
		outcomes := t.dispatcher.callToolBatch(
			t.ctx,
			t.request,
			t.batch.ModelCallSequence,
			t.batch.FirstToolCallIndex+uint32(t.start+offset),
			t.batch.Calls[t.start+offset:t.start+end],
		)
		for index, outcome := range outcomes {
			call := t.batch.Calls[t.start+offset+index]
			if outcome.err != nil {
				return agent.Settlement{}, false, fmt.Errorf("interaction: tool call %q: %w", call.ID, outcome.err)
			}
			if outcome.required != nil {
				if len(outcomes) != 1 {
					return agent.Settlement{}, false, fmt.Errorf(
						"interaction: concurrently executed tool call %q requested external input",
						call.ID,
					)
				}
				settlement, pauseErr := t.pause(uint32(t.start+offset), *outcome.required)
				return settlement, true, pauseErr
			}
		}
		for _, outcome := range outcomes {
			t.results = append(t.results, outcome.result)
			t.advertisedToolNames, err = mergeAdvertisedToolNames(
				t.advertisedToolNames,
				outcome.advertisedToolNames,
			)
			if err != nil {
				return agent.Settlement{}, false, err
			}
		}
		offset = end
	}
	return agent.Settlement{}, false, nil
}

func (t *toolBatchDispatch) pause(index uint32, request ToolInputRequest) (agent.Settlement, error) {
	if t.pauseCount == math.MaxUint32 {
		return agent.Settlement{}, errors.New("interaction: Tool input pause count is exhausted")
	}
	checkpoint := &toolCheckpoint{
		CompletedResults:    slices.Clone(t.results),
		AdvertisedToolNames: slices.Clone(t.advertisedToolNames),
		NextToolCallIndex:   index,
		PauseCount:          t.pauseCount + 1,
		InputRequest:        wireInputRequest(request),
	}
	return toolCheckpointSettlement(t.request.ID(), checkpoint)
}

func (t *toolBatchDispatch) complete() (agent.Settlement, error) {
	payload, err := encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationToolBatch,
		ToolResult: &toolBatchResult{
			Results: t.results, Direct: t.allDirect,
			AdvertisedToolNames: t.advertisedToolNames,
		},
	})
	if err != nil {
		return agent.Settlement{}, err
	}
	return agent.NewSettlement(t.request.ID(), agent.SettlementStatusSucceeded, payload)
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

func (d *Dispatcher) callTool(
	ctx context.Context,
	request agent.EffectRequest,
	modelCallSequence uint32,
	toolCallIndex uint32,
	call chat.ToolCall,
) (
	result chat.ToolResult,
	advertisedToolNames []string,
	required *ToolInputRequest,
	err error,
) {
	invocation := toolInvocationFromRequest(
		request, modelCallSequence, toolCallIndex, call,
	)
	d.observeToolStarted(ctx, invocation)
	defer func() {
		settlement := ToolSettlement{}
		switch {
		case required != nil:
			settlement.InputRequired = true
		case result.ID != "":
			settlement.Result = &result
		case err != nil:
			settlement.Failure = boundedDiagnostic(err.Error())
			settlement.Unknown = errors.Is(err, ErrHostFailure)
		}
		d.observeToolSettled(ctx, invocation, settlement)
	}()
	hosted, found := d.tools[call.Name]
	if !found {
		return chat.ToolResult{
			ID: call.ID, Name: call.Name,
			Result: fmt.Sprintf("error: tool %q is not available", call.Name), IsError: true,
		}, nil, nil, nil
	}
	advertiser := newToolAdvertiser(d.deferredToolNames)
	ctx = withToolInvocation(ctx, invocation)
	ctx = withToolAdvertiser(ctx, advertiser)
	defer func() {
		if recovered := recover(); recovered != nil {
			result = chat.ToolResult{}
			advertisedToolNames = nil
			required = nil
			err = fmt.Errorf("tool panicked: %v", recovered)
		}
	}()
	output, err := hosted.executable.Call(ctx, call.Arguments)
	if err != nil {
		if errors.Is(err, ErrHostFailure) {
			return chat.ToolResult{}, nil, nil, err
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return chat.ToolResult{}, nil, nil, err
		}
		if inputRequired, ok := errors.AsType[*ToolInputRequiredError](err); ok {
			request, valid := inputRequired.inputRequest()
			if !valid {
				return chat.ToolResult{}, nil, nil, ErrInvalidToolInputRequest
			}
			return chat.ToolResult{}, nil, &request, nil
		}
		return chat.ToolResult{
			ID: call.ID, Name: call.Name,
			Result: fmt.Sprintf("error: tool %q failed: %s", call.Name, boundedDiagnostic(err.Error())), IsError: true,
		}, nil, nil, nil
	}
	return chat.ToolResult{ID: call.ID, Name: call.Name, Result: output},
		advertiser.advertisedNames(), nil, nil
}

func (d *Dispatcher) allCallsDirect(calls []chat.ToolCall) bool {
	if len(calls) == 0 {
		return false
	}
	for _, call := range calls {
		hosted, found := d.tools[call.Name]
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

var _ agent.Dispatcher = (*Dispatcher)(nil)
