package runtime

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// RunAgent runs the named agent synchronously and returns the
// resulting process (whether completed or terminal-failed). Pass a
// zero [core.ProcessOptions]{} for defaults.
//
// One `agent.run` span wraps the full invocation, parenting the
// per-tick / per-action / per-plan child spans the runtime emits
// during execution. See doc/OBSERVABILITY.md §3.3 / §4.7.
func (p *Platform) RunAgent(
	ctx context.Context,
	agentDef *core.Agent,
	bindings map[string]any,
	options core.ProcessOptions,
) (*AgentProcess, error) {
	proc, err := p.createProcess(agentDef, bindings, options)
	if err != nil {
		return nil, err
	}

	ctx, span := core.AgentTracer().Start(normalizeContext(ctx), "agent.run",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("gen_ai.agent.name", agentDef.Name),
			attribute.String("agent.process.id", proc.id),
		),
	)
	defer span.End()

	if err := proc.run(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return proc, err
	}
	span.SetAttributes(attribute.String("agent.status", proc.Status().String()))
	return proc, nil
}

// RunInSession runs the agent under a multi-turn session context.
// The session is stamped onto [ProcessOptions.Session] so action
// bodies' chat calls flow through chat-memory keyed by [Session.ID].
// When a [SessionStore] is configured on the platform the session is
// saved before dispatch (so a concurrent reader sees the active
// turn) and re-saved with refreshed [Session.UpdatedAt] after the
// dispatch completes — successful or failed.
//
// Passing a nil session is rejected; build a session via
// [core.NewSession] (or load one via the configured store) before
// calling.
//
// Returns the same (*AgentProcess, error) shape as [RunAgent].
func (p *Platform) RunInSession(
	ctx context.Context,
	agentDef *core.Agent,
	session *core.Session,
	bindings map[string]any,
	options core.ProcessOptions,
) (*AgentProcess, error) {
	if session == nil {
		return nil, errors.New("run in session: session must not be nil")
	}
	if session.ID == "" {
		return nil, errors.New("run in session: session ID must not be empty")
	}
	if session.AgentName == "" && agentDef != nil {
		session.AgentName = agentDef.Name
	}
	options.Session = session

	// Pre-dispatch save so concurrent readers see the active turn
	// (UpdatedAt = "now") even if dispatch is long-running.
	if err := p.touchAndSaveSession(ctx, session); err != nil {
		return nil, fmt.Errorf("run in session: save (pre-dispatch): %w", err)
	}

	proc, runErr := p.RunAgent(ctx, agentDef, bindings, options)

	// Post-dispatch save runs even on RunAgent error so the store
	// reflects activity. Only override runErr's nil with the save
	// error — a real run error wins.
	if saveErr := p.touchAndSaveSession(ctx, session); saveErr != nil && runErr == nil {
		return proc, fmt.Errorf("run in session: save (post-dispatch): %w", saveErr)
	}
	return proc, runErr
}

// touchAndSaveSession refreshes UpdatedAt and persists when a
// SessionStore is configured. No-op when none is wired so callers
// don't have to nil-check the store at every save site.
func (p *Platform) touchAndSaveSession(ctx context.Context, session *core.Session) error {
	if p.sessionStore == nil {
		return nil
	}
	session.Touch()
	return p.sessionStore.Save(ctx, *session)
}

// SessionStore returns the configured session-persistence backend,
// or nil when the platform was constructed without one.
func (p *Platform) SessionStore() core.SessionStore { return p.sessionStore }

// StartAgent runs the agent in the background, returning the process
// and a channel that delivers the final error (or nil on success).
func (p *Platform) StartAgent(
	ctx context.Context,
	agentDef *core.Agent,
	bindings map[string]any,
	options core.ProcessOptions,
) (*AgentProcess, <-chan error) {
	done := make(chan error, 1)

	proc, err := p.createProcess(agentDef, bindings, options)
	if err != nil {
		done <- err
		close(done)
		return nil, done
	}
	go func() {
		done <- proc.run(normalizeContext(ctx))
		close(done)
	}()
	return proc, done
}

// ContinueProcess re-enters the run loop on an already-created
// process. Mirrors embabel's pattern: after [Platform.ResumeProcess]
// delivers an awaitable response, or after a stuck-handler stages
// new blackboard state, ContinueProcess drives the OODA loop until
// the process exits Running again (terminal, waiting, or paused).
//
// Concurrent ContinueProcess calls on the same id are safe — the
// underlying makeRunning rejects when the process is already running
// so only one call drives the loop.
func (p *Platform) ContinueProcess(ctx context.Context, id string) error {
	proc, ok := p.ProcessByID(id)
	if !ok {
		return processNotFoundError("continue process", id)
	}
	return proc.run(normalizeContext(ctx))
}

// ContinueProcessAsync is the background variant of
// [Platform.ContinueProcess]. The returned buffered channel
// receives the run's final error (nil on clean exit) so callers can
// fire-and-forget while still being able to wait on completion.
func (p *Platform) ContinueProcessAsync(ctx context.Context, id string) <-chan error {
	done := make(chan error, 1)

	proc, ok := p.ProcessByID(id)
	if !ok {
		done <- processNotFoundError("continue process asynchronously", id)
		close(done)
		return done
	}
	go func() {
		done <- proc.run(normalizeContext(ctx))
		close(done)
	}()
	return done
}

// ResumeProcess delivers a response to a process parked on
// [AgentProcess.AwaitInput]. The awaitable's typed handler runs
// synchronously (typically mutating the blackboard) and returns the
// [core.ResponseImpact] decision. The process status stays
// [core.StatusWaiting] — call [Platform.ContinueProcess] /
// [Platform.ContinueProcessAsync] next to drive the run loop
// against the now-mutated blackboard.
//
// Splitting "deliver response" from "drive the loop" keeps
// ResumeProcess cheap, synchronous, and ctx-free, and lets the host
// control the continuation (sync vs background, fresh ctx vs the
// original).
func (p *Platform) ResumeProcess(id string, response any) (core.ResponseImpact, error) {
	proc, ok := p.ProcessByID(id)
	if !ok {
		return core.ImpactUnchanged, processNotFoundError("resume process", id)
	}
	impact, err := proc.signals.deliverResponse(response)
	if err != nil {
		return core.ImpactUnchanged, fmt.Errorf("resume process %q: %w", id, err)
	}
	return impact, nil
}

// KillProcess terminates a running process. Returns an error when
// the id is unknown.
func (p *Platform) KillProcess(id string) error {
	proc, ok := p.ProcessByID(id)
	if !ok {
		return processNotFoundError("kill process", id)
	}
	proc.state.setStatus(core.StatusKilled)
	p.publish(event.ProcessKilled{
		BaseEvent: event.NewBaseEvent(id),
		Reason:    "kill requested",
	})
	return nil
}

// KillChildren terminates every non-terminal process whose ParentID
// matches parentID and returns the killed ids (order unspecified).
// Orchestrators call it on turn exit to sweep background children
// spawned via [SpawnChildAsync] that are still running, so background
// work can't outlive the parent that launched it. Already-terminal
// children are skipped — there's nothing to kill and overwriting their
// status would corrupt a clean Completed into Killed.
func (p *Platform) KillChildren(parentID string) []string {
	var killed []string
	for _, proc := range p.procs.list() {
		if proc.ParentID() != parentID || proc.Status().IsTerminal() {
			continue
		}
		if err := p.KillProcess(proc.ID()); err == nil {
			killed = append(killed, proc.ID())
		}
	}
	return killed
}

// RemoveProcess deletes a process from the registry. Mirrors
// embabel's AgentProcessRepository.delete: lets long-running
// services free terminal-state processes that the host has already
// drained. Returns an error when the id is unknown so callers can
// detect typos.
func (p *Platform) RemoveProcess(id string) error {
	if !p.procs.unregister(id) {
		return processNotFoundError("remove process", id)
	}
	return nil
}

// PruneTerminalProcesses removes every registered process whose
// status satisfies [core.AgentProcessStatus.IsTerminal] and returns
// the removed ids. Convenient cleanup for long-lived hosts.
func (p *Platform) PruneTerminalProcesses() []string {
	return p.procs.pruneWhere(func(proc *AgentProcess) bool {
		return proc.Status().IsTerminal()
	})
}

func processNotFoundError(operation, id string) error {
	return fmt.Errorf("%s: process %q not found", operation, id)
}
