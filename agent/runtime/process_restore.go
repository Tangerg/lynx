package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

// ValidateResumableSnapshot verifies that a process snapshot contains a
// continuation the runtime can safely restore. Both an unanswered suspension
// awaiting Resume and an answered suspension awaiting Continue are valid. It
// interprets only FrameworkState and performs no external I/O.
//
// Every failure reports [core.ErrInvalidSnapshot], so a host deciding between
// "this capture is unusable, recover the run as lost" and "something else went
// wrong" has one question to ask rather than a set of errors to enumerate.
func ValidateResumableSnapshot(snapshot core.ProcessSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("runtime.ValidateResumableSnapshot: %w", err)
	}
	if snapshot.Status != core.StatusWaiting || snapshot.Suspension == nil {
		return fmt.Errorf("runtime.ValidateResumableSnapshot: %w: process %q is not waiting on a suspension", core.ErrInvalidSnapshot, snapshot.ID)
	}
	suspension := snapshot.Suspension
	checkpoint, err := decodeSuspensionCheckpoint(suspension.FrameworkState)
	if err != nil {
		return fmt.Errorf("runtime.ValidateResumableSnapshot: %w", err)
	}
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Kind == suspensionCheckpointNestedChild {
		return nil
	}
	if checkpoint.Deployment != snapshot.Deployment {
		return fmt.Errorf("runtime.ValidateResumableSnapshot: %w: tool checkpoint deployment does not match snapshot deployment", core.ErrInvalidSnapshot)
	}
	if checkpoint.Checkpoint.ID != suspension.ID {
		return fmt.Errorf("runtime.ValidateResumableSnapshot: %w: tool checkpoint ID %q does not match suspension ID %q", core.ErrInvalidSnapshot, checkpoint.Checkpoint.ID, suspension.ID)
	}
	return nil
}

// ValidateRestoreTree verifies that the complete caller-supplied process tree
// is structurally valid and references deployments available to this engine.
// It performs no I/O and does not mutate runtime state.
func (e *Engine) ValidateRestoreTree(tree core.ProcessSnapshotTree) error {
	if e == nil {
		return errors.New("runtime.Engine.ValidateRestoreTree: nil Engine")
	}
	if err := tree.Validate(); err != nil {
		return fmt.Errorf("runtime.Engine.ValidateRestoreTree: %w", err)
	}
	if err := validateNestedSnapshotRelations(tree); err != nil {
		return fmt.Errorf("runtime.Engine.ValidateRestoreTree: %w", err)
	}
	for _, snapshot := range tree.Snapshots {
		deployment, ok := e.catalog.lookup(snapshot.Deployment)
		if !ok {
			return fmt.Errorf(
				"runtime.Engine.ValidateRestoreTree: process %q: %w: %s",
				snapshot.ID,
				ErrDeploymentNotFound,
				snapshot.Deployment,
			)
		}
		if snapshot.Status == core.StatusWaiting {
			if err := ValidateResumableSnapshot(snapshot); err != nil {
				// The classification travels with the validator's own error; adding
				// it again here would be a second owner of the same judgement.
				return fmt.Errorf(
					"runtime.Engine.ValidateRestoreTree: process %q continuation: %w",
					snapshot.ID,
					err,
				)
			}
		}
		if _, _, err := deployment.agent.DecodeBlackboard(snapshot.Blackboard, snapshot.Objects); err != nil {
			return fmt.Errorf(
				"runtime.Engine.ValidateRestoreTree: %w: process %q blackboard: %w",
				core.ErrInvalidSnapshot,
				snapshot.ID,
				err,
			)
		}
		if snapshot.GoalName != "" && !agentHasGoal(deployment.agent, snapshot.GoalName) {
			return fmt.Errorf(
				"runtime.Engine.ValidateRestoreTree: %w: process %q references unknown goal %q",
				core.ErrInvalidSnapshot,
				snapshot.ID,
				snapshot.GoalName,
			)
		}
	}
	return nil
}

func agentHasGoal(agent *core.Agent, name string) bool {
	if agent == nil {
		return false
	}
	for _, goal := range agent.Goals() {
		if goal.Name() == name {
			return true
		}
	}
	return false
}

// RestoreTree atomically rebuilds and registers a caller-supplied complete
// process tree. The caller owns loading and classifying external state; runtime
// validates only its execution snapshot contract.
func (e *Engine) RestoreTree(
	ctx context.Context,
	tree core.ProcessSnapshotTree,
	options core.ProcessOptions,
) (*Process, error) {
	if e == nil {
		return nil, errors.New("runtime.Engine.RestoreTree: nil Engine")
	}
	if err := e.ValidateRestoreTree(tree); err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreTree: %w", err)
	}

	byID := make(map[string]core.ProcessSnapshot, len(tree.Snapshots))
	children := make(map[string][]core.ProcessSnapshot)
	for _, snapshot := range tree.Snapshots {
		byID[snapshot.ID] = snapshot
		if snapshot.ID != tree.RootID {
			children[snapshot.ParentID] = append(children[snapshot.ParentID], snapshot)
		}
	}
	for parentID := range children {
		slices.SortFunc(children[parentID], func(left, right core.ProcessSnapshot) int {
			if left.ID < right.ID {
				return -1
			}
			if left.ID > right.ID {
				return 1
			}
			return 0
		})
	}

	var processes []*Process
	root, err := e.restoreSnapshotSubtree(
		normalizeContext(ctx),
		byID[tree.RootID],
		options,
		nil,
		children,
		&processes,
	)
	if err != nil {
		releaseProcessDeployments(processes)
		return nil, fmt.Errorf("runtime.Engine.RestoreTree: %w", err)
	}
	if err := root.budget.restoreAuthority(root.Usage()); err != nil {
		releaseProcessDeployments(processes)
		return nil, fmt.Errorf("runtime.Engine.RestoreTree: restore budget authority: %w", err)
	}
	releaseMutation, err := e.processMutations.acquire(
		normalizeContext(ctx),
		root.ID(),
	)
	if err != nil {
		releaseProcessDeployments(processes)
		return nil, fmt.Errorf("runtime.Engine.RestoreTree: acquire process tree: %w", err)
	}
	defer releaseMutation()
	if !e.processes.registerTree(processes) {
		releaseProcessDeployments(processes)
		return nil, fmt.Errorf("runtime.Engine.RestoreTree: process tree %q is already registered", tree.RootID)
	}
	return root, nil
}

func (e *Engine) restoreSnapshotSubtree(
	ctx context.Context,
	snapshot core.ProcessSnapshot,
	options core.ProcessOptions,
	parent *Process,
	children map[string][]core.ProcessSnapshot,
	processes *[]*Process,
) (*Process, error) {
	process, err := e.buildProcessSnapshot(snapshot, options)
	if err != nil {
		return nil, fmt.Errorf("rebuild process %q: %w", snapshot.ID, err)
	}
	if parent != nil {
		// Depth follows the restored parent link rather than traveling in the
		// snapshot, so the tree is the only place it is expressed.
		process.depth = parent.depth + 1
		parent.budget.addChild(process)
	}
	*processes = append(*processes, process)

	for _, childSnapshot := range children[snapshot.ID] {
		childOptions, err := restoredChildOptions(ctx, process, e, childSnapshot.Deployment)
		if err != nil {
			return nil, err
		}
		if _, err := e.restoreSnapshotSubtree(ctx, childSnapshot, childOptions, process, children, processes); err != nil {
			return nil, err
		}
	}
	return process, nil
}

func restoredChildOptions(
	ctx context.Context,
	parent *Process,
	engine *Engine,
	deploymentRef core.DeploymentRef,
) (core.ProcessOptions, error) {
	deployment, ok := engine.catalog.lookup(deploymentRef)
	if !ok {
		return core.ProcessOptions{}, fmt.Errorf("%w: %s", ErrDeploymentNotFound, deploymentRef)
	}
	options, err := configureChildProcessOptions(ctx, parent, deployment, core.ProcessOptions{})
	if err != nil {
		return core.ProcessOptions{}, err
	}
	extensions, err := parent.childExtensions(options.Extensions)
	if err != nil {
		return core.ProcessOptions{}, fmt.Errorf("restore child extensions: %w", err)
	}
	options.Extensions = extensions
	return options, nil
}

func (e *Engine) buildProcessSnapshot(snapshot core.ProcessSnapshot, options core.ProcessOptions) (*Process, error) {
	if e == nil {
		return nil, errors.New("nil engine")
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}

	deployment, ok := e.catalog.lookup(snapshot.Deployment)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrDeploymentNotFound, snapshot.Deployment)
	}
	if !e.catalog.retain(deployment) {
		return nil, fmt.Errorf("%w: %s", ErrDeploymentNotFound, snapshot.Deployment)
	}
	retained := true
	defer func() {
		if retained {
			e.catalog.release(deployment)
		}
	}()
	agent := deployment.agent

	processOptions, err := snapshotProcessOptions(options)
	if err != nil {
		return nil, err
	}
	dependencies, err := e.prepareProcessDependencies(options.Dependencies)
	if err != nil {
		return nil, err
	}
	blackboard, err := e.resolveBlackboard(agent.SnapshotCodec(), options.Blackboard)
	if err != nil {
		return nil, err
	}
	planner, err := e.resolvePlanner(agent, processOptions.extensions)
	if err != nil {
		return nil, err
	}
	domain, err := planning.DomainForAgent(agent)
	if err != nil {
		return nil, fmt.Errorf("domain: %w", err)
	}

	process := newProcess(snapshot.ID, deployment, &processOptions, blackboard, dependencies, planner, domain, e)
	process.deploymentRetained = true
	process.wireRuntimeDeps(processOptions.extensions)
	process.parentID = snapshot.ParentID
	process.startedAt = snapshot.StartedAt

	process.state.transition(snapshot.Status)
	if err := process.state.restoreSuspension(snapshot.Suspension); err != nil {
		return nil, fmt.Errorf("%w: suspension: %w", core.ErrInvalidSnapshot, err)
	}
	if err := process.restoreNestedSuspension(snapshot.Suspension); err != nil {
		return nil, fmt.Errorf("%w: nested suspension: %w", core.ErrInvalidSnapshot, err)
	}
	if snapshot.GoalName != "" {
		for _, goal := range agent.Goals() {
			if goal.Name() == snapshot.GoalName {
				process.state.pursue(goal)
				break
			}
		}
	}
	if snapshot.Failure != nil {
		failure := *snapshot.Failure
		process.state.restoreFailure(&failure)
	}
	process.budget.restore(snapshot.OwnUsage)

	bindings, objects, err := agent.DecodeBlackboard(snapshot.Blackboard, snapshot.Objects)
	if err != nil {
		return nil, fmt.Errorf("%w: decode blackboard: %w", core.ErrInvalidSnapshot, err)
	}
	if err := restoreBlackboard(blackboard, BlackboardState{
		Bindings:   bindings,
		Conditions: snapshot.Conditions,
		Objects:    objects,
	}); err != nil {
		return nil, fmt.Errorf("%w: restore blackboard: %w", core.ErrInvalidSnapshot, err)
	}

	retained = false
	return process, nil
}

func releaseProcessDeployments(processes []*Process) {
	for _, process := range processes {
		process.releaseDeployment()
	}
}
