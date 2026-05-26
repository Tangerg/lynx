package runtime

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

// SnapshotProcess captures the state of process into a portable
// [core.ProcessSnapshot] suitable for handing to a [core.ProcessStore].
// Acquires only the process's own read locks — no external state is
// mutated.
//
// Blackboard capture is best-effort: any custom blackboard
// implementation that satisfies [BlackboardSnapshotter] gets its
// rich state; everything else falls back to a shallow read of the
// named-key view via [core.BlackboardReader.Get] over recorded
// objects.
func SnapshotProcess(p *AgentProcess) core.ProcessSnapshot {
	if p == nil {
		return core.ProcessSnapshot{}
	}

	snap := core.ProcessSnapshot{
		ID:                   p.ID(),
		ParentID:             p.ParentID(),
		StartedAt:            p.StartedAt(),
		CapturedAt:           time.Now(),
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
	// guard needed before the assertion.
	if s, ok := p.blackboard.(BlackboardSnapshotter); ok {
		snap.Blackboard, snap.Conditions, snap.Objects = s.Snapshot()
	}
	return snap
}

// RestoreProcess rebuilds an [AgentProcess] from a snapshot. The
// process is added to platform's registry under the snapshot's id;
// the agent definition is looked up by [core.ProcessSnapshot.AgentName]
// and must already be deployed.
//
// Resumable statuses (Running / Waiting / Paused) leave the process
// ready for re-entry into the tick loop. Terminal statuses
// (Completed / Failed / Killed / Terminated) load the process
// read-only; callers can inspect History / Usage / Failure but
// not re-run.
func RestoreProcess(platform *Platform, snap core.ProcessSnapshot) (*AgentProcess, error) {
	if platform == nil {
		return nil, fmt.Errorf("restore process: nil platform")
	}
	if snap.ID == "" {
		return nil, fmt.Errorf("restore process: snapshot has empty ID")
	}
	if snap.AgentName == "" {
		return nil, fmt.Errorf("restore process: snapshot has empty AgentName")
	}

	agentDef, ok := platform.agents.find(snap.AgentName)
	if !ok {
		return nil, fmt.Errorf("restore process: agent %q not deployed", snap.AgentName)
	}

	options := &core.ProcessOptions{}
	options.ApplyDefaults()
	blackboard := platform.NewBlackboard()
	plannerInst, err := platform.resolvePlanner(agentDef, options.Extensions)
	if err != nil {
		return nil, fmt.Errorf("restore process: %w", err)
	}
	system := planning.FromAgent(agentDef)

	p := newAgentProcess(snap.ID, agentDef, options, blackboard, plannerInst, system, platform)
	p.parentID = snap.ParentID
	p.startedAt = snap.StartedAt

	// Re-populate state.
	p.state.setStatus(snap.Status)
	if snap.GoalName != "" {
		for _, g := range agentDef.Goals {
			if g.Name == snap.GoalName {
				p.state.setGoal(g)
				break
			}
		}
	}
	if snap.LastWorld != nil {
		p.state.setLastWorld(snap.LastWorld)
	}
	if snap.Failure != "" {
		p.state.setFailure(errors.New(snap.Failure))
	}
	for _, h := range snap.History {
		p.state.recordInvocation(ActionInvocation{
			ActionName: h.ActionName,
			Timestamp:  h.Timestamp,
			Duration:   h.Duration,
			Status:     parseActionStatus(h.Status),
			Attempts:   h.Attempts,
		})
	}

	p.budget.restore(snap.Cost, snap.Tokens, snap.LLMInvocations, snap.EmbeddingInvocations)

	// Re-populate blackboard when the implementation supports it.
	if r, ok := blackboard.(BlackboardRestorer); ok {
		r.Restore(snap.Blackboard, snap.Conditions, snap.Objects)
	}

	platform.procs.register(p)
	return p, nil
}

// BlackboardSnapshotter is the optional capture surface a custom
// [core.Blackboard] implementation exposes so [SnapshotProcess] can
// persist its full state. The three returned values mirror
// [core.ProcessSnapshot]'s Blackboard / Conditions / Objects fields.
// Implementations are free to return nil for any value.
type BlackboardSnapshotter interface {
	Snapshot() (named map[string]any, conditions map[string]bool, objects []any)
}

// BlackboardRestorer is the optional restore surface. The runtime
// passes back whatever [BlackboardSnapshotter.Snapshot] previously
// produced. Implementations may apply selective filtering (e.g. only
// restore JSON-friendly types).
type BlackboardRestorer interface {
	Restore(named map[string]any, conditions map[string]bool, objects []any)
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
