package toolloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"

	"golang.org/x/sync/errgroup"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/agent/internal/toolcall"
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

// Config controls bounded loop policy. Zero values select framework defaults.
// Negative values are invalid. Runner never retries model or tool calls.
type Config struct {
	MaxRounds          int
	MaxConcurrentCalls int
}

// Runner drives a synchronous Model through tool calls. Run and Resume are
// lazy event sequences: no model or tool is called until iteration begins.
// Each run is independent and Runner is safe for concurrent use when
// its Model and ToolResolver are safe for concurrent use.
type Runner struct {
	model              chat.Model
	maxRounds          int
	maxConcurrentCalls int
}

// NewRunner validates model and config and returns an immutable Runner.
func NewRunner(model chat.Model, config Config) (*Runner, error) {
	if valueIsNil(model) {
		return nil, fmt.Errorf("%w: model must not be nil", ErrInvalidConfig)
	}
	if config.MaxRounds < 0 {
		return nil, fmt.Errorf("%w: max rounds must not be negative", ErrInvalidConfig)
	}
	if config.MaxConcurrentCalls < 0 {
		return nil, fmt.Errorf("%w: max concurrent calls must not be negative", ErrInvalidConfig)
	}
	maxRounds := config.MaxRounds
	if maxRounds == 0 {
		maxRounds = defaultMaxRounds
	}
	maxConcurrentCalls := config.MaxConcurrentCalls
	if maxConcurrentCalls == 0 {
		maxConcurrentCalls = DefaultMaxConcurrentCalls
	}
	return &Runner{
		model:              model,
		maxRounds:          maxRounds,
		maxConcurrentCalls: maxConcurrentCalls,
	}, nil
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
	request    *chat.Request
	resolver   ToolResolver
	round      int
	response   *chat.Response
	calls      []chat.ToolCall
	callStates []CallCheckpoint
	nextResult int
	resume     *Resume
	// promotions collects tools promoted mid-loop (see PromoteTools). It is
	// drained into request.Tools before every checkpoint or continuation, so a
	// promoted tool is advertised on the next model round and rides through a
	// pause/resume inside the checkpoint's request.
	promotions toolPromotions
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
	if checkpoint.MaxConcurrentCalls != r.maxConcurrentCalls {
		return nil, fmt.Errorf(
			"%w: checkpoint max concurrent calls %d does not match runner policy %d",
			ErrInvalidInput,
			checkpoint.MaxConcurrentCalls,
			r.maxConcurrentCalls,
		)
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
		request:    copy.Request,
		resolver:   resolver,
		round:      copy.Round,
		response:   copy.Response,
		calls:      calls,
		callStates: cloneCallStates(copy.CallStates),
		nextResult: copy.NextResult,
	}
	if err := state.validateInput(); err != nil {
		return nil, fmt.Errorf("%w: resumed request: %w", ErrInvalidInput, err)
	}
	return state, nil
}

func (r *Runner) validateContext(ctx context.Context) error {
	if r == nil || valueIsNil(r.model) || r.maxRounds < 1 || r.maxConcurrentCalls < 1 {
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
	// Bind the promotion sink for the whole interaction so a tool can advertise
	// resolvable-but-withheld tools (PromoteTools) for subsequent rounds.
	ctx = withPromotions(ctx, &state.promotions)
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
		state.callStates = nil
		state.nextResult = 0
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
	state.callStates = make([]CallCheckpoint, len(state.calls))
	for index := range state.callStates {
		state.callStates[index].Status = CallQueued
	}
	state.nextResult = 0
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
	plans, allDirect := planCalls(state.resolver, state.calls)

	if state.resume != nil {
		position := state.nextResult
		if position < 0 || position >= len(state.callStates) || state.callStates[position].Status != CallPaused {
			yield(Event{}, fmt.Errorf("toolloop: resume has no active paused call at result %d", position))
			return false, false
		}
		call := state.calls[position]
		eventCall := call
		if !yield(Event{Kind: EventToolCall, Round: state.round, ToolCall: &eventCall}, nil) {
			return false, false
		}
		resume := state.resume
		state.resume = nil
		result, pending, err := invokeTool(ctx, call, plans[position].tool, resume)
		if err != nil {
			yield(Event{}, err)
			return false, false
		}
		state.settled(position, result, pending)
	}

	for {
		published, paused := r.publishSettled(state, allDirect, yield)
		if !published {
			return false, false
		}
		if paused {
			return false, false
		}

		start := state.startedCalls()
		if start == len(state.calls) {
			return true, allDirect
		}
		end := segmentEnd(plans, start)
		for index := start; index < end; index++ {
			eventCall := state.calls[index]
			if !yield(Event{Kind: EventToolCall, Round: state.round, ToolCall: &eventCall}, nil) {
				return false, false
			}
		}
		if err := r.runSegment(ctx, state, plans, start, end); err != nil {
			yield(Event{}, err)
			return false, false
		}
	}
}

type toolOutcome struct {
	result  chat.ToolResult
	pending *PendingCall
	err     error
}

func (r *Runner) runSegment(
	ctx context.Context,
	state *runnerState,
	plans []callPlan,
	start int,
	end int,
) error {
	if end-start == 1 {
		result, pending, err := invokeTool(ctx, state.calls[start], plans[start].tool, nil)
		if err != nil {
			return err
		}
		state.settled(start, result, pending)
		return nil
	}

	outcomes := make([]toolOutcome, end-start)
	group, groupContext := errgroup.WithContext(ctx)
	group.SetLimit(r.maxConcurrentCalls)
	for index := start; index < end; index++ {
		group.Go(func() error {
			result, pending, err := invokeTool(groupContext, state.calls[index], plans[index].tool, nil)
			outcomes[index-start] = toolOutcome{result: result, pending: pending, err: err}
			return err
		})
	}
	_ = group.Wait()

	if err := ctx.Err(); err != nil {
		return err
	}
	var canceled error
	for index, outcome := range outcomes {
		if outcome.err == nil {
			continue
		}
		if errors.Is(outcome.err, context.Canceled) || errors.Is(outcome.err, context.DeadlineExceeded) {
			if canceled == nil {
				canceled = outcome.err
			}
			continue
		}
		return fmt.Errorf("toolloop: tool call %q failed: %w", state.calls[start+index].ID, outcome.err)
	}
	if canceled != nil {
		return canceled
	}
	for index, outcome := range outcomes {
		state.settled(start+index, outcome.result, outcome.pending)
	}
	return nil
}

func (r *Runner) publishSettled(
	state *runnerState,
	allDirect bool,
	yield func(Event, error) bool,
) (published, paused bool) {
	// Fold any tools promoted by the segment just run into the advertised
	// toolset before this call can build a pause checkpoint or the loop can
	// build the continuation request: every runSegment is followed by a
	// publishSettled, so this covers both the checkpoint and continuation paths
	// with one merge point. request.Tools grows monotonically within a turn.
	r.mergePromotions(state)
	for state.nextResult < len(state.callStates) {
		callState := state.callStates[state.nextResult]
		switch callState.Status {
		case CallCompleted:
			eventResult := *callState.Result
			final := allDirect && state.nextResult == len(state.callStates)-1
			if !yield(Event{
				Kind:       EventToolResult,
				Round:      state.round,
				Final:      final,
				ToolResult: &eventResult,
			}, nil) {
				return false, false
			}
			state.nextResult++
		case CallPaused:
			checkpoint, err := r.checkpoint(state)
			if err != nil {
				yield(Event{}, err)
				return false, false
			}
			pending := callState.Pending
			yield(Event{Kind: EventPause, Round: state.round, Pause: &Pause{
				ID:           pending.ID,
				Reason:       pending.Reason,
				Prompt:       pending.Prompt,
				ResumeSchema: pending.ResumeSchema,
				Checkpoint:   checkpoint,
			}}, nil)
			return true, true
		case CallQueued:
			return true, false
		default:
			yield(Event{}, fmt.Errorf("toolloop: invalid in-memory call status %q", callState.Status))
			return false, false
		}
	}
	return true, false
}

// mergePromotions drains the promotion sink and advertises each definition that
// (a) is valid, (b) is not already advertised, and (c) resolves to a matching
// tool — the same advertised⊆resolvable invariant validateInput enforces at
// start/resume, applied here so a mid-loop growth of request.Tools can never
// advertise a name the runner cannot execute (which would fail a later resume).
func (r *Runner) mergePromotions(state *runnerState) {
	promoted := state.promotions.drain()
	if len(promoted) == 0 {
		return
	}
	advertised := make(map[string]struct{}, len(state.request.Tools))
	for _, def := range state.request.Tools {
		advertised[def.Name] = struct{}{}
	}
	for _, def := range promoted {
		def = def.Clone()
		if def.Validate() != nil {
			continue
		}
		if _, ok := advertised[def.Name]; ok {
			continue
		}
		tool, ok := state.resolver.Resolve(def.Name)
		if !ok || valueIsNil(tool) || !sameToolDefinition(def, tool.Definition()) {
			continue
		}
		advertised[def.Name] = struct{}{}
		state.request.Tools = append(state.request.Tools, def)
	}
}

func (s *runnerState) startedCalls() int {
	for index, state := range s.callStates {
		if state.Status == CallQueued {
			return index
		}
	}
	return len(s.callStates)
}

func (s *runnerState) settled(index int, result chat.ToolResult, pending *PendingCall) {
	if pending != nil {
		copy := *pending
		s.callStates[index] = CallCheckpoint{Status: CallPaused, Pending: &copy}
		return
	}
	copy := result
	s.callStates[index] = CallCheckpoint{Status: CallCompleted, Result: &copy}
}

func invokeTool(
	ctx context.Context,
	call chat.ToolCall,
	tool tools.Tool,
	resume *Resume,
) (chat.ToolResult, *PendingCall, error) {
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
	ctx = toolcall.Bind(ctx, call)
	if resume != nil {
		ctx = withResume(ctx, *resume)
	}
	output, err := callRuntimeTool(ctx, tool, call.Arguments)
	if err == nil {
		return chat.ToolResult{ID: call.ID, Name: call.Name, Result: output}, nil, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return chat.ToolResult{}, nil, err
	}
	if suspended, ok := errors.AsType[*interaction.SuspendedError](err); ok {
		if validationErr := suspended.Suspension.Validate(); validationErr != nil {
			return chat.ToolResult{}, nil, validationErr
		}
		return chat.ToolResult{}, &PendingCall{
			ID:           suspended.Suspension.ID,
			Reason:       suspended.Error(),
			Prompt:       suspended.Suspension.Prompt,
			ResumeSchema: suspended.Suspension.ResumeSchema,
		}, nil
	}
	if pause, ok := errors.AsType[*PauseError](err); ok {
		if validationErr := pause.validate(); validationErr != nil {
			return chat.ToolResult{}, nil, validationErr
		}
		return chat.ToolResult{}, &PendingCall{
			ID:           pause.ID,
			Reason:       pause.Reason,
			Prompt:       pause.Prompt,
			ResumeSchema: pause.ResumeSchema,
		}, nil
	}
	if abort, ok := errors.AsType[*AbortError](err); ok {
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

// callRuntimeTool contains panics at the executable extension boundary. A tool
// panic is recoverable model feedback, not permission for one plugin goroutine
// to terminate the host process or discard sibling results.
func callRuntimeTool(ctx context.Context, tool tools.Tool, arguments string) (output string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New("tool panicked", recovered)
		}
	}()
	return tool.Call(ctx, arguments)
}

func (r *Runner) checkpoint(state *runnerState) (*Checkpoint, error) {
	request, err := snapshot(state.request)
	if err != nil {
		return nil, fmt.Errorf("toolloop: snapshot paused request: %w", err)
	}
	response, err := snapshot(state.response)
	if err != nil {
		return nil, fmt.Errorf("toolloop: snapshot paused response: %w", err)
	}
	active := state.callStates[state.nextResult]
	if active.Status != CallPaused || active.Pending == nil {
		return nil, errors.New("toolloop: checkpoint has no active pending call")
	}
	checkpoint := &Checkpoint{
		SchemaVersion:      CheckpointSchemaVersion,
		ID:                 active.Pending.ID,
		Round:              state.round,
		MaxRounds:          r.maxRounds,
		MaxConcurrentCalls: r.maxConcurrentCalls,
		Request:            request,
		Response:           response,
		CallStates:         cloneCallStates(state.callStates),
		NextResult:         state.nextResult,
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
	results, err := s.completedResults()
	if err != nil {
		return nil, err
	}
	continuation.Messages = append(continuation.Messages, *assistant, chat.NewToolMessage(results...))
	if err := continuation.Validate(); err != nil {
		return nil, fmt.Errorf("toolloop: continuation request: %w", err)
	}
	return continuation, nil
}

func (s *runnerState) completedResults() ([]chat.ToolResult, error) {
	if len(s.callStates) != len(s.calls) {
		return nil, errors.New("toolloop: call state count does not match response calls")
	}
	results := make([]chat.ToolResult, len(s.callStates))
	for index, state := range s.callStates {
		if state.Status != CallCompleted || state.Result == nil {
			return nil, fmt.Errorf("toolloop: call %q has no completed result", s.calls[index].ID)
		}
		results[index] = *state.Result
	}
	return results, nil
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
