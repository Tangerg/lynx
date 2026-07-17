package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/interaction"
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
	bindings map[string]any,
	options core.ProcessOptions,
) (*Process, error) {
	if agent == nil {
		return nil, errors.New("runtime.Engine.Run: agent definition is nil")
	}
	deployment, err := e.deploymentForProcess(agent)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.Run: %w", err)
	}
	return e.runDeployment(ctx, deployment, bindings, options)
}

func (e *Engine) runDeployment(
	ctx context.Context,
	deployment *Deployment,
	bindings map[string]any,
	options core.ProcessOptions,
) (*Process, error) {
	process, err := e.createProcessFromDeployment(deployment, bindings, options)
	if err != nil {
		return nil, err
	}

	ctx, span := agentTracer.Start(normalizeContext(ctx), "agent.run",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("gen_ai.agent.name", process.agent().Name()),
			attribute.String("agent.process.id", process.id),
		),
	)
	defer span.End()

	if err := process.run(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return process, err
	}
	span.SetAttributes(attribute.String("agent.status", process.Status().String()))
	return process, nil
}

// RunInSession runs the agent under a multi-turn session context.
// The session is stamped onto [ProcessOptions.Session] so action
// bodies' chat calls flow through chat history keyed by [Session.ID].
// When a [SessionStore] is configured on the engine the session is
// saved before dispatch (so a concurrent reader sees the active
// turn) and re-saved with refreshed [Session.UpdatedAt] after the
// dispatch completes — successful or failed.
//
// Passing a nil session is rejected; build a session via
// [core.NewSession] (or load one via the configured store) before
// calling.
//
// Returns the same (*Process, error) shape as [Engine.Run].
func (e *Engine) RunInSession(
	ctx context.Context,
	agent *core.Agent,
	session *core.Session,
	bindings map[string]any,
	options core.ProcessOptions,
) (*Process, error) {
	if session == nil {
		return nil, errors.New("runtime.Engine.RunInSession: session must not be nil")
	}
	if session.ID == "" {
		return nil, errors.New("runtime.Engine.RunInSession: session ID must not be empty")
	}
	if session.AgentName == "" && agent != nil {
		deployment, err := e.deploymentForProcess(agent)
		if err != nil {
			return nil, fmt.Errorf("run in session: compile agent: %w", err)
		}
		session.AgentName = deployment.agent.Name()
	}
	options.Session = session

	// Pre-dispatch save so concurrent readers see the active turn
	// (UpdatedAt = "now") even if dispatch is long-running.
	if err := e.touchAndSaveSession(ctx, session); err != nil {
		return nil, fmt.Errorf("run in session: save (pre-dispatch): %w", err)
	}

	process, runErr := e.Run(ctx, agent, bindings, options)

	// Post-dispatch save runs even on Run error so the store
	// reflects activity. Only override runErr's nil with the save
	// error — a real run error wins.
	if saveErr := e.touchAndSaveSession(ctx, session); saveErr != nil && runErr == nil {
		return process, fmt.Errorf("run in session: save (post-dispatch): %w", saveErr)
	}
	return process, runErr
}

// touchAndSaveSession refreshes UpdatedAt and persists when a
// SessionStore is configured. No-op when none is wired so callers
// don't have to nil-check the store at every save site.
func (e *Engine) touchAndSaveSession(ctx context.Context, session *core.Session) error {
	session.Touch()
	if e.sessionStore == nil {
		return nil
	}
	return e.sessionStore.Save(ctx, *session)
}

// SessionStore returns the configured session-persistence backend,
// or nil when the engine was constructed without one.
func (e *Engine) SessionStore() core.SessionStore { return e.sessionStore }

// Start deploys/resolves the Agent definition and runs it in the background,
// returning the process and a channel that delivers the final error (or nil on
// success). It has the same catalog and conflict semantics as [Engine.Run].
func (e *Engine) Start(
	ctx context.Context,
	agent *core.Agent,
	bindings map[string]any,
	options core.ProcessOptions,
) (*Process, <-chan error) {
	done := make(chan error, 1)

	process, err := e.createProcess(agent, bindings, options)
	if err != nil {
		done <- err
		close(done)
		return nil, done
	}
	go func() {
		done <- process.run(normalizeContext(ctx))
		close(done)
	}()
	return process, done
}

// Continue re-enters the run loop on an already-created
// process. After [Engine.Resume] records a suspension response,
// or after a stuck policy stages new blackboard state,
// Continue drives the OODA loop until the process exits
// Running again (terminal, waiting, or paused).
//
// Concurrent Continue calls on the same id are safe — the
// underlying beginRun rejects when the process is already running
// so only one call drives the loop.
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
// [Engine.Continue]. The returned buffered channel
// receives the run's final error (nil on clean exit) so callers can
// fire-and-forget while still being able to wait on completion.
func (e *Engine) ContinueAsync(ctx context.Context, id string) <-chan error {
	done := make(chan error, 1)

	process, ok := e.Process(id)
	if !ok {
		done <- processNotFoundError("continue process asynchronously", id)
		close(done)
		return done
	}
	if err := process.ensureContinuable(); err != nil {
		done <- err
		close(done)
		return done
	}
	go func() {
		done <- process.run(normalizeContext(ctx))
		close(done)
	}()
	return done
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
// children. Idempotent and safe on any process — an already-terminal one is
// left untouched, so a kill racing natural completion cannot clobber a clean
// terminal state or publish a duplicate event.
func (e *Engine) Kill(id string) error {
	process, ok := e.Process(id)
	if !ok {
		return processNotFoundError("kill process", id)
	}
	if !process.state.markKilled() {
		return nil
	}
	process.signals.fireRunCancel()
	process.signals.fireToolCallCancel()
	e.publish(event.ProcessKilled{
		Header: event.NewHeader(id),
		Reason: "kill requested",
	})
	e.KillChildren(id)
	return nil
}

// KillChildren terminates every non-terminal direct child whose ParentID
// matches parentID and returns those direct child ids (order unspecified).
// Each child Kill recursively terminates its own descendants.
func (e *Engine) KillChildren(parentID string) []string {
	var killed []string
	for _, process := range e.processes.list() {
		if process.ParentID() != parentID || process.Status().IsTerminal() {
			continue
		}
		if err := e.Kill(process.ID()); err == nil {
			killed = append(killed, process.ID())
		}
	}
	return killed
}

// Remove deletes a process from the registry so long-running hosts can free
// terminal-state processes they have already
// drained. Returns an error when the id is unknown so callers can
// detect typos.
func (e *Engine) Remove(id string) error {
	if !e.processes.unregister(id) {
		return processNotFoundError("remove process", id)
	}
	return nil
}

// Prune removes every registered process whose
// status satisfies [core.ProcessStatus.IsTerminal] and returns
// the removed ids. Convenient cleanup for long-lived hosts.
func (e *Engine) Prune() []string {
	return e.processes.pruneWhere(func(process *Process) bool {
		return process.Status().IsTerminal()
	})
}

func processNotFoundError(operation, id string) error {
	return fmt.Errorf("%s: process %q not found", operation, id)
}
