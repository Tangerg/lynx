package toolloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/internal/nilvalue"
	"github.com/Tangerg/lynx/agent/internal/toolcall"
	"github.com/Tangerg/lynx/core/chat"
)

var (
	// ErrInvalidConfig reports an invalid model or Config.
	ErrInvalidConfig = errors.New("toolloop: invalid config")
	// ErrInvalidInput reports invalid Run or Resume input.
	ErrInvalidInput = errors.New("toolloop: invalid input")
	// ErrRoundLimit reports that the model kept requesting tools through the
	// configured number of rounds.
	ErrRoundLimit = errors.New("toolloop: round limit reached")
)

// Config controls loop policy. Both limits belong to whoever drives the loop,
// so Runner never substitutes a value of its own: zero MaxRounds runs until the
// model stops requesting tools, and zero MaxConcurrentCalls executes one call at
// a time. Negative values are invalid. Runner never retries model or tool calls.
type Config struct {
	MaxRounds          int
	MaxConcurrentCalls int
}

// Runner drives a synchronous Model through tool calls. Run, Resume, Continue,
// and ContinuePaused are lazy event sequences: no model or tool is called
// until iteration begins.
// Each run is independent and Runner is safe for concurrent use when
// its Model and ToolResolver are safe for concurrent use.
type Runner struct {
	model              chat.Model
	maxRounds          int
	maxConcurrentCalls int
}

// NewRunner validates model and config and returns an immutable Runner.
func NewRunner(model chat.Model, config Config) (*Runner, error) {
	if nilvalue.Is(model) {
		return nil, fmt.Errorf("%w: model must not be nil", ErrInvalidConfig)
	}
	if config.MaxRounds < 0 {
		return nil, fmt.Errorf("%w: max rounds must not be negative", ErrInvalidConfig)
	}
	if config.MaxConcurrentCalls < 0 {
		return nil, fmt.Errorf("%w: max concurrent calls must not be negative", ErrInvalidConfig)
	}
	return &Runner{
		model:              model,
		maxRounds:          config.MaxRounds,
		maxConcurrentCalls: config.MaxConcurrentCalls,
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
		if _, awaiting, err := checkpoint.AwaitingInput(); err != nil {
			yield(Event{}, fmt.Errorf("%w: %w", ErrInvalidInput, err))
			return
		} else if !awaiting {
			yield(Event{}, fmt.Errorf("%w: checkpoint has no pending input", ErrInvalidInput))
			return
		}
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

// Continue advances a checkpoint whose next unpublished result was completed
// by the host while the loop remained parked. It emits no Resume event and
// invokes no tool for that already-settled result; ordinary ordered publication
// and any subsequent queued work proceed through the same runner state machine.
func (r *Runner) Continue(ctx context.Context, checkpoint *Checkpoint, resolver ToolResolver) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		if _, awaiting, err := checkpoint.AwaitingInput(); err != nil {
			yield(Event{}, fmt.Errorf("%w: %w", ErrInvalidInput, err))
			return
		} else if awaiting {
			yield(Event{}, fmt.Errorf("%w: checkpoint is still awaiting input", ErrInvalidInput))
			return
		}
		state, err := r.checkpointState(ctx, checkpoint, resolver)
		if err != nil {
			yield(Event{}, err)
			return
		}
		r.execute(ctx, state, yield)
	}
}

// ContinuePaused re-enters the active paused tool after the host has proved
// that the tool's durable dependency is internally ready. It emits no Resume
// event, supplies no resume input, and does not invoke any already-completed
// tool. This is distinct from Resume: it must not be used to bypass a tool that
// still requires external input.
func (r *Runner) ContinuePaused(ctx context.Context, checkpoint *Checkpoint, resolver ToolResolver) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		if _, awaiting, err := checkpoint.AwaitingInput(); err != nil {
			yield(Event{}, fmt.Errorf("%w: %w", ErrInvalidInput, err))
			return
		} else if !awaiting {
			yield(Event{}, fmt.Errorf("%w: checkpoint has no active paused tool", ErrInvalidInput))
			return
		}
		state, err := r.checkpointState(ctx, checkpoint, resolver)
		if err != nil {
			yield(Event{}, err)
			return
		}
		state.continuePaused = true
		r.execute(ctx, state, yield)
	}
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
	state, err := r.checkpointState(ctx, checkpoint, resolver)
	if err != nil {
		return nil, err
	}
	if err := resume.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if checkpoint.ID != resume.ID {
		return nil, fmt.Errorf("%w: resume ID %q does not match checkpoint ID %q", ErrInvalidInput, resume.ID, checkpoint.ID)
	}
	return state, nil
}

func (r *Runner) checkpointState(ctx context.Context, checkpoint *Checkpoint, resolver ToolResolver) (*runnerState, error) {
	if err := r.validateContext(ctx); err != nil {
		return nil, err
	}
	if err := checkpoint.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	captured, err := snapshot(checkpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: snapshot checkpoint: %w", ErrInvalidInput, err)
	}
	calls, err := responseToolCalls(captured.Response)
	if err != nil {
		return nil, fmt.Errorf("%w: checkpoint calls: %w", ErrInvalidInput, err)
	}
	state := &runnerState{
		request:    captured.Request,
		resolver:   resolver,
		round:      captured.Round,
		response:   captured.Response,
		calls:      calls,
		callStates: cloneCallStates(captured.CallStates),
		nextResult: captured.NextResult,
	}
	if err := state.validateInput(); err != nil {
		return nil, fmt.Errorf("%w: resumed request: %w", ErrInvalidInput, err)
	}
	return state, nil
}

func (r *Runner) validateContext(ctx context.Context) error {
	if r == nil || nilvalue.Is(r.model) || r.maxRounds < 0 || r.maxConcurrentCalls < 0 {
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
			if r.maxRounds > 0 && state.round >= r.maxRounds {
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
	plans, allDirect, err := planCalls(state.resolver, state.calls)
	if err != nil {
		yield(Event{}, fmt.Errorf("toolloop: plan tool calls: %w", err))
		return false, false
	}

	if state.resume != nil || state.continuePaused {
		position := state.nextResult
		if position < 0 || position >= len(state.callStates) || state.callStates[position].Status != CallPaused {
			yield(Event{}, fmt.Errorf("toolloop: continuation has no active paused call at result %d", position))
			return false, false
		}
		call := state.calls[position]
		if state.continuePaused {
			allowed, err := plans[position].hosted.canContinueWithoutInput()
			if err != nil {
				yield(Event{}, err)
				return false, false
			}
			if !allowed {
				yield(Event{}, fmt.Errorf("%w: tool %q does not allow inputless continuation", ErrInvalidInput, call.Name))
				return false, false
			}
		}
		eventCall := call
		if !yield(Event{Kind: EventToolCall, Round: state.round, ToolCall: &eventCall}, nil) {
			return false, false
		}
		resume := state.resume
		state.resume = nil
		state.continuePaused = false
		result, pending, err := invokeTool(ctx, call, plans[position].hosted, resume)
		if err != nil {
			yield(Event{}, err)
			return false, false
		}
		state.settled(position, result, pending)
	}

	for {
		published, paused, err := r.publishSettled(state, allDirect, yield)
		if err != nil {
			yield(Event{}, err)
			return false, false
		}
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
		result, pending, err := invokeTool(ctx, state.calls[start], plans[start].hosted, nil)
		if err != nil {
			return err
		}
		state.settled(start, result, pending)
		return nil
	}

	outcomes := make([]toolOutcome, end-start)
	group, groupContext := errgroup.WithContext(ctx)
	// Unset concurrency means one call at a time, which is also the floor: the
	// scheduler never runs zero goroutines.
	group.SetLimit(max(1, r.maxConcurrentCalls))
	for index := start; index < end; index++ {
		group.Go(func() error {
			result, pending, err := invokeTool(groupContext, state.calls[index], plans[index].hosted, nil)
			outcomes[index-start] = toolOutcome{result: result, pending: pending, err: err}
			return err
		})
	}
	// The group's error is only the first branch to fail; per-call attribution
	// comes from outcomes below, so joining is all Wait is needed for.
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
) (published, paused bool, err error) {
	// Fold any tools promoted by the segment just run into the advertised
	// toolset before this call can build a pause checkpoint or the loop can
	// build the continuation request: every runSegment is followed by a
	// publishSettled, so this covers both the checkpoint and continuation paths
	// with one merge point. request.Tools grows monotonically within a turn.
	if err := r.mergePromotions(state); err != nil {
		return false, false, fmt.Errorf("toolloop: merge promoted tools: %w", err)
	}
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
				return false, false, nil
			}
			state.nextResult++
		case CallPaused:
			checkpoint, err := r.checkpoint(state)
			if err != nil {
				return false, false, err
			}
			pending := callState.Pending
			yield(Event{Kind: EventPause, Round: state.round, Pause: &Pause{
				ID:           pending.ID,
				Reason:       pending.Reason,
				Prompt:       pending.Prompt,
				ResumeSchema: pending.ResumeSchema,
				Checkpoint:   checkpoint,
			}}, nil)
			return true, true, nil
		case CallQueued:
			return true, false, nil
		default:
			return false, false, fmt.Errorf("toolloop: invalid in-memory call status %q", callState.Status)
		}
	}
	return true, false, nil
}

// mergePromotions drains the promotion sink and advertises each definition that
// (a) is valid, (b) is not already advertised, and (c) resolves to a matching
// tool — the same advertised⊆resolvable invariant validateInput enforces at
// start/resume, applied here so a mid-loop growth of request.Tools can never
// advertise a name the runner cannot execute (which would fail a later resume).
func (r *Runner) mergePromotions(state *runnerState) error {
	promoted := state.promotions.drain()
	if len(promoted) == 0 {
		return nil
	}
	advertised := make(map[string]struct{}, len(state.request.Tools))
	for _, def := range state.request.Tools {
		advertised[def.Name] = struct{}{}
	}
	accepted := make([]chat.ToolDefinition, 0, len(promoted))
	for _, def := range promoted {
		def = def.Clone()
		if def.Validate() != nil {
			continue
		}
		if _, ok := advertised[def.Name]; ok {
			continue
		}
		// A promotion that no longer resolves, or that no longer describes
		// itself the way it was promoted, is dropped rather than rejected: the
		// round is still valid without it.
		_, matched, err := executableFor(state.resolver, def)
		if err != nil {
			return err
		}
		if !matched {
			continue
		}
		advertised[def.Name] = struct{}{}
		accepted = append(accepted, def)
	}
	// Promotions arrive in whatever order concurrent tools reached the sink, so
	// ordering them by name is what keeps one execution reproducible: the same
	// round otherwise sends the model a differently ordered toolset, which also
	// costs a prompt-cache hit. Only the additions are ordered — the manifest the
	// caller supplied stays as the caller arranged it.
	slices.SortFunc(accepted, func(left, right chat.ToolDefinition) int {
		return strings.Compare(left.Name, right.Name)
	})
	state.request.Tools = append(state.request.Tools, accepted...)
	return nil
}

func invokeTool(
	ctx context.Context,
	call chat.ToolCall,
	hosted hostedTool,
	resume *Resume,
) (chat.ToolResult, *PendingCall, error) {
	if err := ctx.Err(); err != nil {
		return chat.ToolResult{}, nil, err
	}
	if hosted.tool == nil {
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
	output, err := hosted.call(ctx, call.Arguments)
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
		SchemaVersion: CheckpointSchemaVersion,
		ID:            active.Pending.ID,
		Round:         state.round,
		Request:       request,
		Response:      response,
		CallStates:    cloneCallStates(state.callStates),
		NextResult:    state.nextResult,
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

func snapshot[T any](value *T) (*T, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var cloned T
	if err := json.Unmarshal(body, &cloned); err != nil {
		return nil, err
	}
	return &cloned, nil
}
