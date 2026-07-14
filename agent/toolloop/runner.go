package toolloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

const defaultMaxRounds = 50

var (
	// ErrInvalidRunner reports invalid construction or run input.
	ErrInvalidRunner = errors.New("toolloop: invalid runner")
	// ErrRoundLimit reports that the model kept requesting tools through the
	// configured number of rounds.
	ErrRoundLimit = errors.New("toolloop: round limit reached")
)

// RunnerConfig controls bounded loop policy. Zero MaxRounds selects 50.
// Negative values are invalid. Runner never retries model or tool calls.
type RunnerConfig struct {
	MaxRounds int
}

// Runner drives a synchronous Model through tool calls. Run and Resume are
// lazy event sequences: no model or tool is called until iteration begins.
// Each invocation is independent and Runner is safe for concurrent use when
// its Model and ToolResolver are safe for concurrent use.
type Runner struct {
	model     chat.Model
	maxRounds int
}

// NewRunner validates model and config and returns an immutable Runner.
func NewRunner(model chat.Model, config RunnerConfig) (*Runner, error) {
	if nilModel(model) {
		return nil, fmt.Errorf("%w: model must not be nil", ErrInvalidRunner)
	}
	if config.MaxRounds < 0 {
		return nil, fmt.Errorf("%w: max rounds must not be negative", ErrInvalidRunner)
	}
	maxRounds := config.MaxRounds
	if maxRounds == 0 {
		maxRounds = defaultMaxRounds
	}
	return &Runner{model: model, maxRounds: maxRounds}, nil
}

// Run emits model, tool, and terminal events until the model produces a
// regular response, an all-direct tool round completes, execution pauses, or
// an error occurs. On failure it yields one zero Event with the error.
func (r *Runner) Run(ctx context.Context, invocation *Invocation) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		state, err := r.startState(ctx, invocation)
		if err != nil {
			yield(Event{}, err)
			return
		}
		r.execute(ctx, state, yield)
	}
}

// Resume continues a serialized checkpoint at its pending call. It emits a
// Resume event first, attaches the resume input to that tool's context, and
// never invokes the model or completed tools again before the pending round
// finishes.
func (r *Runner) Resume(ctx context.Context, checkpoint *Checkpoint, resolver ToolResolver, resume Resume) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		state, err := r.resumeState(ctx, checkpoint, resolver, resume)
		if err != nil {
			yield(Event{}, err)
			return
		}
		eventResume := resume
		if !yield(Event{Kind: EventResume, Resume: &eventResume}, nil) {
			return
		}
		state.resume = &resume
		r.execute(ctx, state, yield)
	}
}

type runnerState struct {
	request  *chat.Request
	resolver ToolResolver
	round    int
	response *chat.Response
	calls    []chat.ToolCall
	results  []chat.ToolResult
	nextCall int
	resume   *Resume
}

func (r *Runner) startState(ctx context.Context, invocation *Invocation) (*runnerState, error) {
	if err := r.validateContext(ctx); err != nil {
		return nil, err
	}
	if invocation == nil {
		return nil, fmt.Errorf("%w: invocation must not be nil", ErrInvalidRunner)
	}
	if err := invocation.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRunner, err)
	}
	request, err := snapshot(invocation.Request)
	if err != nil {
		return nil, fmt.Errorf("%w: snapshot request: %w", ErrInvalidRunner, err)
	}
	return &runnerState{request: request, resolver: invocation.Tools}, nil
}

func (r *Runner) resumeState(ctx context.Context, checkpoint *Checkpoint, resolver ToolResolver, resume Resume) (*runnerState, error) {
	if err := r.validateContext(ctx); err != nil {
		return nil, err
	}
	if err := checkpoint.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRunner, err)
	}
	if err := resume.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRunner, err)
	}
	if checkpoint.ID != resume.ID {
		return nil, fmt.Errorf("%w: resume ID %q does not match checkpoint ID %q", ErrInvalidRunner, resume.ID, checkpoint.ID)
	}
	invocation := &Invocation{Request: checkpoint.Request, Tools: resolver}
	if err := invocation.Validate(); err != nil {
		return nil, fmt.Errorf("%w: resumed invocation: %w", ErrInvalidRunner, err)
	}
	copy, err := snapshot(checkpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: snapshot checkpoint: %w", ErrInvalidRunner, err)
	}
	calls, err := responseToolCalls(copy.Response)
	if err != nil {
		return nil, fmt.Errorf("%w: checkpoint calls: %w", ErrInvalidRunner, err)
	}
	return &runnerState{
		request:  copy.Request,
		resolver: resolver,
		round:    copy.Round,
		response: copy.Response,
		calls:    calls,
		results:  slices.Clone(copy.Results),
		nextCall: copy.NextCall,
	}, nil
}

func (r *Runner) validateContext(ctx context.Context) error {
	if r == nil || nilModel(r.model) || r.maxRounds < 1 {
		return fmt.Errorf("%w: uninitialized runner", ErrInvalidRunner)
	}
	if ctx == nil {
		return fmt.Errorf("%w: context must not be nil", ErrInvalidRunner)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (r *Runner) execute(ctx context.Context, state *runnerState, yield func(Event, error) bool) {
	for {
		if len(state.calls) == 0 {
			if state.round >= r.maxRounds {
				yield(Event{}, fmt.Errorf("%w: limit %d", ErrRoundLimit, r.maxRounds))
				return
			}
			if !r.callModel(ctx, state, yield) {
				return
			}
			if len(state.calls) == 0 {
				return
			}
		}

		completed, direct := r.callTools(ctx, state, yield)
		if !completed {
			return
		}
		if direct {
			return
		}
		request, err := continuationRequest(state.request, state.response, state.results)
		if err != nil {
			yield(Event{}, err)
			return
		}
		state.request = request
		state.response = nil
		state.calls = nil
		state.results = nil
		state.nextCall = 0
		state.resume = nil
	}
}

func (r *Runner) callModel(ctx context.Context, state *runnerState, yield func(Event, error) bool) bool {
	eventRequest, err := snapshot(state.request)
	if err != nil {
		yield(Event{}, fmt.Errorf("toolloop: snapshot model request: %w", err))
		return false
	}
	if !yield(Event{Kind: EventModelRequest, Request: eventRequest}, nil) {
		return false
	}
	modelRequest, err := snapshot(state.request)
	if err != nil {
		yield(Event{}, fmt.Errorf("toolloop: snapshot provider request: %w", err))
		return false
	}
	response, err := r.model.Call(ctx, modelRequest)
	if err != nil {
		yield(Event{}, err)
		return false
	}
	if response == nil {
		yield(Event{}, errors.New("toolloop: model returned nil response without error"))
		return false
	}
	state.response, err = snapshot(response)
	if err != nil {
		yield(Event{}, fmt.Errorf("toolloop: invalid model response: %w", err))
		return false
	}
	state.calls, err = responseToolCalls(state.response)
	if err != nil {
		yield(Event{}, err)
		return false
	}
	state.round++
	eventResponse, err := snapshot(state.response)
	if err != nil {
		yield(Event{}, fmt.Errorf("toolloop: snapshot model response: %w", err))
		return false
	}
	return yield(Event{
		Kind:     EventModelResponse,
		Final:    len(state.calls) == 0,
		Response: eventResponse,
	}, nil)
}

func (r *Runner) callTools(ctx context.Context, state *runnerState, yield func(Event, error) bool) (completed, direct bool) {
	resolved := make([]tools.Tool, len(state.calls))
	allDirect := len(state.calls) > 0
	for i := range state.calls {
		if nilResolver(state.resolver) {
			allDirect = false
			continue
		}
		tool, ok := state.resolver.Resolve(state.calls[i].Name)
		if !ok || nilRuntimeTool(tool) {
			allDirect = false
			continue
		}
		resolved[i] = tool
		allDirect = allDirect && returnsDirectRuntime(tool)
	}

	for state.nextCall < len(state.calls) {
		position := state.nextCall
		call := state.calls[position]
		eventCall := call
		if !yield(Event{Kind: EventToolCall, ToolCall: &eventCall}, nil) {
			return false, false
		}

		result, pause, err := invokeTool(ctx, call, resolved[position], state.resume)
		state.resume = nil
		if err != nil {
			yield(Event{}, err)
			return false, false
		}
		if pause != nil {
			checkpoint, checkpointErr := r.checkpoint(state, *pause)
			if checkpointErr != nil {
				yield(Event{}, checkpointErr)
				return false, false
			}
			yield(Event{Kind: EventPause, Pause: &Pause{
				ID:         pause.ID,
				Reason:     pause.Reason,
				Checkpoint: checkpoint,
			}}, nil)
			return false, false
		}

		state.results = append(state.results, result)
		state.nextCall++
		eventResult := result
		final := allDirect && state.nextCall == len(state.calls)
		if !yield(Event{Kind: EventToolResult, Final: final, ToolResult: &eventResult}, nil) {
			return false, false
		}
	}
	return true, allDirect
}

func invokeTool(ctx context.Context, call chat.ToolCall, tool tools.Tool, resume *Resume) (chat.ToolResult, *PauseError, error) {
	if err := ctx.Err(); err != nil {
		return chat.ToolResult{}, nil, err
	}
	if nilRuntimeTool(tool) {
		return chat.ToolResult{
			ID:      call.ID,
			Name:    call.Name,
			Result:  unknownToolResult(call.Name),
			IsError: true,
		}, nil, nil
	}
	if resume != nil {
		ctx = withResume(ctx, *resume)
	}
	output, err := tool.Call(ctx, call.Arguments)
	if err == nil {
		return chat.ToolResult{ID: call.ID, Name: call.Name, Result: output}, nil, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return chat.ToolResult{}, nil, err
	}
	var pause *PauseError
	if errors.As(err, &pause) {
		if validationErr := pause.validate(); validationErr != nil {
			return chat.ToolResult{}, nil, validationErr
		}
		copy := *pause
		return chat.ToolResult{}, &copy, nil
	}
	var abort *AbortError
	if errors.As(err, &abort) {
		if validationErr := abort.validate(); validationErr != nil {
			return chat.ToolResult{}, nil, validationErr
		}
		return chat.ToolResult{}, nil, err
	}
	return chat.ToolResult{
		ID:      call.ID,
		Name:    call.Name,
		Result:  fmt.Sprintf("error: tool %q failed: %s", call.Name, err),
		IsError: true,
	}, nil, nil
}

func (r *Runner) checkpoint(state *runnerState, pause PauseError) (*Checkpoint, error) {
	request, err := snapshot(state.request)
	if err != nil {
		return nil, fmt.Errorf("toolloop: snapshot paused request: %w", err)
	}
	response, err := snapshot(state.response)
	if err != nil {
		return nil, fmt.Errorf("toolloop: snapshot paused response: %w", err)
	}
	checkpoint := &Checkpoint{
		ID:       pause.ID,
		Round:    state.round,
		Request:  request,
		Response: response,
		Results:  slices.Clone(state.results),
		NextCall: state.nextCall,
	}
	if err := checkpoint.Validate(); err != nil {
		return nil, err
	}
	return checkpoint, nil
}

func continuationRequest(request *chat.Request, response *chat.Response, results []chat.ToolResult) (*chat.Request, error) {
	choice := response.First()
	if choice == nil || choice.Message == nil {
		return nil, errors.New("toolloop: tool response has no canonical assistant message")
	}
	continuation, err := snapshot(request)
	if err != nil {
		return nil, fmt.Errorf("toolloop: snapshot continuation request: %w", err)
	}
	assistant, err := snapshot(choice.Message)
	if err != nil {
		return nil, fmt.Errorf("toolloop: snapshot assistant message: %w", err)
	}
	continuation.Messages = append(continuation.Messages, *assistant, chat.NewToolMessage(results...))
	if err := continuation.Validate(); err != nil {
		return nil, fmt.Errorf("toolloop: continuation request: %w", err)
	}
	return continuation, nil
}

func unknownToolResult(name string) string {
	return fmt.Sprintf("error: tool %q is not available", name)
}

func snapshot[T any](value *T) (*T, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var copy T
	if err := json.Unmarshal(body, &copy); err != nil {
		return nil, err
	}
	return &copy, nil
}

func nilModel(model chat.Model) bool {
	if model == nil {
		return true
	}
	value := reflect.ValueOf(model)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func nilResolver(resolver ToolResolver) bool {
	if resolver == nil {
		return true
	}
	value := reflect.ValueOf(resolver)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
