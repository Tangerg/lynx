package toolloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

const defaultMaxRounds = 50

var (
	// ErrInvalidConfig reports an invalid model or Config.
	ErrInvalidConfig = errors.New("toolloop: invalid config")
	// ErrInvalidInput reports invalid Run or Resume input.
	ErrInvalidInput = errors.New("toolloop: invalid input")
	// ErrRoundLimit reports that the model kept requesting tools through the
	// configured number of rounds.
	ErrRoundLimit = errors.New("toolloop: round limit reached")
)

// Config controls bounded loop policy. Zero MaxRounds selects 50.
// Negative values are invalid. Runner never retries model or tool calls.
type Config struct {
	MaxRounds int
}

// Runner drives a synchronous Model through tool calls. Run and Resume are
// lazy event sequences: no model or tool is called until iteration begins.
// Each run is independent and Runner is safe for concurrent use when
// its Model and ToolResolver are safe for concurrent use.
type Runner struct {
	model     chat.Model
	maxRounds int
}

// NewRunner validates model and config and returns an immutable Runner.
func NewRunner(model chat.Model, config Config) (*Runner, error) {
	if valueIsNil(model) {
		return nil, fmt.Errorf("%w: model must not be nil", ErrInvalidConfig)
	}
	if config.MaxRounds < 0 {
		return nil, fmt.Errorf("%w: max rounds must not be negative", ErrInvalidConfig)
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
func (r *Runner) Run(ctx context.Context, request *chat.Request, resolver ToolResolver) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		state, err := r.startState(ctx, request, resolver)
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
		if !yield(Event{Kind: EventResume, Round: state.round, Resume: &eventResume}, nil) {
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

func (r *Runner) startState(ctx context.Context, request *chat.Request, resolver ToolResolver) (*runnerState, error) {
	if err := r.validateContext(ctx); err != nil {
		return nil, err
	}
	state := &runnerState{request: request, resolver: resolver}
	if err := state.validateInput(); err != nil {
		return nil, err
	}
	request, err := snapshot(request)
	if err != nil {
		return nil, fmt.Errorf("%w: snapshot request: %w", ErrInvalidInput, err)
	}
	state.request = request
	return state, nil
}

func (r *Runner) resumeState(ctx context.Context, checkpoint *Checkpoint, resolver ToolResolver, resume Resume) (*runnerState, error) {
	if err := r.validateContext(ctx); err != nil {
		return nil, err
	}
	if err := checkpoint.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if err := resume.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if checkpoint.ID != resume.ID {
		return nil, fmt.Errorf("%w: resume ID %q does not match checkpoint ID %q", ErrInvalidInput, resume.ID, checkpoint.ID)
	}
	if checkpoint.MaxRounds != r.maxRounds {
		return nil, fmt.Errorf("%w: checkpoint max rounds %d does not match runner policy %d", ErrInvalidInput, checkpoint.MaxRounds, r.maxRounds)
	}
	copy, err := snapshot(checkpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: snapshot checkpoint: %w", ErrInvalidInput, err)
	}
	calls, err := responseToolCalls(copy.Response)
	if err != nil {
		return nil, fmt.Errorf("%w: checkpoint calls: %w", ErrInvalidInput, err)
	}
	state := &runnerState{
		request:  copy.Request,
		resolver: resolver,
		round:    copy.Round,
		response: copy.Response,
		calls:    calls,
		results:  slices.Clone(copy.Results),
		nextCall: copy.NextCall,
	}
	if err := state.validateInput(); err != nil {
		return nil, fmt.Errorf("%w: resumed request: %v", ErrInvalidInput, err)
	}
	return state, nil
}

func (r *Runner) validateContext(ctx context.Context) error {
	if r == nil || valueIsNil(r.model) || r.maxRounds < 1 {
		return fmt.Errorf("%w: uninitialized runner", ErrInvalidInput)
	}
	if ctx == nil {
		return fmt.Errorf("%w: context must not be nil", ErrInvalidInput)
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
		request, err := state.continuationRequest()
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
	if !yield(Event{Kind: EventModelRequest, Round: state.round + 1, Request: eventRequest}, nil) {
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
		Round:    state.round,
		Final:    len(state.calls) == 0,
		Response: eventResponse,
	}, nil)
}

func (r *Runner) callTools(ctx context.Context, state *runnerState, yield func(Event, error) bool) (completed, direct bool) {
	resolved := make([]tools.Tool, len(state.calls))
	allDirect := len(state.calls) > 0
	for i := range state.calls {
		if valueIsNil(state.resolver) {
			allDirect = false
			continue
		}
		tool, ok := state.resolver.Resolve(state.calls[i].Name)
		if !ok || valueIsNil(tool) {
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
		if !yield(Event{Kind: EventToolCall, Round: state.round, ToolCall: &eventCall}, nil) {
			return false, false
		}

		result, pause, err := state.invokeTool(ctx, call, resolved[position])
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
			yield(Event{Kind: EventPause, Round: state.round, Pause: &Pause{
				ID:           pause.ID,
				Reason:       pause.Reason,
				Prompt:       pause.Prompt,
				ResumeSchema: pause.ResumeSchema,
				Checkpoint:   checkpoint,
			}}, nil)
			return false, false
		}

		state.results = append(state.results, result)
		state.nextCall++
		eventResult := result
		final := allDirect && state.nextCall == len(state.calls)
		if !yield(Event{Kind: EventToolResult, Round: state.round, Final: final, ToolResult: &eventResult}, nil) {
			return false, false
		}
	}
	return true, allDirect
}

func (s *runnerState) invokeTool(ctx context.Context, call chat.ToolCall, tool tools.Tool) (chat.ToolResult, *PauseError, error) {
	resume := s.resume
	s.resume = nil
	if err := ctx.Err(); err != nil {
		return chat.ToolResult{}, nil, err
	}
	if valueIsNil(tool) {
		return chat.ToolResult{
			ID:      call.ID,
			Name:    call.Name,
			Result:  fmt.Sprintf("error: tool %q is not available", call.Name),
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
	var suspended *interaction.SuspendedError
	if errors.As(err, &suspended) {
		if validationErr := suspended.Suspension.Validate(); validationErr != nil {
			return chat.ToolResult{}, nil, validationErr
		}
		return chat.ToolResult{}, &PauseError{
			ID:           suspended.Suspension.ID,
			Reason:       suspended.Error(),
			Prompt:       suspended.Suspension.Prompt,
			ResumeSchema: suspended.Suspension.ResumeSchema,
		}, nil
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
		SchemaVersion: 1,
		ID:            pause.ID,
		Round:         state.round,
		MaxRounds:     r.maxRounds,
		Request:       request,
		Response:      response,
		Results:       slices.Clone(state.results),
		NextCall:      state.nextCall,
	}
	checkpoint.ToolsetDigest, err = toolsetDigest(request.Tools)
	if err != nil {
		return nil, fmt.Errorf("toolloop: digest paused toolset: %w", err)
	}
	if err := checkpoint.Validate(); err != nil {
		return nil, err
	}
	return checkpoint, nil
}

func (s *runnerState) continuationRequest() (*chat.Request, error) {
	choice := s.response.First()
	if choice == nil || choice.Message == nil {
		return nil, errors.New("toolloop: tool response has no canonical assistant message")
	}
	continuation, err := snapshot(s.request)
	if err != nil {
		return nil, fmt.Errorf("toolloop: snapshot continuation request: %w", err)
	}
	assistant, err := snapshot(choice.Message)
	if err != nil {
		return nil, fmt.Errorf("toolloop: snapshot assistant message: %w", err)
	}
	continuation.Messages = append(continuation.Messages, *assistant, chat.NewToolMessage(s.results...))
	if err := continuation.Validate(); err != nil {
		return nil, fmt.Errorf("toolloop: continuation request: %w", err)
	}
	return continuation, nil
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
