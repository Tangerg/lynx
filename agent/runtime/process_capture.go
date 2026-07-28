package runtime

import (
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

func (p *Process) snapshotClaimed() (core.ProcessSnapshot, error) {
	state := p.captureSnapshotState()
	snapshot := core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            p.ID(),
		ParentID:      p.ParentID(),
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
	blackboardState, err := snapshotBlackboard(p.blackboard)
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("capture blackboard: %w", err)
	}
	snapshot.Blackboard, snapshot.Objects, err = p.agent().EncodeBlackboard(blackboardState.Bindings, blackboardState.Objects)
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("encode blackboard: %w", err)
	}
	snapshot.Hidden, err = encodeBlackboardValues(p.agent().SnapshotCodec(), blackboardState.Hidden)
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("encode hidden blackboard state: %w", err)
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
		ownUsage:   p.budget.ownUsage(),
	}
}
