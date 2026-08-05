package runtime

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/interaction"
)

var (
	// ErrProcessNotFound is the stable identity for an operation that addressed a
	// process no longer present in the engine registry. Callers performing
	// idempotent teardown can match it with [errors.Is] without parsing text.
	ErrProcessNotFound = errors.New("runtime: process not found")

	// ErrProcessRunning reports that another caller currently owns the process
	// run loop.
	ErrProcessRunning = errors.New("runtime: process is already running")

	// ErrProcessCheckpointBusy reports that another caller is capturing or
	// mutating continuation state at a stable process boundary.
	ErrProcessCheckpointBusy = errors.New("runtime: process checkpoint is busy")

	// ErrProcessActive reports an attempt to remove a process before it reaches
	// a terminal state. Call Kill first when active work must be discarded.
	ErrProcessActive = errors.New("runtime: process is active")

	// ErrChildProcessCanceled reports that a parked AgentTool invocation was
	// settled by canceling the delegated child rather than by producing its
	// typed result.
	ErrChildProcessCanceled = errors.New("runtime: delegated child process was canceled")
)

// Run deploys/resolves the Agent definition, runs it synchronously, and returns
// the resulting process (whether completed or terminal-failed). The first run
// of a definition installs its immutable Deployment in the catalog; later runs
// resolve that exact deployment. A conflicting active definition still
// requires explicit [Engine.Replace]. Pass zero [core.ProcessOptions]{} for
// defaults.
//
// One `agent.run` span wraps the full invocation, parenting the
// per-tick / per-action / per-plan child spans the runtime emits
// during execution. See doc/OBSERVABILITY.md §3.3 / §4.7.
func (e *Engine) Run(
	ctx context.Context,
	agent *core.Agent,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*Process, error) {
	if agent == nil {
		return nil, errors.New("runtime.Engine.Run: agent definition is nil")
	}
	deployment, err := e.deploymentForProcess(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.Run: %w", err)
	}
	return e.runDeployment(ctx, deployment, bindings, options)
}

func (e *Engine) runDeployment(
	ctx context.Context,
	deployment *Deployment,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*Process, error) {
	ctx, span := startAgentRunSpan(ctx, deployment.agent.Name())
	defer span.End()

	process, err := e.admitProcessRun(deployment, bindings, options)
	if err != nil {
		finishAgentRunSpan(span, nil, err)
		return nil, err
	}
	span.SetAttributes(attribute.String(attrProcessID, process.id))
	process.publishCreated(ctx)

	if err := process.runOwned(ctx); err != nil {
		finishAgentRunSpan(span, process, err)
		return process, err
	}
	finishAgentRunSpan(span, process, nil)
	return process, nil
}

func startAgentRunSpan(ctx context.Context, agentName string) (context.Context, trace.Span) {
	return agentTracer.Start(normalizeContext(ctx), spanRun,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String(attrAgentName, agentName)),
	)
}

func finishAgentRunSpan(span trace.Span, process *Process, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	if process != nil {
		span.SetAttributes(attribute.String(attrProcessStatus, process.Status().String()))
	}
}

// Start deploys/resolves the Agent definition and starts one background
// [RunHandle]. Definition resolution and process construction errors are returned
// synchronously with a nil handle. It has the same catalog and conflict
// semantics as [Engine.Run].
func (e *Engine) Start(
	ctx context.Context,
	agent *core.Agent,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*RunHandle, error) {
	if agent == nil {
		return nil, errors.New("runtime.Engine.Start: agent definition is nil")
	}
	deployment, err := e.deploymentForProcess(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.Start: %w", err)
	}
	ctx, span := startAgentRunSpan(ctx, deployment.agent.Name())
	process, err := e.admitProcessRun(deployment, bindings, options)
	if err != nil {
		finishAgentRunSpan(span, nil, err)
		span.End()
		return nil, err
	}
	span.SetAttributes(attribute.String(attrProcessID, process.id))
	process.publishCreated(ctx)
	handle := newRunHandle(process)
	go process.runOwnedHandle(ctx, handle, func(runErr error) {
		finishAgentRunSpan(span, process, runErr)
		span.End()
	})
	return handle, nil
}

// Continue re-enters the run loop on an already-created
// process. After [Engine.Respond] records a suspension response,
// or after a stuck policy stages new blackboard state,
// Continue drives the OODA loop until the process exits
// Running again (terminal, waiting, or paused).
//
// Concurrent Continue calls on the same id are safe. Exactly one caller drives
// the loop; overlapping callers receive [ErrProcessRunning].
func (e *Engine) Continue(ctx context.Context, id string) error {
	ctx = normalizeContext(ctx)
	process, started, err := e.admitContinuation(ctx, id, "runtime.Engine.Continue")
	if err != nil {
		return err
	}
	if !started {
		return nil
	}
	return process.runOwned(ctx)
}

// ContinueAsync is the background variant of [Engine.Continue]. Admission
// errors are returned synchronously; a successful call returns the one RunHandle
// that owns this continuation.
func (e *Engine) ContinueAsync(ctx context.Context, id string) (*RunHandle, error) {
	ctx = normalizeContext(ctx)
	process, started, err := e.admitContinuation(ctx, id, "runtime.Engine.ContinueAsync")
	if err != nil {
		return nil, err
	}
	handle := newRunHandle(process)
	if !started {
		handle.complete(process.captureCompletion(nil))
		return handle, nil
	}
	go process.runOwnedHandle(ctx, handle, nil)
	return handle, nil
}

func (e *Engine) admitContinuation(
	ctx context.Context,
	id string,
	operation string,
) (*Process, bool, error) {
	process, ok := e.Process(id)
	if !ok {
		return nil, false, processNotFoundError(operation, id)
	}
	releaseMutation, err := e.processMutations.acquire(ctx, e.processTreeRootID(process))
	if err != nil {
		return nil, false, fmt.Errorf("%s: acquire process tree: %w", operation, err)
	}
	defer releaseMutation()
	if !e.processes.available(process) {
		return nil, false, processNotFoundError(operation, id)
	}
	if err := process.ensureContinuable(); err != nil {
		return nil, false, err
	}
	started, err := process.beginRun()
	return process, started, err
}

func (p *Process) ensureContinuable() error {
	if p.Status() != core.StatusWaiting {
		return nil
	}
	suspension := p.Suspension()
	continuable, err := suspensionContinuable(suspension)
	if err != nil {
		return fmt.Errorf("runtime: inspect process %q continuation: %w", p.ID(), err)
	}
	if !continuable {
		return fmt.Errorf("%w: process %q is still waiting for a suspension response", interaction.ErrSuspensionStale, p.ID())
	}
	return nil
}

// Respond validates and records a response for the exact suspension ID.
// The process status stays [core.StatusWaiting] until Continue re-enters
// the action and decodes the response at its original linear call site.
//
// Splitting "record response" from "drive the loop" lets the host control the
// continuation (sync vs background) while ctx bounds admission behind another
// mutation of the same process tree.
func (e *Engine) Respond(ctx context.Context, id, suspensionID string, response any) error {
	process, ok := e.Process(id)
	if !ok {
		return processNotFoundError("respond to process", id)
	}
	releaseMutation, err := e.processMutations.acquire(normalizeContext(ctx), e.processTreeRootID(process))
	if err != nil {
		return fmt.Errorf("respond to process %q: acquire process tree: %w", id, err)
	}
	defer releaseMutation()
	if !e.processes.available(process) {
		return processNotFoundError("respond to process", id)
	}
	admission, err := e.prepareResponse(process, suspensionID, response)
	if err != nil {
		return fmt.Errorf("respond to process %q: %w", id, err)
	}
	defer admission.release()
	if err := admission.apply(false); err != nil {
		return fmt.Errorf("respond to process %q: %w", id, err)
	}
	return nil
}

// RespondAndContinueAsync atomically records a response and admits the continuation run.
// admissionCtx bounds waiting for exclusive process-tree ownership; runCtx owns
// the admitted run's lifetime. A returned error means the response was not
// recorded. A successful call always returns the unique RunHandle driving the
// continuation, so callers never need to repair a half-resumed process.
func (e *Engine) RespondAndContinueAsync(
	admissionCtx context.Context,
	runCtx context.Context,
	id, suspensionID string,
	response any,
) (*RunHandle, error) {
	process, ok := e.Process(id)
	if !ok {
		return nil, processNotFoundError("respond to and continue process", id)
	}
	admissionCtx = normalizeContext(admissionCtx)
	releaseMutation, err := e.processMutations.acquire(admissionCtx, e.processTreeRootID(process))
	if err != nil {
		return nil, fmt.Errorf("respond to and continue process %q: acquire process tree: %w", id, err)
	}
	defer releaseMutation()
	if !e.processes.available(process) {
		return nil, processNotFoundError("respond to and continue process", id)
	}
	admission, err := e.prepareResponse(process, suspensionID, response)
	if err != nil {
		return nil, fmt.Errorf("respond to and continue process %q: %w", id, err)
	}
	defer admission.release()
	if err := admission.apply(true); err != nil {
		return nil, fmt.Errorf("respond to and continue process %q: %w", id, err)
	}
	admission.release()
	handle := newRunHandle(process)
	go process.runOwnedHandle(normalizeContext(runCtx), handle, nil)
	return handle, nil
}

type suspensionResponse struct {
	state        *processState
	suspensionID string
	responseJSON json.RawMessage
}

type responseAdmission struct {
	root      *Process
	claims    []*processState
	responses []suspensionResponse
}

func (e *Engine) prepareResponse(process *Process, suspensionID string, response any) (*responseAdmission, error) {
	admission := &responseAdmission{root: process}
	if err := e.collectResponse(admission, process, suspensionID, response, map[string]struct{}{}); err != nil {
		admission.release()
		return nil, err
	}
	return admission, nil
}

// collectResponse validates the complete active nested-child branch and claims
// every process checkpoint before recording any response. Sibling children
// remain parked. Claims are acquired root → leaf, matching snapshot traversal.
func (e *Engine) collectResponse(
	admission *responseAdmission,
	process *Process,
	suspensionID string,
	response any,
	visited map[string]struct{},
) error {
	if process == nil {
		return errors.New("respond to process: process is nil")
	}
	if _, duplicate := visited[process.ID()]; duplicate {
		return fmt.Errorf("%w: nested process cycle at %q", interaction.ErrSuspensionConflict, process.ID())
	}
	visited[process.ID()] = struct{}{}
	if err := process.state.claimCheckpoint(false); err != nil {
		return err
	}
	admission.claims = append(admission.claims, &process.state)

	suspension := process.Suspension()
	if suspension == nil || process.Status() != core.StatusWaiting || suspension.ID != suspensionID {
		return fmt.Errorf("%w: process %q has no pending suspension %q", interaction.ErrSuspensionStale, process.ID(), suspensionID)
	}
	continuable, err := suspensionContinuable(suspension)
	if err != nil {
		return err
	}
	if continuable {
		return fmt.Errorf("%w: process %q suspension %q is already continuable", interaction.ErrSuspensionStale, process.ID(), suspensionID)
	}
	canonical, err := suspension.ValidateResponse(response)
	if err != nil {
		return err
	}
	checkpoint, err := nestedChildrenFromSuspension(suspension)
	if err != nil {
		return err
	}
	relation := checkpoint.active
	if relation != nil {
		child, ok := e.Process(relation.ChildID)
		if !ok {
			return fmt.Errorf("%w: nested child process %q is missing", interaction.ErrSuspensionStale, relation.ChildID)
		}
		if err := relation.validateProcess(process, child); err != nil {
			return err
		}
		if childSuspension := child.Suspension(); childSuspension != nil && child.Status() == core.StatusWaiting {
			if err := e.collectResponse(admission, child, childSuspension.ID, response, visited); err != nil {
				return err
			}
		}
	}
	admission.responses = append(admission.responses, suspensionResponse{
		state:        &process.state,
		suspensionID: suspensionID,
		responseJSON: canonical,
	})
	return nil
}

func (admission *responseAdmission) apply(start bool) error {
	type appliedResponse struct {
		state    *processState
		previous *interaction.Suspension
	}
	var applied []appliedResponse
	revert := func() {
		for index := len(applied) - 1; index >= 0; index-- {
			applied[index].state.restoreClaimedSuspension(applied[index].previous)
		}
	}
	for _, prepared := range admission.responses {
		previous, err := prepared.state.installClaimedSuspensionResponse(
			prepared.suspensionID,
			prepared.responseJSON,
		)
		if err != nil {
			revert()
			return err
		}
		applied = append(applied, appliedResponse{state: prepared.state, previous: previous})
	}
	if !start {
		return nil
	}
	started, err := admission.root.state.beginRunFromCheckpoint()
	if err != nil {
		revert()
		return err
	}
	if !started {
		revert()
		return fmt.Errorf("%w: process %q cannot continue from its terminal state", interaction.ErrSuspensionStale, admission.root.ID())
	}
	return nil
}

func (admission *responseAdmission) release() {
	for index := len(admission.claims) - 1; index >= 0; index-- {
		admission.claims[index].releaseCheckpoint()
	}
	admission.claims = nil
}

// Kill terminates a process and its live descendants. It transitions the
// target and descendants to [core.StatusKilled], cancels their active Run /
// Continue contexts and current tool calls, then publishes
// [event.ProcessKilled]. It is idempotent and safe on
// any process: an already-terminal one is left untouched, so a kill racing
// natural completion cannot clobber a clean terminal state or publish a
// duplicate event.
func (e *Engine) Kill(ctx context.Context, id string) error {
	_, err := e.killProcess(ctx, id)
	return err
}

func (e *Engine) killProcess(ctx context.Context, id string) (bool, error) {
	process, ok := e.Process(id)
	if !ok {
		return false, processNotFoundError("kill process", id)
	}
	ctx = normalizeContext(ctx)
	releaseMutation, err := e.processMutations.acquire(ctx, e.processTreeRootID(process))
	if err != nil {
		return false, fmt.Errorf("runtime.Engine.Kill: acquire process tree: %w", err)
	}
	defer releaseMutation()
	if !e.processes.available(process) {
		releaseMutation()
		return false, processNotFoundError("kill process", id)
	}
	if err := e.ensureSubtreeMutationAvailable(process); err != nil {
		releaseMutation()
		return false, fmt.Errorf("runtime.Engine.Kill: %w", err)
	}
	won, killed := e.killSubtreeOwned(process)
	releaseMutation()
	publishKilledProcesses(ctx, killed, "kill requested")
	return won, nil
}

// killSubtreeOwned performs only internal state transitions while the caller
// owns the process-tree mutation. Event listeners are caller code; they run
// after this critical section releases ownership so they may safely reenter the
// Engine.
func (e *Engine) killSubtreeOwned(process *Process) (bool, []*Process) {
	won := process.state.markKilled(nil)
	if won {
		process.signals.fireRunCancel()
		process.signals.cancelActiveToolCall()
	}

	children := e.directChildren(process.ID())
	var killed []*Process
	for _, child := range children {
		_, childKilled := e.killSubtreeOwned(child)
		killed = append(killed, childKilled...)
	}
	if won {
		killed = append(killed, process)
	}
	return won, killed
}

func (e *Engine) ensureSubtreeMutationAvailable(process *Process) error {
	if process.state.checkpointBusy() {
		return fmt.Errorf("process %q: %w", process.ID(), ErrProcessCheckpointBusy)
	}
	for _, child := range e.directChildren(process.ID()) {
		if err := e.ensureSubtreeMutationAvailable(child); err != nil {
			return err
		}
	}
	return nil
}

func publishKilledProcesses(ctx context.Context, processes []*Process, reason string) {
	for _, process := range processes {
		process.publishEvent(ctx, event.ProcessKilled{
			Header: event.NewHeader(process.ID()),
			Reason: reason,
		})
	}
}

// KillChildren terminates every non-terminal direct child whose ParentID
// matches parentID and returns the ids it changed in lexical order. Each child
// termination recursively includes its descendants. parentID must identify a
// registered process in the same complete tree.
func (e *Engine) KillChildren(ctx context.Context, parentID string) ([]string, error) {
	ctx = normalizeContext(ctx)
	parent, ok := e.Process(parentID)
	if !ok {
		return nil, processNotFoundError("kill child processes", parentID)
	}
	releaseMutation, err := e.processMutations.acquire(ctx, e.processTreeRootID(parent))
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.KillChildren: acquire process tree: %w", err)
	}
	defer releaseMutation()
	if !e.processes.available(parent) {
		releaseMutation()
		return nil, processNotFoundError("kill child processes", parentID)
	}
	for _, child := range e.directChildren(parentID) {
		if err := e.ensureSubtreeMutationAvailable(child); err != nil {
			releaseMutation()
			return nil, fmt.Errorf("runtime.Engine.KillChildren: %w", err)
		}
	}
	killed, events := e.killChildrenOwned(parentID)
	releaseMutation()
	publishKilledProcesses(ctx, events, "kill children requested")
	return killed, nil
}

func (e *Engine) killChildrenOwned(parentID string) ([]string, []*Process) {
	processes := e.directChildren(parentID)
	var killed []string
	var events []*Process
	for _, process := range processes {
		won, processEvents := e.killSubtreeOwned(process)
		events = append(events, processEvents...)
		if won {
			killed = append(killed, process.ID())
		}
	}
	return killed, events
}

func (e *Engine) directChildren(parentID string) []*Process {
	processes := e.processes.list()
	processes = slices.DeleteFunc(processes, func(process *Process) bool {
		return process.ParentID() != parentID
	})
	slices.SortFunc(processes, func(left, right *Process) int {
		return cmp.Compare(left.ID(), right.ID())
	})
	return processes
}

type processNotFound struct {
	operation string
	id        string
}

func (e *processNotFound) Error() string {
	return fmt.Sprintf("%s: process %q not found", e.operation, e.id)
}

func (*processNotFound) Unwrap() error { return ErrProcessNotFound }

func processNotFoundError(operation, id string) error {
	return &processNotFound{operation: operation, id: id}
}
