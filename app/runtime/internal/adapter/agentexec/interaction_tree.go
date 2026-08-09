package agentexec

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/interactioninput"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

const interactionBarrierPauseReason = "runtime human-input tree barrier"

// CancelRunningSubtree submits a product-owned cancellation to one exact live
// managed Delegate. Agent Framework owns propagation to descendants and the resulting
// child completion; Runtime observes those facts through the normal tree pump.
func (executor *InteractionExecutor) CancelRunningSubtree(
	ctx context.Context,
	ref runs.ExecutorRef,
	memberID string,
	reason string,
) error {
	session, err := executor.session(ref)
	if err != nil {
		return err
	}
	processID, err := agent.ParseProcessID(memberID)
	if err != nil {
		return fmt.Errorf("agentexec: parse running Interaction member: %w", err)
	}
	session.mu.Lock()
	root := session.process
	managed := session.delegateChildren[processID]
	available := !session.finished && session.boundary == interactionBoundaryInactive
	session.mu.Unlock()
	if !available || root == nil {
		return runs.ErrExecutorNotLive
	}
	if managed == nil || processID == root.ID() {
		return errors.New("agentexec: running subtree target is not a managed Delegate")
	}
	process, found := session.engine.Process(processID)
	if !found || process.Relation().RootID() != root.ID() {
		return runs.ErrExecutorNotLive
	}
	controlCtx, cancel := session.lifecycleContext(ctx)
	defer cancel()
	if err := process.RequestCancellation(executionctx.WithScope(controlCtx, session.scope), reason); err != nil {
		return fmt.Errorf("agentexec: cancel running Interaction member %s: %w", processID, err)
	}
	return nil
}

// captureHumanInputBarrier first proves that the tree contains an externally
// addressable input wait, then pauses other Running branches at their next safe
// boundary. A parent waiting only on children is not itself a product barrier;
// pausing its still-opening child before this proof would capture a half-step
// with no durable Interrupt to resume.
func (session *interactionSession) captureHumanInputBarrier(
	ctx context.Context,
) (agent.TreeSnapshot, []runs.MemberInterruption, bool, error) {
	root := session.processHandle()
	if root == nil || root.Status() != agent.StatusWaiting {
		return agent.TreeSnapshot{}, nil, false, errors.New("agentexec: Interaction root is not waiting")
	}
	for {
		tree, err := session.engine.CaptureTree(ctx, root.Relation().RootID())
		if err != nil {
			return agent.TreeSnapshot{}, nil, false, err
		}
		interruptions, err := session.pendingInterruptions(tree)
		if err != nil {
			return agent.TreeSnapshot{}, nil, false, err
		}
		if len(interruptions) == 0 {
			return agent.TreeSnapshot{}, nil, false, nil
		}
		var paused bool
		for _, snapshot := range tree.ProcessSnapshots() {
			if snapshot.Status() != agent.StatusRunning {
				continue
			}
			process, found := session.engine.Process(snapshot.ProcessID())
			if !found {
				paused = true
				continue
			}
			if err := process.Pause(ctx, interactionBarrierPauseReason); err != nil &&
				!errors.Is(err, agent.ErrProcessFinished) {
				return agent.TreeSnapshot{}, nil, false, fmt.Errorf("pause Interaction member %s: %w", snapshot.ProcessID(), err)
			}
			paused = true
		}
		if paused {
			continue
		}
		return tree, interruptions, true, nil
	}
}

func (session *interactionSession) pendingInterruptions(
	tree agent.TreeSnapshot,
) ([]runs.MemberInterruption, error) {
	if !tree.Valid() {
		return nil, errors.New("agentexec: inspect pending inputs from invalid Interaction tree")
	}
	interruptions := make([]runs.MemberInterruption, 0)
	for _, snapshot := range tree.ProcessSnapshots() {
		pending, found, err := interaction.PendingToolInputFromSnapshot(snapshot)
		if err != nil {
			return nil, fmt.Errorf("inspect Interaction member %s input: %w", snapshot.ProcessID(), err)
		}
		if !found {
			continue
		}
		if _, bound := session.executorMemberByProcessID(snapshot.ProcessID()); !bound {
			return nil, fmt.Errorf("pending Interaction member %s has no product binding", snapshot.ProcessID())
		}
		prompt, err := interactioninput.DecodePrompt(pending.Prompt())
		if err != nil {
			return nil, fmt.Errorf("decode Interaction member %s prompt: %w", snapshot.ProcessID(), err)
		}
		interruptions = append(interruptions, runs.MemberInterruption{
			MemberID: snapshot.ProcessID().String(), RequestID: pending.WaitID().String(), Interrupt: prompt,
		})
	}
	slices.SortFunc(interruptions, func(left, right runs.MemberInterruption) int {
		if order := strings.Compare(left.MemberID, right.MemberID); order != 0 {
			return order
		}
		return strings.Compare(left.RequestID, right.RequestID)
	})
	return interruptions, nil
}

func (session *interactionSession) unknownEffectIDs(ctx context.Context) ([]agent.EffectID, error) {
	session.mu.Lock()
	root := session.process
	children := make([]agent.ProcessID, 0, len(session.delegateChildren))
	for processID := range session.delegateChildren {
		children = append(children, processID)
	}
	session.mu.Unlock()
	if root == nil {
		return nil, runs.ErrExecutorNotLive
	}
	slices.SortFunc(children, func(left, right agent.ProcessID) int {
		return strings.Compare(left.String(), right.String())
	})
	processes := make([]*agent.Process, 0, len(children)+1)
	processes = append(processes, root)
	for _, processID := range children {
		process, found := session.engine.Process(processID)
		if found {
			processes = append(processes, process)
		}
	}
	ids := make([]agent.EffectID, 0)
	for _, process := range processes {
		unknown, err := process.UnknownEffectIDs(ctx)
		if err != nil {
			return nil, fmt.Errorf("inspect Interaction member %s effects: %w", process.ID(), err)
		}
		ids = append(ids, unknown...)
	}
	slices.SortFunc(ids, func(left, right agent.EffectID) int {
		return strings.Compare(left.String(), right.String())
	})
	ids = slices.Compact(ids)
	return ids, nil
}

func (session *interactionSession) pausedProcessIDs() ([]agent.ProcessID, error) {
	session.mu.Lock()
	checkpoint := session.waitingCheckpoint.Clone()
	session.mu.Unlock()
	state, err := decodeInteractionCheckpointPayload(checkpoint.Payload)
	if err != nil {
		return nil, err
	}
	paused := make([]agent.ProcessID, 0)
	for _, snapshot := range state.tree.ProcessSnapshots() {
		if snapshot.Status() == agent.StatusPaused {
			paused = append(paused, snapshot.ProcessID())
		}
	}
	return paused, nil
}

func (session *interactionSession) resumePausedProcesses(
	ctx context.Context,
	processIDs []agent.ProcessID,
) error {
	processes := make([]*agent.Process, len(processIDs))
	for index, processID := range processIDs {
		process, found := session.engine.Process(processID)
		if !found {
			return fmt.Errorf("agentexec: paused Interaction member %s is unavailable", processID)
		}
		if process.Status() != agent.StatusPaused {
			return fmt.Errorf("agentexec: Interaction member %s left its paused boundary", processID)
		}
		processes[index] = process
	}
	for index, process := range processes {
		if err := process.Resume(ctx); err != nil {
			return fmt.Errorf("agentexec: resume Interaction member %s: %w", processIDs[index], err)
		}
	}
	return nil
}
