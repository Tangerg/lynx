package runtime

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

// Snapshot captures the process's state into a portable
// [core.ProcessSnapshot] suitable for handing to a [core.ProcessStore].
// Acquires only the process's own read locks — no external state is
// mutated.
//
// Blackboard capture is best-effort: any custom blackboard
// implementation that satisfies [BlackboardSnapshotter] gets its
// rich state; everything else falls back to a shallow read of the
// named-key view via [core.BlackboardReader.Get] over recorded
// objects.
func (p *AgentProcess) Snapshot() core.ProcessSnapshot {
	if p == nil {
		return core.ProcessSnapshot{}
	}

	snap := core.ProcessSnapshot{
		ID:                   p.ID(),
		ParentID:             p.ParentID(),
		Depth:                p.depth,
		StartedAt:            p.StartedAt(),
		CapturedAt:           core.Now(),
		Status:               p.Status(),
		LastWorld:            p.LastWorldState(),
		LLMInvocations:       p.LLMInvocations(),
		EmbeddingInvocations: p.EmbeddingInvocations(),
	}

	if p.agent != nil {
		snap.AgentName = p.agent.Name
		if p.agent.Version != nil {
			snap.AgentVersion = p.agent.Version.String()
		}
	}
	if goal := p.Goal(); goal != nil {
		snap.GoalName = goal.Name
	}
	if err := p.Failure(); err != nil {
		snap.Failure = err.Error()
	}
	cost, tokens, _ := p.Usage()
	snap.Cost = cost
	snap.Tokens = tokens

	hist := p.History()
	if len(hist) > 0 {
		snap.History = make([]core.SnapshotActionInvocation, len(hist))
		for i, inv := range hist {
			snap.History[i] = core.SnapshotActionInvocation{
				ActionName: inv.ActionName,
				Timestamp:  inv.Timestamp,
				Duration:   inv.Duration,
				Status:     inv.Status.String(),
				Attempts:   inv.Attempts,
			}
		}
	}

	// Type assertion on a nil interface returns (zero, false) — no
	// guard needed before the assertion. Values are type-tagged so a JSON
	// round-trip through the store reconstructs their concrete Go types on
	// restore (see core.TagBlackboard) rather than decoding into bare maps.
	if s, ok := p.blackboard.(BlackboardSnapshotter); ok {
		named, conditions, objects := s.Snapshot()
		snap.Blackboard, snap.Objects = core.TagBlackboard(named, objects)
		snap.Conditions = conditions
	}
	return snap
}

// RestoreFromSnapshot rebuilds an [AgentProcess] from a snapshot the
// caller already holds — the pure-rebuild primitive, no store I/O.
// ([Platform.RestoreProcess] is the store-backed sibling: it loads the
// snapshot by id, then calls this.) The process is added to platform's
// registry under the snapshot's id; the agent definition is looked up by
// [core.ProcessSnapshot.AgentName] and must already be deployed.
//
// Resumable statuses (Running / Waiting / Paused) leave the process
// ready for re-entry into the tick loop. Terminal statuses
// (Completed / Failed / Killed / Terminated) load the process
// read-only; callers can inspect History / Usage / Failure but
// not re-run.
//
// Resuming a restored StatusWaiting process takes four steps, because
// the pending awaitable's handler closure does not round-trip (see
// [core.ProcessSnapshot]):
//
//  1. RestoreFromSnapshot — status is Waiting, but nothing is parked yet
//     (PendingAwaitable returns nil).
//  2. ContinueProcess — re-ticks once; the awaiting action re-issues
//     AwaitInput against the restored blackboard and the process parks
//     again (now PendingAwaitable is populated).
//  3. ResumeProcess(id, response) — delivers the response to the freshly
//     re-parked handler, which mutates the blackboard.
//  4. ContinueProcess — drives the loop to a terminal state against the
//     now-decided blackboard.
//
// This works only when the awaiting action is idempotent: it must
// re-park when its decision condition is unset and proceed when it's
// set. The framework cannot reconstruct the closure for it.
//
// options carries the per-process wiring the snapshot can't hold — the
// session-scoped [core.ProcessOptions.Extensions] (observer / event
// listener / tool decorators) and the [core.ProcessOptions.Session]
// binding. A restored process re-enters the tick loop with the same
// observability + session context a fresh one gets from
// [Platform.StartAgent], so the continuation streams and keys chat history
// correctly. Pass the zero value to restore read-only (audit / inspect).
func (p *Platform) RestoreFromSnapshot(snap core.ProcessSnapshot, options core.ProcessOptions) (*AgentProcess, error) {
	if p == nil {
		return nil, errors.New("runtime.Platform.RestoreFromSnapshot: nil platform")
	}
	if snap.ID == "" {
		return nil, errors.New("runtime.Platform.RestoreFromSnapshot: snapshot has empty ID")
	}
	if snap.AgentName == "" {
		return nil, errors.New("runtime.Platform.RestoreFromSnapshot: snapshot has empty AgentName")
	}

	agentDef, ok := p.agents.find(snap.AgentName)
	if !ok {
		return nil, fmt.Errorf("runtime.Platform.RestoreFromSnapshot: agent %q not deployed", snap.AgentName)
	}

	if err := validateProcessExtensions(options.Extensions); err != nil {
		return nil, fmt.Errorf("runtime.Platform.RestoreFromSnapshot: %w", err)
	}
	options.ApplyDefaults()
	blackboard := p.resolveBlackboard(options.Blackboard)
	plannerInst, err := p.resolvePlanner(agentDef, options.Extensions)
	if err != nil {
		return nil, fmt.Errorf("runtime.Platform.RestoreFromSnapshot: %w", err)
	}
	system := planning.FromAgent(agentDef)

	proc := newAgentProcess(snap.ID, agentDef, &options, blackboard, plannerInst, system, p)
	// Wire the determiner + event multicast the same way createProcess
	// does — without it a resumable snapshot panics on its first
	// post-restore tick (nil determiner in observe). The caller's
	// Extensions (observer / listener) attach here too.
	proc.wireRuntimeDeps(options.Extensions)
	proc.parentID = snap.ParentID
	proc.depth = snap.Depth
	proc.startedAt = snap.StartedAt

	// Re-populate state.
	proc.state.setStatus(snap.Status)
	if snap.GoalName != "" {
		for _, g := range agentDef.Goals {
			if g.Name == snap.GoalName {
				proc.state.setGoal(g)
				break
			}
		}
	}
	if snap.LastWorld != nil {
		proc.state.setLastWorld(snap.LastWorld)
	}
	if snap.Failure != "" {
		proc.state.setFailure(errors.New(snap.Failure))
	}
	for _, h := range snap.History {
		proc.state.recordInvocation(ActionInvocation{
			ActionName: h.ActionName,
			Timestamp:  h.Timestamp,
			Duration:   h.Duration,
			Status:     parseActionStatus(h.Status),
			Attempts:   h.Attempts,
		})
	}

	proc.budget.restore(snap.Cost, snap.Tokens, snap.LLMInvocations, snap.EmbeddingInvocations)

	// Re-populate blackboard when the implementation supports it. The
	// tagged values decode back to their concrete Go types via the type
	// table the agent's action I/O bindings declare (see
	// core.UntagBlackboard) — so a restored typed-action input is the
	// original struct, not the map JSON would otherwise yield.
	if r, ok := blackboard.(BlackboardRestorer); ok {
		named, objects := core.UntagBlackboard(snap.Blackboard, snap.Objects, agentDef)
		r.Restore(named, snap.Conditions, objects)
	}

	// Restore keeps the snapshot's ORIGINAL process id, so refuse to clobber an
	// id still held by a live process (e.g. an auto-snapshot re-restoring while
	// the original ticks) — that would split the id across two objects. A
	// terminal / absent slot replaces cleanly.
	if !p.procs.registerNew(proc) {
		return nil, fmt.Errorf("runtime: cannot restore process %s: a live process with that id is already running", proc.id)
	}
	return proc, nil
}

// parseActionStatus maps the string form back to the enum. Unknown
// values fall back to [core.ActionFailed] so the runtime treats them
// conservatively rather than silently downgrading to Succeeded.
func parseActionStatus(s string) core.ActionStatus {
	switch s {
	case core.ActionSucceeded.String():
		return core.ActionSucceeded
	case core.ActionFailed.String():
		return core.ActionFailed
	case core.ActionWaiting.String():
		return core.ActionWaiting
	case core.ActionPaused.String():
		return core.ActionPaused
	default:
		return core.ActionFailed
	}
}
