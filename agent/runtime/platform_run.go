package runtime

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// RunAgent runs the named agent synchronously and returns the
// resulting process (whether completed or terminal-failed). Pass a
// zero [core.ProcessOptions]{} for defaults.
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
	if err := proc.run(normalizeContext(ctx)); err != nil {
		return proc, err
	}
	return proc, nil
}

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
	proc, ok := p.GetProcess(id)
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

	proc, ok := p.GetProcess(id)
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
	proc, ok := p.GetProcess(id)
	if !ok {
		return core.ResponseImpactUnchanged, processNotFoundError("resume process", id)
	}
	impact, err := proc.signals.deliverResponse(response)
	if err != nil {
		return core.ResponseImpactUnchanged, fmt.Errorf("resume process %q: %w", id, err)
	}
	return impact, nil
}

// KillProcess terminates a running process. Returns an error when
// the id is unknown.
func (p *Platform) KillProcess(id string) error {
	proc, ok := p.GetProcess(id)
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
