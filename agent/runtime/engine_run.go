package runtime

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

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
	// run loop. The lifecycle may also be StatusRunning after durable restore;
	// only transient run ownership makes this error true.
	ErrProcessRunning = errors.New("runtime: process is already running")

	// ErrProcessCheckpointBusy reports that another caller is capturing or
	// mutating continuation state at a stable process boundary.
	ErrProcessCheckpointBusy = errors.New("runtime: process checkpoint is busy")

	// ErrProcessActive reports an attempt to remove a process before it reaches
	// a terminal state. Call Kill first when active work must be discarded.
	ErrProcessActive = errors.New("runtime: process is active")

	// ErrProcessHasChildren reports that registry cleanup would detach a process
	// from descendants that still belong to its execution tree.
	ErrProcessHasChildren = errors.New("runtime: process still owns registered children")
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

	process, eventBindings, err := e.admitProcessRun(deployment, bindings, options)
	if err != nil {
		finishAgentRunSpan(span, nil, err)
		return nil, err
	}
	span.SetAttributes(attribute.String(attrProcessID, process.id))
	process.publishCreated(ctx, eventBindings)

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

// RunInSession runs the agent under a multi-turn session context.
// The session is stamped onto [core.ProcessOptions.Session] so action
// bodies' chat calls flow through chat history keyed by [core.Session.ID].
// When a [core.SessionStore] is configured on the engine the session is
// saved before dispatch (so a concurrent reader sees the active
// turn) and re-saved with refreshed [core.Session.UpdatedAt] after the
// dispatch completes — successful or failed.
//
// Calls sharing a session ID are ordered only within this Engine instance. The
// framework makes no cross-process or distributed coordination guarantee;
// hosts that need one must provide it outside the Engine boundary.
//
// The runtime takes an ownership-isolated copy of session. Build it via
// [core.NewSession] (or load it from the configured store) before calling. If
// agent is nil, the active deployment named by [core.Session.AgentName] is
// used. If agent is non-nil, an empty AgentName is bound to its compiled
// deployment and a conflicting name is rejected.
//
// Returns the same (*Process, error) shape as [Engine.Run].
func (e *Engine) RunInSession(
	ctx context.Context,
	agent *core.Agent,
	session core.Session,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*Process, error) {
	return e.runInSession(ctx, agent, session, bindings, options)
}

func (e *Engine) runInSession(
	ctx context.Context,
	agent *core.Agent,
	session core.Session,
	bindings core.Bindings,
	options core.ProcessOptions,
) (process *Process, err error) {
	session = session.Clone()
	deployment, err := e.sessionDeployment(ctx, agent, session)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.RunInSession: %w", err)
	}
	sessionID := session.ID

	ctx = normalizeContext(ctx)
	release, err := e.sessionTurns.acquire(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.RunInSession: acquire session turn %q: %w", sessionID, err)
	}
	defer release()

	if err := session.BindAgent(deployment.agent.Name()); err != nil {
		return nil, fmt.Errorf("runtime.Engine.RunInSession: %w", err)
	}
	if err := session.Validate(); err != nil {
		return nil, fmt.Errorf("runtime.Engine.RunInSession: %w", err)
	}
	options.Session = &session

	// Pre-dispatch save so concurrent readers see the active turn
	// (UpdatedAt = "now") even if dispatch is long-running.
	if err := e.touchAndSaveSession(ctx, &session); err != nil {
		return nil, fmt.Errorf("runtime.Engine.RunInSession: save before dispatch: %w", err)
	}

	process, runErr := e.runDeployment(ctx, deployment, bindings, options)

	// Finalization must survive request cancellation so durable audit time still
	// reflects a failed or canceled dispatch. Preserve context values and spans,
	// but detach cancellation from the store write.
	postContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), e.sessionFinalizeTimeout)
	defer cancel()
	if saveErr := e.touchAndSaveSession(postContext, &session); saveErr != nil {
		saveErr = fmt.Errorf("runtime.Engine.RunInSession: save after dispatch: %w", saveErr)
		return process, errors.Join(runErr, saveErr)
	}
	return process, runErr
}

func (e *Engine) sessionDeployment(ctx context.Context, agent *core.Agent, session core.Session) (*Deployment, error) {
	if agent != nil {
		candidate := session
		if err := candidate.BindAgent(agent.Name()); err != nil {
			return nil, err
		}
		if err := candidate.Validate(); err != nil {
			return nil, err
		}
		return e.deploymentForProcess(ctx, agent)
	}
	if err := session.Validate(); err != nil {
		return nil, err
	}
	deployment, ok := e.catalog.activeDeployment(session.AgentName)
	if !ok {
		return nil, fmt.Errorf("%w: agent %q is not active", ErrDeploymentNotFound, session.AgentName)
	}
	return deployment, nil
}

// touchAndSaveSession refreshes UpdatedAt and persists when a
// root SessionStore is configured. No-op when none is wired so callers
// don't have to nil-check the store at every save site.
func (e *Engine) touchAndSaveSession(ctx context.Context, session *core.Session) error {
	session.Touch()
	if e.sessionStore == nil {
		return nil
	}
	return e.sessionStore.Save(ctx, *session)
}

// Start deploys/resolves the Agent definition and starts one background
// [Segment]. Definition resolution and process construction errors are returned
// synchronously with a nil segment. It has the same catalog and conflict
// semantics as [Engine.Run].
func (e *Engine) Start(
	ctx context.Context,
	agent *core.Agent,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*Segment, error) {
	if agent == nil {
		return nil, errors.New("runtime.Engine.Start: agent definition is nil")
	}
	deployment, err := e.deploymentForProcess(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.Start: %w", err)
	}
	ctx, span := startAgentRunSpan(ctx, deployment.agent.Name())
	process, eventBindings, err := e.admitProcessRun(deployment, bindings, options)
	if err != nil {
		finishAgentRunSpan(span, nil, err)
		span.End()
		return nil, err
	}
	span.SetAttributes(attribute.String(attrProcessID, process.id))
	process.publishCreated(ctx, eventBindings)
	segment := newSegment(process)
	go process.runOwnedSegment(ctx, segment, func(runErr error) {
		finishAgentRunSpan(span, process, runErr)
		span.End()
	})
	return segment, nil
}

// Continue re-enters the run loop on an already-created
// process. After [Engine.Resume] records a suspension response,
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
// errors are returned synchronously; a successful call returns the one Segment
// that owns this continuation.
func (e *Engine) ContinueAsync(ctx context.Context, id string) (*Segment, error) {
	ctx = normalizeContext(ctx)
	process, started, err := e.admitContinuation(ctx, id, "runtime.Engine.ContinueAsync")
	if err != nil {
		return nil, err
	}
	segment := newSegment(process)
	if !started {
		segment.complete(process.captureCompletion(nil))
		return segment, nil
	}
	go process.runOwnedSegment(ctx, segment, nil)
	return segment, nil
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
	if suspension == nil || !suspension.Responded() {
		return fmt.Errorf("%w: process %q is still waiting for a suspension response", interaction.ErrSuspensionStale, p.ID())
	}
	return nil
}

// Resume validates and records a response for the exact suspension ID.
// The process status stays [core.StatusWaiting] until Continue re-enters
// the action and decodes the response at its original linear call site.
//
// Splitting "record response" from "drive the loop" lets the host control the
// continuation (sync vs background) while ctx bounds admission behind another
// mutation of the same process tree.
func (e *Engine) Resume(ctx context.Context, id, suspensionID string, response any) error {
	process, ok := e.Process(id)
	if !ok {
		return processNotFoundError("resume process", id)
	}
	releaseMutation, err := e.processMutations.acquire(normalizeContext(ctx), e.processTreeRootID(process))
	if err != nil {
		return fmt.Errorf("resume process %q: acquire process tree: %w", id, err)
	}
	defer releaseMutation()
	if !e.processes.available(process) {
		return processNotFoundError("resume process", id)
	}
	transaction, err := e.prepareResume(process, suspensionID, response)
	if err != nil {
		return fmt.Errorf("resume process %q: %w", id, err)
	}
	defer transaction.release()
	if err := transaction.commit(false); err != nil {
		return fmt.Errorf("resume process %q: %w", id, err)
	}
	return nil
}

// ResumeAsync atomically records a response and admits the continuation run.
// admissionCtx bounds waiting for exclusive process-tree ownership; runCtx owns
// the admitted segment's lifetime. A returned error means the response was not
// recorded. A successful call always returns the unique Segment driving the
// continuation, so callers never need to repair a half-resumed process.
func (e *Engine) ResumeAsync(
	admissionCtx context.Context,
	runCtx context.Context,
	id, suspensionID string,
	response any,
) (*Segment, error) {
	process, ok := e.Process(id)
	if !ok {
		return nil, processNotFoundError("resume process asynchronously", id)
	}
	admissionCtx = normalizeContext(admissionCtx)
	releaseMutation, err := e.processMutations.acquire(admissionCtx, e.processTreeRootID(process))
	if err != nil {
		return nil, fmt.Errorf("resume process %q asynchronously: acquire process tree: %w", id, err)
	}
	defer releaseMutation()
	if !e.processes.available(process) {
		return nil, processNotFoundError("resume process asynchronously", id)
	}
	transaction, err := e.prepareResume(process, suspensionID, response)
	if err != nil {
		return nil, fmt.Errorf("resume process %q asynchronously: %w", id, err)
	}
	defer transaction.release()
	if err := transaction.commit(true); err != nil {
		return nil, fmt.Errorf("resume process %q asynchronously: %w", id, err)
	}
	transaction.release()
	segment := newSegment(process)
	go process.runOwnedSegment(normalizeContext(runCtx), segment, nil)
	return segment, nil
}

type resumeResponse struct {
	state      *processState
	suspension string
	response   json.RawMessage
}

type resumeTransaction struct {
	root      *Process
	claims    []*processState
	responses []resumeResponse
}

func (e *Engine) prepareResume(process *Process, suspensionID string, response any) (*resumeTransaction, error) {
	transaction := &resumeTransaction{root: process}
	if err := e.collectResume(transaction, process, suspensionID, response, map[string]struct{}{}); err != nil {
		transaction.release()
		return nil, err
	}
	return transaction, nil
}

// collectResume validates the complete active nested-child branch and claims
// every process checkpoint before recording any response. Sibling children
// remain parked. Claims are acquired root → leaf, matching save traversal.
func (e *Engine) collectResume(
	transaction *resumeTransaction,
	process *Process,
	suspensionID string,
	response any,
	visited map[string]struct{},
) error {
	if process == nil {
		return errors.New("resume process: process is nil")
	}
	if _, duplicate := visited[process.ID()]; duplicate {
		return fmt.Errorf("%w: nested process cycle at %q", interaction.ErrSuspensionConflict, process.ID())
	}
	visited[process.ID()] = struct{}{}
	if err := process.state.claimCheckpoint(false); err != nil {
		return err
	}
	transaction.claims = append(transaction.claims, &process.state)

	suspension := process.Suspension()
	if suspension == nil || process.Status() != core.StatusWaiting || suspension.ID != suspensionID {
		return fmt.Errorf("%w: process %q has no pending suspension %q", interaction.ErrSuspensionStale, process.ID(), suspensionID)
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
		if child.Status() == core.StatusWaiting {
			if err := e.collectResume(transaction, child, relation.SuspensionID, response, visited); err != nil {
				return err
			}
		}
	}
	transaction.responses = append(transaction.responses, resumeResponse{
		state:      &process.state,
		suspension: suspensionID,
		response:   canonical,
	})
	return nil
}

func (transaction *resumeTransaction) commit(start bool) error {
	type appliedResponse struct {
		state    *processState
		previous *interaction.Suspension
	}
	var applied []appliedResponse
	rollback := func() {
		for index := len(applied) - 1; index >= 0; index-- {
			applied[index].state.restoreClaimedSuspension(applied[index].previous)
		}
	}
	now := time.Now()
	for _, prepared := range transaction.responses {
		previous, changed, err := prepared.state.installClaimedSuspensionResponse(
			prepared.suspension,
			prepared.response,
			now,
		)
		if err != nil {
			rollback()
			return err
		}
		if changed {
			applied = append(applied, appliedResponse{state: prepared.state, previous: previous})
		}
	}
	if !start {
		return nil
	}
	started, err := transaction.root.state.beginRunFromCheckpoint()
	if err != nil {
		rollback()
		return err
	}
	if !started {
		rollback()
		return fmt.Errorf("%w: process %q cannot continue from its terminal state", interaction.ErrSuspensionStale, transaction.root.ID())
	}
	return nil
}

func (transaction *resumeTransaction) release() {
	if transaction == nil {
		return
	}
	for index := len(transaction.claims) - 1; index >= 0; index-- {
		transaction.claims[index].releaseCheckpoint()
	}
	transaction.claims = nil
}

// Kill terminates a process and its live descendants. It transitions the
// target and descendants to [core.StatusKilled], cancels their active Run /
// Continue contexts and current tool calls, completes any Kill-owned automatic
// snapshots, then publishes [event.ProcessKilled]. When automatic snapshots
// are enabled, Kill persists idle or completion-publishing targets itself; only
// the driving phase owns the final snapshot. Kill returns any descendant or
// snapshot failures. It is idempotent and safe on
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
	if !e.processes.available(process) {
		releaseMutation()
		return false, processNotFoundError("kill process", id)
	}
	if err := e.ensureSubtreeMutationAvailable(process); err != nil {
		releaseMutation()
		return false, fmt.Errorf("runtime.Engine.Kill: %w", err)
	}
	won, killed, snapshots := e.killSubtreeOwned(process)
	releaseMutation()
	snapshotErr := snapshotKilledProcesses(ctx, snapshots)
	publishKilledProcesses(ctx, killed)
	return won, snapshotErr
}

// killSubtreeOwned performs only internal state transitions while the caller
// owns the process-tree mutation. Event listeners and stores are caller code;
// they run after this transaction releases ownership so they may safely reenter
// the Engine.
func (e *Engine) killSubtreeOwned(process *Process) (bool, []*Process, []*Process) {
	won, driverWillSnapshot := process.state.markKilled(nil)
	if won {
		process.signals.fireRunCancel()
		process.signals.fireToolCallCancel()
	}

	children := e.directChildren(process.ID())
	var killed []*Process
	var snapshots []*Process
	if won {
		killed = append(killed, process)
	}
	for _, child := range children {
		_, childKilled, childSnapshots := e.killSubtreeOwned(child)
		killed = append(killed, childKilled...)
		snapshots = append(snapshots, childSnapshots...)
	}
	if won && !driverWillSnapshot {
		snapshots = append(snapshots, process)
	}
	return won, killed, snapshots
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

func publishKilledProcesses(ctx context.Context, processes []*Process) {
	for _, process := range processes {
		process.publishEvent(ctx, event.ProcessKilled{
			Header: event.NewHeader(process.ID()),
			Reason: "kill requested",
		})
	}
}

func snapshotKilledProcesses(ctx context.Context, processes []*Process) error {
	var errs []error
	for _, process := range processes {
		if err := process.maybeAutoSnapshot(ctx); err != nil {
			errs = append(errs, fmt.Errorf("snapshot killed process %q: %w", process.ID(), err))
		}
	}
	return errors.Join(errs...)
}

// KillChildren terminates every non-terminal direct child whose ParentID
// matches parentID and returns the ids it changed in lexical order. Each child
// Kill recursively terminates its own descendants. The returned error joins
// every descendant or snapshot failure without abandoning the remaining
// children.
func (e *Engine) KillChildren(ctx context.Context, parentID string) ([]string, error) {
	ctx = normalizeContext(ctx)
	if parent, ok := e.Process(parentID); ok {
		releaseMutation, err := e.processMutations.acquire(ctx, e.processTreeRootID(parent))
		if err != nil {
			return nil, fmt.Errorf("runtime.Engine.KillChildren: acquire process tree: %w", err)
		}
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
		killed, events, snapshots := e.killChildrenOwned(parentID)
		releaseMutation()
		snapshotErr := snapshotKilledProcesses(ctx, snapshots)
		publishKilledProcesses(ctx, events)
		return killed, snapshotErr
	}

	processes := e.directChildren(parentID)
	var killed []string
	var killErrs []error
	for _, process := range processes {
		won, err := e.killProcess(ctx, process.ID())
		if err != nil {
			killErrs = append(killErrs, fmt.Errorf("kill child process %q: %w", process.ID(), err))
		}
		if won {
			killed = append(killed, process.ID())
		}
	}
	return killed, errors.Join(killErrs...)
}

func (e *Engine) killChildrenOwned(parentID string) ([]string, []*Process, []*Process) {
	processes := e.directChildren(parentID)
	var killed []string
	var events []*Process
	var snapshots []*Process
	for _, process := range processes {
		won, processEvents, processSnapshots := e.killSubtreeOwned(process)
		events = append(events, processEvents...)
		snapshots = append(snapshots, processSnapshots...)
		if won {
			killed = append(killed, process.ID())
		}
	}
	return killed, events, snapshots
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

// Remove deletes a terminal process from the registry so long-running hosts
// can free work they have drained. Active processes must be killed and allowed
// to finish first; rejecting their removal keeps cancellation, child ownership,
// and durable cleanup reachable through the Engine.
func (e *Engine) Remove(ctx context.Context, id string) error {
	process, found := e.processes.get(id)
	if !found {
		return processNotFoundError("remove process", id)
	}
	releaseMutation, err := e.processMutations.acquire(normalizeContext(ctx), e.processTreeRootID(process))
	if err != nil {
		return fmt.Errorf("runtime.Engine.Remove: acquire process tree: %w", err)
	}
	defer releaseMutation()
	if !e.processes.reserveProcesses([]*Process{process}) {
		return processNotFoundError("remove process", id)
	}
	reserved := true
	defer func() {
		if reserved {
			e.processes.releaseProcesses([]*Process{process})
		}
	}()
	if !process.state.removable() {
		return fmt.Errorf("runtime.Engine.Remove: process %q: %w", id, ErrProcessActive)
	}
	found, hasChildren := e.processes.unregisterReservedLeaf(process)
	if !found {
		return processNotFoundError("remove process", id)
	}
	if hasChildren {
		return fmt.Errorf("runtime.Engine.Remove: process %q: %w", id, ErrProcessHasChildren)
	}
	reserved = false
	return nil
}

// Prune removes every registered process whose
// status satisfies [core.ProcessStatus.IsTerminal] and returns
// the removed ids. Convenient cleanup for long-lived hosts.
func (e *Engine) Prune(ctx context.Context) ([]string, error) {
	var removed []string
	for {
		processes := e.Processes()
		slices.SortFunc(processes, func(left, right *Process) int {
			return cmp.Or(
				cmp.Compare(right.depth, left.depth),
				cmp.Compare(left.id, right.id),
			)
		})
		pruned := 0
		for _, process := range processes {
			if !process.Status().IsTerminal() {
				continue
			}
			err := e.Remove(ctx, process.ID())
			switch {
			case err == nil:
				removed = append(removed, process.ID())
				pruned++
			case errors.Is(err, ErrProcessActive),
				errors.Is(err, ErrProcessHasChildren),
				errors.Is(err, ErrProcessNotFound):
				continue
			default:
				return removed, err
			}
		}
		if pruned == 0 {
			return removed, nil
		}
	}
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
