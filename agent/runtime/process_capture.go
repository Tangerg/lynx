package runtime

import (
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

func (p *Process) snapshotClaimed() (core.ProcessSnapshot, error) {
	state := p.captureSnapshotState()
	snapshot := core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            p.ID(),
		ParentID:      p.ParentID(),
		Depth:         p.depth,
		Deployment:    p.Deployment(),
		StartedAt:     p.StartedAt(),
		Status:        state.status,
		Suspension:    state.suspension,
		OwnUsage:      state.ownUsage,
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
		return core.ProcessSnapshot{}, fmt.Errorf("capture blackboard: %w", err)
	}
	snapshot.Blackboard, snapshot.Objects, err = p.agent().EncodeBlackboard(blackboardState.Bindings, blackboardState.Objects)
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("encode blackboard: %w", err)
	}
	snapshot.Conditions = blackboardState.Conditions
	if err := snapshot.Validate(); err != nil {
		return core.ProcessSnapshot{}, err
	}
	return snapshot, nil
}

type processCaptureState struct {
	status     core.ProcessStatus
	goal       *core.Goal
	failure    error
	suspension *interaction.Suspension
	history    []ActionRun
	ownUsage   core.Usage
}

func (p *Process) captureSnapshotState() processCaptureState {
	p.state.mu.RLock()
	defer p.state.mu.RUnlock()
	var suspension *interaction.Suspension
	if p.state.pendingSuspension != nil {
		suspension = p.state.pendingSuspension.Clone()
	}
	return processCaptureState{
		status:     p.state.currentStatus,
		goal:       p.state.currentGoal,
		failure:    p.state.runErr,
		suspension: suspension,
		history:    slices.Clone(p.state.history),
		ownUsage:   p.budget.own,
	}
}
