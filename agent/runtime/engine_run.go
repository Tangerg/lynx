package runtime

import (
	"cmp"
	"context"
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

	process, err := e.createProcessFromDeployment(ctx, deployment, bindings, options)
	if err != nil {
		finishAgentRunSpan(span, nil, err)
		return nil, err
	}
	span.SetAttributes(attribute.String(attrProcessID, process.id))

	if err := process.run(ctx); err != nil {
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
	release, err := acquireSessionTurn(ctx, e.sessionTurnSequencer, sessionID)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.RunInSession: acquire session turn %q: %w", sessionID, err)
	}
	defer func() {
		if releaseErr := releaseSessionTurn(release); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("runtime.Engine.RunInSession: release session turn %q: %w", sessionID, releaseErr))
		}
	}()

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

// SessionStore returns the configured root-session persistence backend, or nil
// when the engine was constructed without one.
func (e *Engine) SessionStore() core.SessionStore { return e.sessionStore }

// Start deploys/resolves the Agent definition and runs it in the background,
// returning the process and a channel that delivers the final run error (or nil
// on success). Definition resolution and process construction errors are
// returned synchronously with a nil process and channel. It has the same
// catalog and conflict semantics as [Engine.Run].
func (e *Engine) Start(
	ctx context.Context,
	agent *core.Agent,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*Process, <-chan error, error) {
	if agent == nil {
		return nil, nil, errors.New("runtime.Engine.Start: agent definition is nil")
	}
	deployment, err := e.deploymentForProcess(ctx, agent)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime.Engine.Start: %w", err)
	}
	ctx, span := startAgentRunSpan(ctx, deployment.agent.Name())
	process, err := e.createProcessFromDeployment(ctx, deployment, bindings, options)
	if err != nil {
		finishAgentRunSpan(span, nil, err)
		span.End()
		return nil, nil, err
	}
	span.SetAttributes(attribute.String(attrProcessID, process.id))
	done := make(chan error, 1)
	go func() {
		err := process.run(ctx)
		finishAgentRunSpan(span, process, err)
		span.End()
		done <- err
		close(done)
	}()
	return process, done, nil
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
	process, ok := e.Process(id)
	if !ok {
		return processNotFoundError("continue process", id)
	}
	if err := process.ensureContinuable(); err != nil {
		return err
	}
	return process.run(normalizeContext(ctx))
}

// ContinueAsync is the background variant of
// [Engine.Continue]. Admission errors are returned synchronously; after a run
// starts, the returned buffered channel receives its final error (nil on clean
// exit).
func (e *Engine) ContinueAsync(ctx context.Context, id string) (<-chan error, error) {
	process, ok := e.Process(id)
	if !ok {
		return nil, processNotFoundError("continue process asynchronously", id)
	}
	if err := process.ensureContinuable(); err != nil {
		return nil, err
	}
	started, err := process.beginRun()
	if err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	if !started {
		done <- nil
		close(done)
		return done, nil
	}
	go func() {
		done <- process.runOwned(ctx)
		close(done)
	}()
	return done, nil
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
// Splitting "record response" from "drive the loop" keeps
// Resume cheap, synchronous, and ctx-free, and lets the host
// control the continuation (sync vs background, fresh ctx vs the
// original).
func (e *Engine) Resume(id, suspensionID string, response any) error {
	process, ok := e.Process(id)
	if !ok {
		return processNotFoundError("resume process", id)
	}
	if err := e.resumeProcess(process, suspensionID, response, map[string]struct{}{}); err != nil {
		return fmt.Errorf("resume process %q: %w", id, err)
	}
	return nil
}

// resumeProcess records one response along the active nested-child branch back
// to the requested parent. Sibling children remain parked. Locks are acquired
// root → leaf, matching save traversal and avoiding child → parent cycles.
func (e *Engine) resumeProcess(process *Process, suspensionID string, response any, visited map[string]struct{}) error {
	if process == nil {
		return errors.New("resume process: process is nil")
	}
	if _, duplicate := visited[process.ID()]; duplicate {
		return fmt.Errorf("%w: nested process cycle at %q", interaction.ErrSuspensionConflict, process.ID())
	}
	visited[process.ID()] = struct{}{}
	process.checkpointMu.Lock()
	defer process.checkpointMu.Unlock()

	suspension := process.Suspension()
	if suspension == nil || process.Status() != core.StatusWaiting || suspension.ID != suspensionID {
		return fmt.Errorf("%w: process %q has no pending suspension %q", interaction.ErrSuspensionStale, process.ID(), suspensionID)
	}
	if _, err := suspension.ValidateResponse(response); err != nil {
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
			if err := e.resumeProcess(child, relation.SuspensionID, response, visited); err != nil {
				return err
			}
		}
	}
	if err := process.state.respondToSuspension(suspensionID, response, time.Now()); err != nil {
		return err
	}
	return nil
}

// Kill terminates a process and its live descendants. It transitions the
// target to [core.StatusKilled], cancels its active Run / Continue context and
// current tool call, publishes [event.ProcessKilled], then recursively kills
// children. When automatic snapshots are enabled, Kill persists idle targets
// itself; an active run remains responsible for its final snapshot. Kill
// returns any descendant or snapshot failures. It is idempotent and safe on
// any process: an already-terminal one is left untouched, so a kill racing
// natural completion cannot clobber a clean terminal state or publish a
// duplicate event.
func (e *Engine) Kill(ctx context.Context, id string) error {
	process, ok := e.Process(id)
	if !ok {
		return processNotFoundError("kill process", id)
	}
	won, runOwned := process.state.markKilled(nil)
	if won {
		process.signals.fireRunCancel()
		process.signals.fireToolCallCancel()
		process.publishEvent(normalizeContext(ctx), event.ProcessKilled{
			Header: event.NewHeader(id),
			Reason: "kill requested",
		})
	}
	// Descendants have independent run ownership. Sweep them even when the
	// target was already terminal: a completed parent may still own background
	// children started with StartChild.
	_, childErr := e.killChildren(ctx, id)
	var snapshotErr error
	if won && !runOwned {
		snapshotErr = process.maybeAutoSnapshot(ctx)
	}
	return errors.Join(childErr, snapshotErr)
}

// KillChildren terminates every non-terminal direct child whose ParentID
// matches parentID and returns the ids it changed in lexical order. Each child
// Kill recursively terminates its own descendants. The returned error joins
// every descendant or snapshot failure without abandoning the remaining
// children.
func (e *Engine) KillChildren(ctx context.Context, parentID string) ([]string, error) {
	return e.killChildren(normalizeContext(ctx), parentID)
}

func (e *Engine) killChildren(ctx context.Context, parentID string) ([]string, error) {
	processes := e.processes.list()
	slices.SortFunc(processes, func(left, right *Process) int {
		return cmp.Compare(left.ID(), right.ID())
	})
	var killed []string
	var killErrs []error
	for _, process := range processes {
		if process.ParentID() != parentID {
			continue
		}
		wasTerminal := process.Status().IsTerminal()
		if err := e.Kill(ctx, process.ID()); err != nil {
			killErrs = append(killErrs, fmt.Errorf("kill child process %q: %w", process.ID(), err))
		}
		if !wasTerminal && process.Status() == core.StatusKilled {
			killed = append(killed, process.ID())
		}
	}
	return killed, errors.Join(killErrs...)
}

// Remove deletes a terminal process from the registry so long-running hosts
// can free work they have drained. Active processes must be killed and allowed
// to finish first; rejecting their removal keeps cancellation, child ownership,
// and durable cleanup reachable through the Engine.
func (e *Engine) Remove(id string) error {
	found, terminal, hasChildren := e.processes.unregisterTerminalLeaf(id)
	if !found {
		return processNotFoundError("remove process", id)
	}
	if !terminal {
		return fmt.Errorf("runtime.Engine.Remove: process %q: %w", id, ErrProcessActive)
	}
	if hasChildren {
		return fmt.Errorf("runtime.Engine.Remove: process %q: %w", id, ErrProcessHasChildren)
	}
	return nil
}

// Prune removes every registered process whose
// status satisfies [core.ProcessStatus.IsTerminal] and returns
// the removed ids. Convenient cleanup for long-lived hosts.
func (e *Engine) Prune() []string {
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
			found, terminal, hasChildren := e.processes.unregisterTerminalLeaf(process.ID())
			if found && terminal && !hasChildren {
				removed = append(removed, process.ID())
				pruned++
			}
		}
		if pruned == 0 {
			return removed
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
