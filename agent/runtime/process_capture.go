package runtime

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

// Snapshot captures the process's state into a portable
// [core.ProcessSnapshot] suitable for handing to a [core.ProcessStore].
// It rejects an active run instead of blocking behind arbitrary action,
// extension, or model code. Successful capture owns one stable process
// boundary and does not mutate external state.
//
// Blackboard capture is strict: the blackboard must expose
// [BlackboardSnapshotter], every durable value must be declared and JSON-safe,
// and invalid durable state returns an error.
func (p *Process) Snapshot() (core.ProcessSnapshot, error) {
	if p == nil {
		return core.ProcessSnapshot{}, errors.New("runtime.Process.Snapshot: nil process")
	}
	if err := p.state.claimCheckpoint(false); err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("runtime.Process.Snapshot: %w", err)
	}
	defer p.state.releaseCheckpoint()
	return p.snapshotClaimed()
}

func (p *Process) snapshotClaimed() (core.ProcessSnapshot, error) {
	state := p.captureDurableState()
	snapshot := core.ProcessSnapshot{
		SchemaVersion:     core.ProcessSnapshotSchemaVersion,
		ID:                p.ID(),
		ParentID:          p.ParentID(),
		Depth:             p.depth,
		Deployment:        p.Deployment(),
		StartedAt:         p.StartedAt(),
		CapturedAt:        time.Now(),
		Status:            state.status,
		Suspension:        state.suspension,
		OwnCost:           state.ownCost,
		OwnTokens:         state.ownTokens,
		OwnModelCalls:     state.modelCalls,
		OwnEmbeddingCalls: state.embeddingCalls,
	}

	if goal := state.goal; goal != nil {
		snapshot.GoalName = goal.Name()
	}
	if state.failure != nil {
		snapshot.Failure = &core.ProcessFailure{Message: state.failure.Error()}
	}
	history := state.history
	if len(history) > 0 {
		snapshot.History = make([]core.ActionRunSnapshot, len(history))
		for i, run := range history {
			snapshot.History[i] = core.ActionRunSnapshot{
				ActionName: run.ActionName,
				StartedAt:  run.StartedAt,
				Duration:   run.Duration,
				Status:     run.Status,
			}
		}
	}

	blackboardState, err := snapshotBlackboard(p.blackboard)
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("runtime.Process.Snapshot: capture blackboard: %w", err)
	}
	snapshot.Blackboard, snapshot.Objects, err = p.agent().EncodeBlackboard(blackboardState.Bindings, blackboardState.Objects)
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("runtime.Process.Snapshot: encode blackboard: %w", err)
	}
	snapshot.Conditions = blackboardState.Conditions
	if err := snapshot.Validate(); err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("runtime.Process.Snapshot: %w", err)
	}
	return snapshot, nil
}

type durableProcessState struct {
	status         core.ProcessStatus
	goal           *core.Goal
	failure        error
	suspension     *interaction.Suspension
	history        []ActionRun
	ownCost        float64
	ownTokens      int
	modelCalls     []core.ModelCall
	embeddingCalls []core.EmbeddingCall
}

func (p *Process) captureDurableState() durableProcessState {
	p.state.mu.RLock()
	defer p.state.mu.RUnlock()
	var suspension *interaction.Suspension
	if p.state.pendingSuspension != nil {
		suspension = p.state.pendingSuspension.Clone()
	}
	return durableProcessState{
		status:         p.state.currentStatus,
		goal:           p.state.currentGoal,
		failure:        p.state.runErr,
		suspension:     suspension,
		history:        slices.Clone(p.state.history),
		ownCost:        p.budget.ownCost,
		ownTokens:      p.budget.ownTokens,
		modelCalls:     slices.Clone(p.budget.modelCalls),
		embeddingCalls: slices.Clone(p.budget.embeddingCalls),
	}
}
